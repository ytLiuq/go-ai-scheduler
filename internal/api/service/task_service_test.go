package service

import (
	"context"
	"testing"

	"github.com/example/go-ai-scheduler/internal/repo/memory"
)

func TestTaskServiceCreateTaskComputesNextTriggerFromCron(t *testing.T) {
	svc := NewTaskService(memory.NewTaskRepository())

	task, err := svc.CreateTask(context.Background(), TaskUpsertRequest{
		Name:     "cron-task",
		Type:     "http",
		CronExpr: "*/10 * * * *",
		Payload:  "http://example.com",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.NextTriggerTime.IsZero() {
		t.Fatalf("expected computed next trigger time")
	}
}

func TestTaskServiceDeleteTaskRemovesTask(t *testing.T) {
	svc := NewTaskService(memory.NewTaskRepository())

	task, err := svc.CreateTask(context.Background(), TaskUpsertRequest{
		Name:    "delete-task",
		Type:    "http",
		Payload: "http://example.com",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := svc.DeleteTask(context.Background(), task.ID); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	if _, err := svc.GetTask(context.Background(), task.ID); err == nil {
		t.Fatalf("expected deleted task lookup to fail")
	}
}
