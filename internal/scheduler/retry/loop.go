package retry

import (
	"context"
	"log/slog"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
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
	logger       *slog.Logger
	interval     time.Duration
	schedulerURL string
}

// NewLoop creates a retry loop.
func NewLoop(
	tasks repo.TaskRepository,
	instances repo.TaskInstanceRepository,
	router *route.Router,
	dispatcher *dispatch.Client,
	logger *slog.Logger,
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
			l.logger.Debug("retry loop stopped")
			return
		case <-ticker.C:
			l.scan(ctx)
		}
	}
}

func (l *Loop) scan(ctx context.Context) {
	instances, err := l.instances.ListDueRetryInstances(ctx, time.Now(), 100)
	if err != nil {
		l.logger.Warn("list retry_waiting instances failed", "error", err)
		return
	}

	for _, instance := range instances {
		task, err := l.tasks.GetTask(ctx, instance.TaskID)
		if err != nil {
			l.logger.Warn("load task for retry instance failed", "instance_id", instance.ID, "error", err)
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
			l.logger.Warn("pick worker for retry instance failed", "instance_id", instance.ID, "error", err)
			continue
		}

		dispatchTime := time.Now()
		if err := l.instances.UpdateInstanceDispatch(ctx, instance.ID, worker.ID, dispatchTime.Format(time.RFC3339Nano)); err != nil {
			_ = l.router.Release(ctx, worker)
			l.logger.Warn("update retry instance dispatch failed", "instance_id", instance.ID, "error", err)
			continue
		}

		err = l.dispatcher.Dispatch(ctx, worker, model.ExecuteTaskRequest{
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
			l.logger.Warn("dispatch retry instance failed", "instance_id", instance.ID, "error", err)
			continue
		}

		l.logger.Debug("retry instance dispatched", "instance_id", instance.ID, "worker_id", worker.ID, "retry_count", instance.RetryCount)
	}
}
