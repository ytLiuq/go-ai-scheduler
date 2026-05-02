package predictduration

import (
	"testing"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
)

func TestComputeStatsEmpty(t *testing.T) {
	stats := ComputeStats(nil)
	if stats.Count != 0 {
		t.Fatalf("expected 0 count, got %d", stats.Count)
	}
}

func TestComputeStatsEmptyInstances(t *testing.T) {
	stats := ComputeStats([]*model.TaskInstance{})
	if stats.Count != 0 {
		t.Fatalf("expected 0 count, got %d", stats.Count)
	}
}

func TestComputeStatsSkipsRunning(t *testing.T) {
	now := time.Now()
	instances := []*model.TaskInstance{
		{
			Status:       "running",
			DispatchTime: now.Add(-1 * time.Minute),
			UpdatedAt:    now,
		},
		{
			Status:       "dispatched",
			DispatchTime: now.Add(-2 * time.Minute),
			UpdatedAt:    now,
		},
	}
	stats := ComputeStats(instances)
	if stats.Count != 0 {
		t.Fatalf("expected 0 count for running/dispatched, got %d", stats.Count)
	}
}

func TestComputeStatsFromDispatchTime(t *testing.T) {
	now := time.Now()
	instances := []*model.TaskInstance{
		{
			Status:       "success",
			DispatchTime: now.Add(-10 * time.Second),
			UpdatedAt:    now,
		},
		{
			Status:       "failed",
			DispatchTime: now.Add(-20 * time.Second),
			UpdatedAt:    now,
		},
	}
	stats := ComputeStats(instances)
	if stats.Count != 2 {
		t.Fatalf("expected 2 count, got %d", stats.Count)
	}
	if stats.MinDuration < 9 || stats.MinDuration > 11 {
		t.Fatalf("expected min ~10s, got %.1f", stats.MinDuration)
	}
	if stats.MaxDuration < 19 || stats.MaxDuration > 21 {
		t.Fatalf("expected max ~20s, got %.1f", stats.MaxDuration)
	}
}

func TestComputeStatsPrefersStartedAt(t *testing.T) {
	now := time.Now()
	instances := []*model.TaskInstance{
		{
			Status:       "success",
			DispatchTime: now.Add(-30 * time.Second),
			StartedAt:    now.Add(-5 * time.Second),
			FinishedAt:   now,
			UpdatedAt:    now,
		},
	}
	stats := ComputeStats(instances)
	if stats.Count != 1 {
		t.Fatalf("expected 1 count, got %d", stats.Count)
	}
	if stats.MinDuration < 4 || stats.MinDuration > 6 {
		t.Fatalf("expected ~5s from started_at, got %.1f", stats.MinDuration)
	}
}

func TestPercentile(t *testing.T) {
	// p50 of [1,2,3,4,5] = 3
	if p := percentile([]float64{1, 2, 3, 4, 5}, 50); p != 3 {
		t.Fatalf("expected p50=3, got %.1f", p)
	}
	// p95 of 100 elements 1..100 = 95
	sorted := make([]float64, 100)
	for i := range sorted {
		sorted[i] = float64(i + 1)
	}
	if p := percentile(sorted, 95); p != 95 {
		t.Fatalf("expected p95=95, got %.1f", p)
	}
}

func TestPercentileEdgeCases(t *testing.T) {
	if p := percentile(nil, 50); p != 0 {
		t.Fatalf("expected 0 for nil, got %.1f", p)
	}
	if p := percentile([]float64{}, 50); p != 0 {
		t.Fatalf("expected 0 for empty, got %.1f", p)
	}
	// Single element.
	if p := percentile([]float64{42}, 99); p != 42 {
		t.Fatalf("expected 42 for single element, got %.1f", p)
	}
}

func TestPredictWithLLMRequiresLLM(t *testing.T) {
	_, err := PredictWithLLM(t.Context(), nil, &model.Task{}, DurationStats{Count: 5})
	if err != ErrLLMRequired {
		t.Fatalf("expected ErrLLMRequired, got %v", err)
	}
}
