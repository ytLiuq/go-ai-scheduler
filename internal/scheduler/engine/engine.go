package engine

import (
	"container/heap"
	"context"
	"log"
	"sync"
	"time"

	"github.com/example/go-ai-scheduler/internal/repo"
)

// Engine is a hybrid scheduler that combines a timing wheel for periodic tasks
// and a min-heap for short-term and retry tasks, with Redis caching.
type Engine struct {
	wheel   *TimingWheel
	heap    *taskHeap
	mu      sync.Mutex

	taskRepo     repo.TaskRepository
	instanceRepo repo.TaskInstanceRepository
	logger       *log.Logger

	// Callback invoked for each task that is due.
	OnTrigger func(taskID int64)
}

// New creates a new hybrid scheduling engine.
func New(taskRepo repo.TaskRepository, instanceRepo repo.TaskInstanceRepository, l *log.Logger) *Engine {
	return &Engine{
		wheel:        NewTimingWheel(100*time.Millisecond, 600),
		heap:         &taskHeap{},
		taskRepo:     taskRepo,
		instanceRepo: instanceRepo,
		logger:       l,
	}
}

// AddToWheel schedules a task into the timing wheel for coarse-grained triggering.
func (e *Engine) AddToWheel(taskID int64, triggerTime time.Time) {
	e.wheel.Add(taskID, triggerTime)
}

// RemoveFromWheel removes a task from the timing wheel.
func (e *Engine) RemoveFromWheel(taskID int64) {
	e.wheel.Remove(taskID)
}

// AddToHeap pushes a task into the min-heap for precise triggering.
func (e *Engine) AddToHeap(taskID int64, triggerTime time.Time) {
	e.mu.Lock()
	heap.Push(e.heap, heapItem{TaskID: taskID, TriggerTime: triggerTime})
	e.mu.Unlock()
}

// Start runs the hybrid engine loop.
func (e *Engine) Start(ctx context.Context) {
	wheelTicker := time.NewTicker(e.wheel.TickDuration())
	defer wheelTicker.Stop()

	// Scan heap more frequently for precise short-term triggers.
	heapTicker := time.NewTicker(50 * time.Millisecond)
	defer heapTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.logger.Printf("scheduling engine stopped")
			return
		case <-wheelTicker.C:
			e.processWheelTick()
		case <-heapTicker.C:
			e.processHeapTick()
		}
	}
}

func (e *Engine) processWheelTick() {
	taskIDs := e.wheel.Tick()
	for _, id := range taskIDs {
		if e.OnTrigger != nil {
			e.OnTrigger(id)
		}
	}
}

func (e *Engine) processHeapTick() {
	e.mu.Lock()
	items := e.heap.PopUntil(time.Now())
	e.mu.Unlock()

	for _, item := range items {
		if e.OnTrigger != nil {
			e.OnTrigger(item.TaskID)
		}
	}
}

// Warm loads upcoming tasks (due within the wheel's span) into the wheel,
// and retry tasks into the min-heap.
func (e *Engine) Warm(ctx context.Context) error {
	span := e.wheel.SlotSpan()
	cutoff := time.Now().Add(span)

	// Load periodic tasks due within the wheel span.
	tasks, err := e.taskRepo.ListDueTasks(ctx, 500)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.NextTriggerTime.Before(cutoff) {
			e.AddToWheel(t.ID, t.NextTriggerTime)
		}
	}

	// Load retry_waiting instances into the min-heap for precise timing.
	retryInstances, err := e.instanceRepo.ListDueRetryInstances(ctx, cutoff, 500)
	if err != nil {
		e.logger.Printf("engine warm: list retry instances failed: %v", err)
		return nil // non-fatal
	}
	for _, inst := range retryInstances {
		if !inst.NextRetryTime.IsZero() && inst.NextRetryTime.Before(cutoff) {
			e.AddToHeap(inst.TaskID, inst.NextRetryTime)
		}
	}

	return nil
}

// WarmPeriodically re-warms the engine on a fixed interval.
func (e *Engine) WarmPeriodically(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.Warm(ctx); err != nil {
				e.logger.Printf("engine warm failed: %v", err)
			}
		}
	}
}
