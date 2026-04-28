package advisor

import (
	"testing"
)

func TestHeuristicGenerateHealthy(t *testing.T) {
	ctx := Context{
		AvgWorkerLoad:     0.3,
		TotalWorkers:      5,
		OnlineWorkers:     5,
		PendingInstances:  10,
		FailedLastHour:    2,
		MaxPendingConfig:  1000,
	}
	advices := heuristicGenerate(ctx)
	if len(advices) == 0 {
		t.Fatal("expected at least one advice (system healthy)")
	}
	if advices[0].Type != "info" {
		t.Fatalf("expected info type, got %s", advices[0].Type)
	}
}

func TestHeuristicGenerateWorkersOffline(t *testing.T) {
	ctx := Context{
		AvgWorkerLoad:    0.3,
		TotalWorkers:     5,
		OnlineWorkers:    2,
		PendingInstances: 10,
		MaxPendingConfig: 1000,
	}
	advices := heuristicGenerate(ctx)
	hasScale := false
	for _, a := range advices {
		if a.Type == "scale" {
			hasScale = true
			break
		}
	}
	if !hasScale {
		t.Fatal("expected scale advice for offline workers")
	}
}

func TestHeuristicGenerateHighLoad(t *testing.T) {
	ctx := Context{
		AvgWorkerLoad:    0.9,
		TotalWorkers:     5,
		OnlineWorkers:    5,
		MaxPendingConfig: 1000,
	}
	advices := heuristicGenerate(ctx)
	hasScale := false
	for _, a := range advices {
		if a.Type == "scale" && a.Title == "High worker load" {
			hasScale = true
			break
		}
	}
	if !hasScale {
		t.Fatal("expected scale advice for high load")
	}
}

func TestHeuristicGenerateBackpressure(t *testing.T) {
	ctx := Context{
		AvgWorkerLoad:    0.5,
		OnlineWorkers:    3,
		TotalWorkers:     3,
		PendingInstances: 900,
		MaxPendingConfig: 1000,
	}
	advices := heuristicGenerate(ctx)
	hasThrottle := false
	for _, a := range advices {
		if a.Type == "throttle" {
			hasThrottle = true
			break
		}
	}
	if !hasThrottle {
		t.Fatal("expected throttle advice for high pending")
	}
}

func TestHeuristicGenerateHighFailures(t *testing.T) {
	ctx := Context{
		AvgWorkerLoad:    0.3,
		OnlineWorkers:    3,
		TotalWorkers:     3,
		FailedLastHour:   20,
		MaxPendingConfig: 1000,
	}
	advices := heuristicGenerate(ctx)
	hasConfig := false
	for _, a := range advices {
		if a.Type == "config" && a.Title == "Elevated failure rate" {
			hasConfig = true
			break
		}
	}
	if !hasConfig {
		t.Fatal("expected config advice for failures")
	}
}

func TestGenerateFallback(t *testing.T) {
	// No LLM — should fall back to heuristic.
	advices, err := Generate(t.Context(), nil, Context{
		OnlineWorkers:    1,
		TotalWorkers:     3,
		MaxPendingConfig: 1000,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(advices) == 0 {
		t.Fatal("expected at least one advice")
	}
}
