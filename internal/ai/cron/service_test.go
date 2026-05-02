package cron

import (
	"testing"
	"time"
)

func TestNextRunValid(t *testing.T) {
	resp, err := NextRun("*/5 * * * *", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("next run: %v", err)
	}
	if resp.Expression != "*/5 * * * *" {
		t.Fatal("wrong expression")
	}
	if resp.NextRun.IsZero() {
		t.Fatal("next run should not be zero")
	}
}

func TestNextRunInvalidExpr(t *testing.T) {
	_, err := NextRun("not-a-cron", time.Now())
	if err == nil {
		t.Fatal("expected error for invalid cron")
	}
}
