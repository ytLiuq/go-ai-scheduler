package service

import (
	"context"
	"testing"
	"time"

	"github.com/example/go-ai-scheduler/internal/repo/teststore"
)

func TestTaskServiceCreateTaskComputesNextTriggerFromCron(t *testing.T) {
	svc := NewTaskService(teststore.NewTaskRepository(), nil)

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
	svc := NewTaskService(teststore.NewTaskRepository(), nil)

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

func TestTaskServicePauseResumeTask(t *testing.T) {
	svc := NewTaskService(teststore.NewTaskRepository(), nil)

	task, err := svc.CreateTask(context.Background(), TaskUpsertRequest{
		Name:    "pause-task",
		Type:    "http",
		Payload: "http://example.com",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	paused, err := svc.PauseTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("pause task: %v", err)
	}
	if paused.Status != "disabled" {
		t.Fatalf("expected disabled, got %s", paused.Status)
	}

	resumed, err := svc.ResumeTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("resume task: %v", err)
	}
	if resumed.Status != "enabled" {
		t.Fatalf("expected enabled, got %s", resumed.Status)
	}
}

func TestTaskServiceResumeRecalculatesNextTrigger(t *testing.T) {
	svc := NewTaskService(teststore.NewTaskRepository(), nil)

	task, err := svc.CreateTask(context.Background(), TaskUpsertRequest{
		Name:    "cron-resume-task",
		Type:    "http",
		CronExpr: "*/5 * * * *",
		Payload: "http://example.com",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Pause it.
	if _, err := svc.PauseTask(context.Background(), task.ID); err != nil {
		t.Fatalf("pause task: %v", err)
	}

	// Resume should recalculate next_trigger_time.
	resumed, err := svc.ResumeTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("resume task: %v", err)
	}
	if resumed.NextTriggerTime.IsZero() {
		t.Fatalf("expected non-zero next trigger time after resume")
	}
	if resumed.NextTriggerTime.Before(task.NextTriggerTime) {
		t.Fatalf("expected new trigger time to be in the future relative to original")
	}
}

func TestTaskServiceTriggerTask(t *testing.T) {
	svc := NewTaskService(teststore.NewTaskRepository(), nil)

	task, err := svc.CreateTask(context.Background(), TaskUpsertRequest{
		Name:    "trigger-task",
		Type:    "http",
		Payload: "http://example.com",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	triggered, err := svc.TriggerTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("trigger task: %v", err)
	}
	if !triggered.NextTriggerTime.Before(time.Now()) {
		t.Fatalf("expected next_trigger_time to be in the past after manual trigger")
	}
}

func TestTaskServiceTriggerDisabledTaskFails(t *testing.T) {
	svc := NewTaskService(teststore.NewTaskRepository(), nil)

	task, err := svc.CreateTask(context.Background(), TaskUpsertRequest{
		Name:    "disabled-trigger-task",
		Type:    "http",
		Payload: "http://example.com",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := svc.PauseTask(context.Background(), task.ID); err != nil {
		t.Fatalf("pause task: %v", err)
	}

	_, err = svc.TriggerTask(context.Background(), task.ID)
	if err == nil {
		t.Fatalf("expected error when triggering a disabled task")
	}
}

func TestRetryDelayFixedInterval(t *testing.T) {
	if d := retryDelay("fixed_interval", 3, 0); d != 0 {
		t.Fatalf("expected 0 delay for fixed_interval, got %v", d)
	}
}

func TestRetryDelayExponentialBackoff(t *testing.T) {
	tests := []struct {
		retryCount int
		expected   time.Duration
	}{
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
	}
	for _, tc := range tests {
		d := retryDelay("exponential_backoff", tc.retryCount, 0)
		if d != tc.expected {
			t.Fatalf("retry_count=%d: expected %v, got %v", tc.retryCount, tc.expected, d)
		}
	}
}

func TestRetryDelayExponentialBackoffCap(t *testing.T) {
	d := retryDelay("exponential_backoff", 20, 0)
	if d != 10*time.Minute {
		t.Fatalf("expected cap at 10m, got %v", d)
	}
}

func TestShouldRetryOnErrors(t *testing.T) {
	// Without error_code policy, always retry.
	if !shouldRetryOnErrors("fixed_interval", "", "E001") {
		t.Fatal("expected retry for non-error_code policy")
	}

	// With error_code policy and matching code.
	if !shouldRetryOnErrors("error_code", "E001,E002", "E001") {
		t.Fatal("expected retry for matching error code")
	}

	// With error_code policy and non-matching code.
	if shouldRetryOnErrors("error_code", "E001,E002", "E003") {
		t.Fatal("expected no retry for non-matching error code")
	}

	// With error_code policy and empty retry list.
	if !shouldRetryOnErrors("error_code", "", "E001") {
		t.Fatal("expected retry when retry_on_errors is empty")
	}
}
