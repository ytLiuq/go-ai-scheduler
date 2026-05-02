package trend

import (
	"testing"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
)

func TestComputeSnapshotEmpty(t *testing.T) {
	snap := ComputeSnapshot(nil, nil, nil, 24)
	if snap.WorkerCount != 0 {
		t.Fatalf("expected 0 workers, got %d", snap.WorkerCount)
	}
	if snap.TaskCount != 0 {
		t.Fatalf("expected 0 tasks, got %d", snap.TaskCount)
	}
	if snap.TotalInstances != 0 {
		t.Fatalf("expected 0 instances, got %d", snap.TotalInstances)
	}
	if snap.TimeWindowHours != 24 {
		t.Fatalf("expected 24h window, got %d", snap.TimeWindowHours)
	}
}

func TestComputeSnapshotWorkers(t *testing.T) {
	workers := []*model.WorkerNode{
		{ID: "w1", Status: "online", CurrentLoad: 5, MaxConcurrency: 10},
		{ID: "w2", Status: "online", CurrentLoad: 1, MaxConcurrency: 10},
		{ID: "w3", Status: "offline", CurrentLoad: 0, MaxConcurrency: 5},
	}
	snap := ComputeSnapshot(workers, nil, nil, 1)
	if snap.WorkerCount != 3 {
		t.Fatalf("expected 3 total, got %d", snap.WorkerCount)
	}
	if snap.OnlineWorkers != 2 {
		t.Fatalf("expected 2 online, got %d", snap.OnlineWorkers)
	}
	if snap.AvgWorkerLoad < 0.29 || snap.AvgWorkerLoad > 0.31 {
		t.Fatalf("expected avg load ~0.30, got %.3f", snap.AvgWorkerLoad)
	}
}

func TestComputeSnapshotTasks(t *testing.T) {
	tasks := []*model.Task{
		{ID: 1, Status: "enabled"},
		{ID: 2, Status: "enabled"},
		{ID: 3, Status: "disabled"},
	}
	snap := ComputeSnapshot(nil, tasks, nil, 1)
	if snap.TaskCount != 3 {
		t.Fatalf("expected 3 tasks, got %d", snap.TaskCount)
	}
	if snap.EnabledTasks != 2 {
		t.Fatalf("expected 2 enabled, got %d", snap.EnabledTasks)
	}
}

func TestComputeSnapshotTimeWindow(t *testing.T) {
	now := time.Now()
	instances := []*model.TaskInstance{
		{Status: "failed", CreatedAt: now.Add(-30 * time.Minute)},
		{Status: "success", CreatedAt: now.Add(-2 * time.Hour)},
		{Status: "pending", CreatedAt: now.Add(-10 * time.Minute)},
	}
	// 1 hour window: only the 30min-old failed and 10min-old pending should be included.
	snap := ComputeSnapshot(nil, nil, instances, 1)
	if snap.TotalInstances != 2 {
		t.Fatalf("expected 2 within 1h window, got %d", snap.TotalInstances)
	}
	if snap.FailedInstances != 1 {
		t.Fatalf("expected 1 failed, got %d", snap.FailedInstances)
	}
	if snap.PendingInstances != 1 {
		t.Fatalf("expected 1 pending, got %d", snap.PendingInstances)
	}
}

func TestComputeSnapshotAllTime(t *testing.T) {
	now := time.Now()
	instances := []*model.TaskInstance{
		{Status: "failed", CreatedAt: now.Add(-48 * time.Hour)},
		{Status: "success", CreatedAt: now.Add(-72 * time.Hour)},
	}
	// timeWindowHours=0 means all time.
	snap := ComputeSnapshot(nil, nil, instances, 0)
	if snap.TotalInstances != 2 {
		t.Fatalf("expected 2 (all time), got %d", snap.TotalInstances)
	}
}

func TestAnalyzeWithLLMRequiresLLM(t *testing.T) {
	_, err := AnalyzeWithLLM(t.Context(), nil, DataSnapshot{})
	if err != ErrLLMRequired {
		t.Fatalf("expected ErrLLMRequired, got %v", err)
	}
}
