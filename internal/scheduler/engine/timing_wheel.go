package engine

import (
	"sync"
	"time"
)

const (
	defaultTickDuration = 100 * time.Millisecond
	defaultWheelSize    = 600 // 60 seconds with 100ms ticks
)

// slot holds the task IDs scheduled for a specific tick.
type slot struct {
	taskIDs map[int64]struct{}
}

// TimingWheel is a circular buffer of time slots for coarse-grained scheduling.
// Each slot spans one tick duration. Tasks are placed into the slot whose
// position corresponds to (triggerTime / tickDuration) % wheelSize.
type TimingWheel struct {
	mu           sync.Mutex
	slots        []slot
	tickDuration time.Duration
	wheelSize    int
	currentPos   int
}

// NewTimingWheel creates a timing wheel.
func NewTimingWheel(tickDuration time.Duration, wheelSize int) *TimingWheel {
	if tickDuration <= 0 {
		tickDuration = defaultTickDuration
	}
	if wheelSize <= 0 {
		wheelSize = defaultWheelSize
	}
	slots := make([]slot, wheelSize)
	for i := range slots {
		slots[i] = slot{taskIDs: make(map[int64]struct{})}
	}
	return &TimingWheel{
		slots:        slots,
		tickDuration: tickDuration,
		wheelSize:    wheelSize,
	}
}

// Add places a task ID into the slot corresponding to its trigger time.
func (tw *TimingWheel) Add(taskID int64, triggerTime time.Time) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	ticks := int(triggerTime.UnixNano() / tw.tickDuration.Nanoseconds())
	pos := ticks % tw.wheelSize
	tw.slots[pos].taskIDs[taskID] = struct{}{}
}

// Remove deletes a task ID from all slots (used when task is updated/deleted).
func (tw *TimingWheel) Remove(taskID int64) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	for i := range tw.slots {
		delete(tw.slots[i].taskIDs, taskID)
	}
}

// Tick advances the wheel by one position and returns task IDs in the current slot.
func (tw *TimingWheel) Tick() []int64 {
	tw.mu.Lock()
	tw.currentPos = (tw.currentPos + 1) % tw.wheelSize
	cur := tw.slots[tw.currentPos]
	// Extract and clear the slot atomically.
	taskIDs := make([]int64, 0, len(cur.taskIDs))
	for id := range cur.taskIDs {
		taskIDs = append(taskIDs, id)
	}
	tw.slots[tw.currentPos].taskIDs = make(map[int64]struct{})
	tw.mu.Unlock()
	return taskIDs
}

// TickDuration returns the duration of one tick.
func (tw *TimingWheel) TickDuration() time.Duration {
	return tw.tickDuration
}

// SlotSpan returns the total time span covered by the wheel.
func (tw *TimingWheel) SlotSpan() time.Duration {
	return tw.tickDuration * time.Duration(tw.wheelSize)
}
