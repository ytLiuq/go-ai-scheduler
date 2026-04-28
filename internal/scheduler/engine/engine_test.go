package engine

import (
	"context"
	"testing"
	"time"
)

func TestTimingWheelAddAndTick(t *testing.T) {
	tw := NewTimingWheel(10*time.Millisecond, 10)
	now := time.Now()

	// Add task and verify it's stored in some slot.
	tw.Add(42, now.Add(20*time.Millisecond))
	tw.mu.Lock()
	found := false
	for i := range tw.slots {
		if _, ok := tw.slots[i].taskIDs[42]; ok {
			found = true
			break
		}
	}
	tw.mu.Unlock()
	if !found {
		t.Fatal("task 42 should be in some slot")
	}
}

func TestTimingWheelRemove(t *testing.T) {
	tw := NewTimingWheel(10*time.Millisecond, 10)
	tw.Add(1, time.Now().Add(10*time.Millisecond))
	tw.Remove(1)

	tw.mu.Lock()
	for i := range tw.slots {
		if _, ok := tw.slots[i].taskIDs[1]; ok {
			tw.mu.Unlock()
			t.Fatal("task 1 should have been removed")
		}
	}
	tw.mu.Unlock()
}

func TestTimingWheelMultipleTicks(t *testing.T) {
	tw := NewTimingWheel(10*time.Millisecond, 100)
	now := time.Now()
	tw.Add(99, now.Add(50*time.Millisecond))

	found := false
	for i := 0; i < 100; i++ {
		for _, id := range tw.Tick() {
			if id == 99 {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("task 99 should be found after ticking")
	}
}

func TestTimingWheelSpan(t *testing.T) {
	tw := NewTimingWheel(100*time.Millisecond, 600)
	if tw.TickDuration() != 100*time.Millisecond {
		t.Fatal("wrong tick duration")
	}
	if tw.SlotSpan() != 60*time.Second {
		t.Fatal("wrong slot span")
	}
}

func TestHeapPeekAndLen(t *testing.T) {
	h := &taskHeap{}
	now := time.Now()

	h.Push(heapItem{TaskID: 1, TriggerTime: now})
	h.Push(heapItem{TaskID: 2, TriggerTime: now.Add(1 * time.Second)})

	item, ok := h.Peek()
	if !ok || item.TaskID != 1 {
		t.Fatalf("expected task 1 at peek, got %+v", item)
	}
	if h.Len() != 2 {
		t.Fatalf("expected len 2, got %d", h.Len())
	}
}

func TestPopUntilWithSortedItems(t *testing.T) {
	h := &taskHeap{}
	now := time.Now()

	h.Push(heapItem{TaskID: 1, TriggerTime: now.Add(-1 * time.Second)})
	h.Push(heapItem{TaskID: 2, TriggerTime: now})
	h.Push(heapItem{TaskID: 3, TriggerTime: now.Add(10 * time.Second)})

	items := h.PopUntil(now.Add(5 * time.Second))
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(items), items)
	}
	if items[0].TaskID != 1 || items[1].TaskID != 2 {
		t.Fatalf("expected tasks [1,2], got %+v", items)
	}
	if h.Len() != 1 || (*h)[0].TaskID != 3 {
		t.Fatalf("expected task 3 remaining, len=%d", h.Len())
	}
}

func TestPopUntilEmpty(t *testing.T) {
	h := &taskHeap{}
	items := h.PopUntil(time.Now())
	if len(items) != 0 {
		t.Fatal("expected empty")
	}
}

func TestEngineNew(t *testing.T) {
	eng := New(nil, nil, nil)
	if eng == nil {
		t.Fatal("engine should not be nil")
	}
}

func TestEngineStartStop(t *testing.T) {
	eng := New(nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		eng.Start(ctx)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("engine did not stop after context cancel")
	}
}
