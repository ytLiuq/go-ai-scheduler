package engine

import "time"

// heapItem is an entry in the min-heap, ordered by trigger time.
type heapItem struct {
	TaskID      int64
	TriggerTime time.Time
}

// taskHeap is a min-heap of trigger times.
type taskHeap []heapItem

func (h taskHeap) Len() int           { return len(h) }
func (h taskHeap) Less(i, j int) bool { return h[i].TriggerTime.Before(h[j].TriggerTime) }
func (h taskHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *taskHeap) Push(x any) {
	*h = append(*h, x.(heapItem))
}

func (h *taskHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// Peek returns the earliest trigger time without removing it.
func (h taskHeap) Peek() (heapItem, bool) {
	if len(h) == 0 {
		return heapItem{}, false
	}
	return h[0], true
}

// PopUntil returns all items whose trigger time is at or before cutoff.
func (h *taskHeap) PopUntil(cutoff time.Time) []heapItem {
	var result []heapItem
	for len(*h) > 0 {
		item := (*h)[0]
		if item.TriggerTime.After(cutoff) {
			break
		}
		result = append(result, item)
		_ = h.Pop()
	}
	return result
}
