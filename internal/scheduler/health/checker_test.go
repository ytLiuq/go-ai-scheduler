package health

import (
	"context"
	"testing"
	"time"

	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/pkg/logger"
	"github.com/example/go-ai-scheduler/internal/repo/teststore"
)

func TestCheckerEvictsStaleWorker(t *testing.T) {
	repo := teststore.NewWorkerRepository()
	svc := apiservice.NewWorkerService(repo)

	// Register an online worker with an old heartbeat.
	_, err := svc.RegisterWorker(context.Background(), apiservice.WorkerRegistrationRequest{
		WorkerID:       "stale-worker",
		Hostname:       "stale-host",
		MaxConcurrency: 10,
	})
	if err != nil {
		t.Fatalf("register worker: %v", err)
	}

	// Manually set the heartbeat far in the past.
	w, _ := repo.GetWorker(context.Background(), "stale-worker")
	w.LastHeartbeatAt = time.Now().Add(-2 * time.Minute)
	if err := repo.UpsertWorker(context.Background(), w); err != nil {
		t.Fatalf("upsert stale worker: %v", err)
	}

	// Also register a healthy worker.
	_, err = svc.RegisterWorker(context.Background(), apiservice.WorkerRegistrationRequest{
		WorkerID:       "healthy-worker",
		Hostname:       "healthy-host",
		MaxConcurrency: 10,
	})
	if err != nil {
		t.Fatalf("register healthy worker: %v", err)
	}

	// Evict with a 1-minute timeout — only the stale worker should be evicted.
	count, err := svc.EvictStaleWorkers(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("evict stale workers: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 eviction, got %d", count)
	}

	staleWorker, err := repo.GetWorker(context.Background(), "stale-worker")
	if err != nil {
		t.Fatalf("get stale worker: %v", err)
	}
	if staleWorker.Status != "offline" {
		t.Fatalf("expected stale worker status=offline, got %s", staleWorker.Status)
	}
	if staleWorker.CurrentLoad != 0 {
		t.Fatalf("expected stale worker load=0, got %d", staleWorker.CurrentLoad)
	}

	healthyWorker, err := repo.GetWorker(context.Background(), "healthy-worker")
	if err != nil {
		t.Fatalf("get healthy worker: %v", err)
	}
	if healthyWorker.Status != "online" {
		t.Fatalf("expected healthy worker status=online, got %s", healthyWorker.Status)
	}
}

func TestCheckerStartStop(t *testing.T) {
	repo := teststore.NewWorkerRepository()
	svc := apiservice.NewWorkerService(repo)
	logr := logger.New("health-loop-test")

	checker := NewChecker(svc, logr, 50*time.Millisecond, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Register a worker with a very old heartbeat.
	_, err := svc.RegisterWorker(context.Background(), apiservice.WorkerRegistrationRequest{
		WorkerID:       "loop-stale",
		Hostname:       "loop-host",
		MaxConcurrency: 10,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	w, _ := repo.GetWorker(context.Background(), "loop-stale")
	w.LastHeartbeatAt = time.Now().Add(-time.Hour)
	if err := repo.UpsertWorker(context.Background(), w); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	checker.Start(ctx)

	// Wait for the context to expire (checker will stop then).
	<-ctx.Done()

	// The stale worker should have been evicted.
	stale, err := repo.GetWorker(context.Background(), "loop-stale")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stale.Status != "offline" {
		t.Fatalf("expected offline, got %s", stale.Status)
	}
}
