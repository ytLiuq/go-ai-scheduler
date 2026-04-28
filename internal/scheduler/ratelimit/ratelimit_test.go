package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestTokenBucketAllow(t *testing.T) {
	tb := NewTokenBucket(10, 10)
	// Consume all tokens.
	for i := 0; i < 10; i++ {
		if !tb.Allow() {
			t.Fatalf("expected allow at token %d", i)
		}
	}
	if tb.Allow() {
		t.Fatal("expected reject when empty")
	}
}

func TestTokenBucketRefill(t *testing.T) {
	tb := NewTokenBucket(100, 10)
	// Exhaust.
	for i := 0; i < 10; i++ {
		tb.Allow()
	}
	if tb.Allow() {
		t.Fatal("should be empty")
	}
	// Wait for refill.
	time.Sleep(50 * time.Millisecond)
	if !tb.Allow() {
		t.Fatal("should have refilled at least 1 token after 50ms at rate=100/s")
	}
}

func TestTokenBucketAllowN(t *testing.T) {
	tb := NewTokenBucket(10, 10)
	if !tb.AllowN(5) {
		t.Fatal("expected allow 5")
	}
	if !tb.AllowN(5) {
		t.Fatal("expected allow remaining 5")
	}
	if tb.AllowN(1) {
		t.Fatal("expected reject when empty")
	}
}

func TestTokenBucketWaitTime(t *testing.T) {
	tb := NewTokenBucket(10, 10)
	for i := 0; i < 10; i++ {
		tb.Allow()
	}
	wt := tb.WaitTime(1)
	if wt <= 0 {
		t.Fatal("expected positive wait time when empty")
	}
}

func TestConcurrencyLimiter(t *testing.T) {
	cl := NewConcurrencyLimiter(3)
	if cl.Current() != 0 {
		t.Fatal("initial count should be 0")
	}
	cl.Acquire()
	cl.Acquire()
	cl.Acquire()
	if cl.Current() != 3 {
		t.Fatalf("expected 3, got %d", cl.Current())
	}

	// Release should allow another acquire.
	cl.Release()
	if cl.Current() != 2 {
		t.Fatalf("expected 2 after release, got %d", cl.Current())
	}
	cl.Acquire()
	if cl.Current() != 3 {
		t.Fatalf("expected 3 after re-acquire, got %d", cl.Current())
	}
}

func TestBackpressureControllerStates(t *testing.T) {
	bp := NewBackpressureController(BackpressureConfig{
		MaxPending:  100,
		YellowRatio: 0.7,
		RedRatio:    0.9,
	})

	bp.UpdatePending(context.Background(), 50)
	if bp.State() != PressureGreen {
		t.Fatal("expected green at 50%")
	}

	bp.UpdatePending(context.Background(), 75)
	if bp.State() != PressureYellow {
		t.Fatal("expected yellow at 75%")
	}

	bp.UpdatePending(context.Background(), 95)
	if bp.State() != PressureRed {
		t.Fatal("expected red at 95%")
	}

	if bp.AllowDispatch() {
		t.Fatal("should not allow dispatch in red state")
	}
	if !bp.ShouldThrottle() {
		t.Fatal("should throttle in red state")
	}

	bp.UpdatePending(context.Background(), 10)
	if bp.State() != PressureGreen {
		t.Fatal("expected green after recovery")
	}
}

func TestBackpressureThrottleDelay(t *testing.T) {
	bp := NewBackpressureController(BackpressureConfig{
		MaxPending:  100,
		YellowRatio: 0.7,
		RedRatio:    0.9,
	})

	bp.UpdatePending(context.Background(), 80)
	if d := bp.ThrottleDelay(); d <= 0 {
		t.Fatal("expected positive delay in yellow state")
	}

	bp.UpdatePending(context.Background(), 95)
	if d := bp.ThrottleDelay(); d <= 0 {
		t.Fatal("expected positive delay in red state")
	}

	bp.UpdatePending(context.Background(), 10)
	if d := bp.ThrottleDelay(); d != 0 {
		t.Fatal("expected zero delay in green state")
	}
}
