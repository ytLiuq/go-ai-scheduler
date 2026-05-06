package worker

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
)

func TestNewStore(t *testing.T) {
	dir := os.TempDir()
	rep := NewReportClient("http", "")
	l := slog.New(slog.NewTextHandler(os.Stderr, nil))
	store, err := NewStore(dir, rep, l)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if store.PendingCount() != 0 {
		t.Fatal("expected 0 pending on new store")
	}
}

func TestBufferAndRemove(t *testing.T) {
	dir := t.TempDir()
	rep := NewReportClient("http", "")
	l := slog.New(slog.NewTextHandler(os.Stderr, nil))
	store, err := NewStore(dir, rep, l)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	sid := "task-1-12345"
	store.Buffer("http://localhost:8080", apiservice.TaskStatusReportRequest{
		ScheduleInstanceID: sid,
		WorkerID:           "worker-1",
		Status:             "success",
	})

	if store.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", store.PendingCount())
	}

	store.Remove(sid)
	if store.PendingCount() != 0 {
		t.Fatalf("expected 0 after remove, got %d", store.PendingCount())
	}
}

func TestFlushWithNoServer(t *testing.T) {
	dir := t.TempDir()
	rep := NewReportClient("http", "")
	l := slog.New(slog.NewTextHandler(os.Stderr, nil))
	store, err := NewStore(dir, rep, l)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	sid := "task-1-99999"
	store.Buffer("http://127.0.0.1:19999", apiservice.TaskStatusReportRequest{
		ScheduleInstanceID: sid,
		WorkerID:           "worker-1",
		Status:             "failed",
		ErrorCode:          "test",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Flush should fail (no server) but not crash.
	remaining := store.Flush(ctx)
	if remaining != 1 {
		t.Fatalf("expected 1 remaining after failed flush, got %d", remaining)
	}
}

func TestStartFlushLoop(t *testing.T) {
	dir := t.TempDir()
	rep := NewReportClient("http", "")
	l := slog.New(slog.NewTextHandler(os.Stderr, nil))
	store, err := NewStore(dir, rep, l)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go store.StartFlushLoop(ctx, 100*time.Millisecond)

	time.Sleep(200 * time.Millisecond)
	cancel()
	// Should stop cleanly.
	time.Sleep(50 * time.Millisecond)
}
