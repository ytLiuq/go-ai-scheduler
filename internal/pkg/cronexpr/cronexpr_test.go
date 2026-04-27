package cronexpr

import (
	"testing"
	"time"
)

func TestNextAfterStepMinute(t *testing.T) {
	now := time.Date(2026, time.April, 27, 14, 3, 24, 0, time.UTC)

	next, err := NextAfter(now, "*/5 * * * *")
	if err != nil {
		t.Fatalf("next after: %v", err)
	}

	expected := time.Date(2026, time.April, 27, 14, 5, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Fatalf("unexpected next time: got %s want %s", next, expected)
	}
}

func TestNextAfterDayOfWeekConstraint(t *testing.T) {
	now := time.Date(2026, time.April, 27, 14, 3, 24, 0, time.UTC) // Monday

	next, err := NextAfter(now, "15 9 * * 2")
	if err != nil {
		t.Fatalf("next after: %v", err)
	}

	expected := time.Date(2026, time.April, 28, 9, 15, 0, 0, time.UTC) // Tuesday
	if !next.Equal(expected) {
		t.Fatalf("unexpected next time: got %s want %s", next, expected)
	}
}

func TestValidateRejectsInvalidExpr(t *testing.T) {
	if err := Validate("* * *"); err == nil {
		t.Fatalf("expected invalid cron expression error")
	}
}
