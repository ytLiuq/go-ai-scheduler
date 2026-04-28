package retry

import (
	"context"
	"log"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/rpc"
	"github.com/example/go-ai-scheduler/internal/repo"
	"github.com/example/go-ai-scheduler/internal/scheduler/dispatch"
	"github.com/example/go-ai-scheduler/internal/scheduler/route"
)

// Loop re-dispatches retry instances that are waiting for capacity.
type Loop struct {
	tasks        repo.TaskRepository
	instances    repo.TaskInstanceRepository
	router       *route.Router
	dispatcher   *dispatch.Client
	logger       *log.Logger
	interval     time.Duration
	schedulerURL string
}

// NewLoop creates a retry loop.
func NewLoop(
	tasks repo.TaskRepository,
	instances repo.TaskInstanceRepository,
	router *route.Router,
	dispatcher *dispatch.Client,
	logger *log.Logger,
	interval time.Duration,
	schedulerURL string,
) *Loop {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	return &Loop{
		tasks:        tasks,
		instances:    instances,
		router:       router,
		dispatcher:   dispatcher,
		logger:       logger,
		interval:     interval,
		schedulerURL: schedulerURL,
	}
}

// Start runs the retry scan loop.
func (l *Loop) Start(ctx context.Context) {
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()

	l.scan(ctx)
	for {
		select {
		case <-ctx.Done():
			l.logger.Printf("retry loop stopped")
			return
		case <-ticker.C:
			l.scan(ctx)
		}
	}
}

func (l *Loop) scan(ctx context.Context) {
	instances, err := l.instances.ListDueRetryInstances(ctx, time.Now(), 100)
	if err != nil {
		l.logger.Printf("list retry_waiting instances failed: %v", err)
		return
	}

	for _, instance := range instances {
		task, err := l.tasks.GetTask(ctx, instance.TaskID)
		if err != nil {
			l.logger.Printf("load task for retry instance failed instance_id=%d err=%v", instance.ID, err)
			continue
		}

		worker, err := l.router.Pick(ctx, route.SelectOptions{
			Labels:   model.DecodeLabels(task.Labels),
			Strategy: task.RouteStrategy,
		})
		if err != nil {
			if err == route.ErrNoAvailableWorker {
				return
			}
			l.logger.Printf("pick worker for retry instance failed instance_id=%d err=%v", instance.ID, err)
			continue
		}

		dispatchTime := time.Now()
		if err := l.instances.UpdateInstanceDispatch(ctx, instance.ID, worker.ID, dispatchTime.Format(time.RFC3339Nano)); err != nil {
			_ = l.router.Release(ctx, worker)
			l.logger.Printf("update retry instance dispatch failed instance_id=%d err=%v", instance.ID, err)
			continue
		}

		err = l.dispatcher.Dispatch(ctx, worker, rpc.ExecuteTaskRequest{
			ScheduleInstanceID: instance.ScheduleInstanceID,
			TaskID:             task.ID,
			TaskType:           task.Type,
			Payload:            task.Payload,
			TimeoutSeconds:     task.TimeoutSeconds,
			RetryCount:         instance.RetryCount,
			ShardNo:            instance.ShardNo,
			ShardTotal:         instance.ShardTotal,
			IdempotencyKey:     task.IdempotencyKey,
			SchedulerURL:       l.schedulerURL,
		})
		if err != nil {
			_ = l.instances.UpdateInstanceStatus(ctx, instance.ID, "retry_waiting")
			_ = l.router.Release(ctx, worker)
			l.logger.Printf("dispatch retry instance failed instance_id=%d err=%v", instance.ID, err)
			continue
		}

		l.logger.Printf("retry instance dispatched instance_id=%d worker_id=%s retry_count=%d",
			instance.ID, worker.ID, instance.RetryCount)
	}
}
