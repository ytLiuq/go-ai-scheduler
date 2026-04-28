package ratelimit

import (
	"sync"
	"time"
)

// TokenBucket implements a token-bucket rate limiter.
type TokenBucket struct {
	mu        sync.Mutex
	rate      float64 // tokens per second
	capacity  float64 // max tokens
	tokens    float64
	lastRefill time.Time
}

// NewTokenBucket creates a token bucket with the given rate and capacity.
func NewTokenBucket(ratePerSec, capacity int) *TokenBucket {
	if ratePerSec <= 0 {
		ratePerSec = 1000
	}
	if capacity <= 0 {
		capacity = ratePerSec * 2
	}
	return &TokenBucket{
		rate:       float64(ratePerSec),
		capacity:   float64(capacity),
		tokens:     float64(capacity),
		lastRefill: time.Now(),
	}
}

// Allow consumes one token if available. Returns true if allowed.
func (tb *TokenBucket) Allow() bool {
	return tb.AllowN(1)
}

// AllowN consumes n tokens if available.
func (tb *TokenBucket) AllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()
	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}
	return false
}

// WaitTime returns the estimated wait time before n tokens would be available.
func (tb *TokenBucket) WaitTime(n int) time.Duration {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()
	if tb.tokens >= float64(n) {
		return 0
	}
	needed := float64(n) - tb.tokens
	return time.Duration(needed / tb.rate * float64(time.Second))
}

func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}
	tb.lastRefill = now
}

// ConcurrencyLimiter limits the number of concurrent operations.
type ConcurrencyLimiter struct {
	mu      sync.Mutex
	limit   int
	current int
	cond    *sync.Cond
}

// NewConcurrencyLimiter creates a concurrency limiter.
func NewConcurrencyLimiter(limit int) *ConcurrencyLimiter {
	if limit <= 0 {
		limit = 100
	}
	cl := &ConcurrencyLimiter{limit: limit}
	cl.cond = sync.NewCond(&cl.mu)
	return cl
}

// Acquire blocks until a slot is available, then increments the counter.
func (cl *ConcurrencyLimiter) Acquire() {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	for cl.current >= cl.limit {
		cl.cond.Wait()
	}
	cl.current++
}

// Release decrements the counter and signals one waiting goroutine.
func (cl *ConcurrencyLimiter) Release() {
	cl.mu.Lock()
	if cl.current > 0 {
		cl.current--
	}
	cl.cond.Signal()
	cl.mu.Unlock()
}

// Current returns the current concurrency count.
func (cl *ConcurrencyLimiter) Current() int {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.current
}

// Limit returns the configured limit.
func (cl *ConcurrencyLimiter) Limit() int {
	return cl.limit
}
