package trigger

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/rpc"
	"github.com/example/go-ai-scheduler/internal/repo"
	"github.com/example/go-ai-scheduler/internal/scheduler/dispatch"
	"github.com/example/go-ai-scheduler/internal/scheduler/route"
)

// Loop scans due tasks and generates task instances.
type Loop struct {
	taskRepo     repo.TaskRepository
	instanceRepo repo.TaskInstanceRepository
	router       *route.Router
	dispatcher   *dispatch.Client
	logger       *loggerAdapter
	interval     time.Duration
	schedulerURL string
}

type loggerAdapter struct {
	printf func(string, ...any)
}

// NewLoop creates a trigger loop with a fixed scan interval.
func NewLoop(
	taskRepo repo.TaskRepository,
	instanceRepo repo.TaskInstanceRepository,
	router *route.Router,
	dispatcher *dispatch.Client,
	l *log.Logger,
	interval time.Duration,
	schedulerURL string,
) *Loop {
	if interval <= 0 {
		interval = time.Second
	}
	return &Loop{
		taskRepo:     taskRepo,
		instanceRepo: instanceRepo,
		router:       router,
		dispatcher:   dispatcher,
		logger:       &loggerAdapter{printf: l.Printf},
		interval:     interval,
		schedulerURL: schedulerURL,
	}
}

// Start runs the scan loop until the context is cancelled.
func (l *Loop) Start(ctx context.Context) {
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()

	l.scan(ctx)

	for {
		select {
		case <-ctx.Done():
			l.logger.printf("trigger loop stopped")
			return
		case <-ticker.C:
			l.scan(ctx)
		}
	}
}

func (l *Loop) scan(ctx context.Context) {
	tasks, err := l.taskRepo.ListDueTasks(ctx, 100)
	if err != nil {
		l.logger.printf("list due tasks failed: %v", err)
		return
	}

	for _, task := range tasks {
		if err := l.handleTask(ctx, task); err != nil {
			l.logger.printf("handle due task failed task_id=%d err=%v", task.ID, err)
		}
	}
}

func (l *Loop) handleTask(ctx context.Context, task *model.Task) error {
	instance := &model.TaskInstance{
		TaskID:             task.ID,
		ScheduleInstanceID: scheduleInstanceID(task.ID),
		TriggerTime:        time.Now(),
		Status:             "pending",
	}
	if err := l.instanceRepo.CreateInstance(ctx, instance); err != nil {
		return fmt.Errorf("create task instance: %w", err)
	}

	worker, err := l.router.PickAndReserveWorker(ctx)
	if err != nil {
		if err == route.ErrNoAvailableWorker {
			task.NextTriggerTime = time.Now().Add(10 * time.Second)
			if updateErr := l.taskRepo.UpdateTask(ctx, task); updateErr != nil {
				return fmt.Errorf("defer next trigger time: %w", updateErr)
			}
			l.logger.printf("no worker available for task_id=%d instance_id=%d", task.ID, instance.ID)
			return nil
		}
		return fmt.Errorf("pick worker: %w", err)
	}

	dispatchTime := time.Now()
	if err := l.instanceRepo.UpdateInstanceDispatch(ctx, instance.ID, worker.ID, dispatchTime.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("update instance dispatch: %w", err)
	}
	if err := l.dispatcher.Dispatch(ctx, worker, rpc.ExecuteTaskRequest{
		ScheduleInstanceID: instance.ScheduleInstanceID,
		TaskID:             task.ID,
		TaskType:           task.Type,
		Payload:            task.Payload,
		TimeoutSeconds:     task.TimeoutSeconds,
		RetryCount:         instance.RetryCount,
		SchedulerURL:       l.schedulerURL,
	}); err != nil {
		_ = l.instanceRepo.UpdateInstanceResult(ctx, instance.ScheduleInstanceID, "failed", "dispatch_failed", err.Error())
		_ = l.router.Release(ctx, worker)
		task.NextTriggerTime = time.Now().Add(10 * time.Second)
		_ = l.taskRepo.UpdateTask(ctx, task)
		return fmt.Errorf("dispatch to worker: %w", err)
	}

	task.NextTriggerTime = nextTriggerTime(task)
	if err := l.taskRepo.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("update next trigger time: %w", err)
	}

	l.logger.printf(
		"task dispatched task_id=%d instance_id=%d worker_id=%s next_trigger=%s",
		task.ID,
		instance.ID,
		worker.ID,
		task.NextTriggerTime.Format(time.RFC3339),
	)
	return nil
}

func scheduleInstanceID(taskID int64) string {
	return fmt.Sprintf("task-%d-%d", taskID, time.Now().UnixNano())
}

func nextTriggerTime(task *model.Task) time.Time {
	// Bootstrap implementation:
	// if cron is absent, mark the task as no longer due by moving it far into the future.
	// if cron exists, reschedule using a fixed 1-minute interval placeholder until a real cron engine is added.
	if task.CronExpr == "" {
		return time.Now().Add(24 * time.Hour * 365)
	}
	return time.Now().Add(time.Minute)
}
