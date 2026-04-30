package trigger

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/pkg/cronexpr"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	"github.com/example/go-ai-scheduler/internal/repo"
	"github.com/example/go-ai-scheduler/internal/rpc"
	schedulercache "github.com/example/go-ai-scheduler/internal/scheduler/cache"
	"github.com/example/go-ai-scheduler/internal/scheduler/dispatch"
	"github.com/example/go-ai-scheduler/internal/scheduler/ratelimit"
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
	maxPending   int
	cache        *schedulercache.Manager
	bp           *ratelimit.BackpressureController
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
	maxPending int,
	cache *schedulercache.Manager,
	bp *ratelimit.BackpressureController,
) *Loop {
	if interval <= 0 {
		interval = time.Second
	}
	if maxPending <= 0 {
		maxPending = 1000
	}
	return &Loop{
		taskRepo:     taskRepo,
		instanceRepo: instanceRepo,
		router:       router,
		dispatcher:   dispatcher,
		logger:       &loggerAdapter{printf: l.Printf},
		interval:     interval,
		schedulerURL: schedulerURL,
		maxPending:   maxPending,
		cache:        cache,
		bp:           bp,
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
	pending, err := l.instanceRepo.CountInstancesByStatus(ctx, "pending")
	if err != nil {
		l.logger.printf("count pending instances failed: %v", err)
		return
	}

	// Update backpressure controller with current pending count.
	if l.bp != nil {
		l.bp.UpdatePending(ctx, pending)
		if !l.bp.AllowDispatch() {
			l.logger.printf("backpressure: state=%s pending=%d, rejecting scan", l.bp.State().String(), pending)
			return
		}
		if delay := l.bp.ThrottleDelay(); delay > 0 {
			time.Sleep(delay)
		}
	}

	var tasks []*model.Task

	if l.cache != nil && l.cache.Enabled() {
		cachedIDs, cacheErr := l.cache.GetCachedDueTaskIDs(ctx, time.Now())
		if cacheErr == nil && len(cachedIDs) > 0 {
			tasks = make([]*model.Task, 0, len(cachedIDs))
			for _, id := range cachedIDs {
				t, err := l.taskRepo.GetTask(ctx, id)
				if err != nil {
					continue
				}
				if t.Status == "enabled" && !t.NextTriggerTime.After(time.Now()) {
					tasks = append(tasks, t)
				}
			}
			if len(tasks) > 100 {
				tasks = tasks[:100]
			}
		}
	}

	if len(tasks) == 0 {
		var err error
		tasks, err = l.taskRepo.ListDueTasks(ctx, 100)
		if err != nil {
			l.logger.printf("list due tasks failed: %v", err)
			return
		}
	}

	for _, task := range tasks {
		if l.bp != nil && !l.bp.AllowDispatch() {
			l.logger.printf("backpressure triggered mid-scan, remaining tasks deferred")
			return
		}
		if err := l.handleTask(ctx, task); err != nil {
			l.logger.printf("handle due task failed task_id=%d err=%v", task.ID, err)
		}
	}
}

func (l *Loop) handleTask(ctx context.Context, task *model.Task) error {
	// Check dependencies before triggering.
	upstream, err := l.taskRepo.ListUpstreamDeps(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("list upstream dependencies: %w", err)
	}
	if len(upstream) > 0 {
		// Check if all upstream tasks have recent successful instances.
		allSatisfied := true
		for _, depID := range upstream {
			instances, err := l.instanceRepo.ListInstancesByStatus(ctx, "success", 1)
			if err != nil || len(instances) == 0 {
				allSatisfied = false
				break
			}
			found := false
			for _, inst := range instances {
				if inst.TaskID == depID {
					found = true
					break
				}
			}
			if !found {
				allSatisfied = false
				break
			}
		}
		if !allSatisfied {
			// Defer this task; dependencies not yet met.
			task.NextTriggerTime = time.Now().Add(5 * time.Second)
			_ = l.taskRepo.UpdateTask(ctx, task)
			l.logger.printf("task_id=%d deferred: upstream dependencies not satisfied", task.ID)
			return nil
		}
	}

	shardTotal := task.TotalShards
	if shardTotal <= 0 {
		shardTotal = 1
	}

	for shard := 0; shard < shardTotal; shard++ {
		instance := &model.TaskInstance{
			TaskID:             task.ID,
			ScheduleInstanceID: scheduleInstanceID(task.ID),
			ShardNo:            shard,
			ShardTotal:         task.TotalShards,
			TriggerTime:        time.Now(),
			Status:             "pending",
		}
		if err := l.instanceRepo.CreateInstance(ctx, instance); err != nil {
			return fmt.Errorf("create task instance shard=%d: %w", shard, err)
		}

		worker, err := l.router.Pick(ctx, route.SelectOptions{
			Labels:   model.DecodeLabels(task.Labels),
			Strategy: task.RouteStrategy,
		})
		if err != nil {
			if err == route.ErrNoAvailableWorker {
				task.NextTriggerTime = time.Now().Add(10 * time.Second)
				_ = l.taskRepo.UpdateTask(ctx, task)
				l.logger.printf("no worker for task_id=%d shard=%d", task.ID, shard)
				return nil
			}
			return fmt.Errorf("pick worker shard=%d: %w", shard, err)
		}

		dispatchTime := time.Now()
		_ = l.instanceRepo.UpdateInstanceDispatch(ctx, instance.ID, worker.ID, dispatchTime.Format(time.RFC3339Nano))
		if err := l.dispatcher.Dispatch(ctx, worker, rpc.ExecuteTaskRequest{
			ScheduleInstanceID: instance.ScheduleInstanceID,
			TaskID:             task.ID,
			TaskType:           task.Type,
			Payload:            task.Payload,
			Image:              task.Image,
			TimeoutSeconds:     task.TimeoutSeconds,
			RetryCount:         instance.RetryCount,
			ShardNo:            shard,
			ShardTotal:         task.TotalShards,
			IdempotencyKey:     task.IdempotencyKey,
			SchedulerURL:       l.schedulerURL,
		}); err != nil {
			metrics.DefaultRegistry.IncCounter("scheduler_dispatch_total", map[string]string{"result": "error"})
			_ = l.instanceRepo.UpdateInstanceResult(ctx, instance.ScheduleInstanceID, "failed", "dispatch_failed", err.Error())
			_ = l.router.Release(ctx, worker)
			continue
		}
		metrics.DefaultRegistry.IncCounter("scheduler_dispatch_total", map[string]string{"result": "success"})

		l.logger.printf(
			"task dispatched task_id=%d instance_id=%d worker_id=%s shard=%d/%d",
			task.ID, instance.ID, worker.ID, shard, shardTotal,
		)
	}

	nextTrigger, err := nextTriggerTime(task, time.Now())
	if err != nil {
		return fmt.Errorf("compute next trigger time: %w", err)
	}
	task.NextTriggerTime = nextTrigger
	if err := l.taskRepo.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("update next trigger time: %w", err)
	}
	return nil
}

func scheduleInstanceID(taskID int64) string {
	return fmt.Sprintf("task-%d-%d", taskID, time.Now().UnixNano())
}

func nextTriggerTime(task *model.Task, now time.Time) (time.Time, error) {
	if task.CronExpr == "" {
		return now.Add(24 * time.Hour * 365), nil
	}
	return cronexpr.NextAfter(now, task.CronExpr)
}
