package ratelimit

import (
	"context"
	"sync"
	"time"
)

// PressureState indicates the current backpressure level.
type PressureState int

const (
	PressureGreen  PressureState = iota // normal operation
	PressureYellow                      // approaching limit, throttle
	PressureRed                         // at limit, reject new work
)

func (s PressureState) String() string {
	switch s {
	case PressureGreen:
		return "green"
	case PressureYellow:
		return "yellow"
	case PressureRed:
		return "red"
	default:
		return "unknown"
	}
}

// BackpressureController monitors system load and signals when to throttle.
type BackpressureController struct {
	mu sync.RWMutex

	// Thresholds
	maxPending    int
	yellowRatio   float64 // ratio of maxPending that triggers yellow
	redRatio      float64 // ratio of maxPending that triggers red
	maxWorkerLoad float64 // average load per worker that triggers throttling

	// Current state
	state        PressureState
	pendingCount int
	workerLoads  map[string]int64
	lastUpdate   time.Time
}

// BackpressureConfig configures thresholds.
type BackpressureConfig struct {
	MaxPending    int
	YellowRatio   float64
	RedRatio      float64
	MaxWorkerLoad float64
}

// DefaultBackpressureConfig returns sensible defaults.
func DefaultBackpressureConfig() BackpressureConfig {
	return BackpressureConfig{
		MaxPending:    1000,
		YellowRatio:   0.7,
		RedRatio:      0.9,
		MaxWorkerLoad: 0.85,
	}
}

// NewBackpressureController creates a backpressure controller.
func NewBackpressureController(cfg BackpressureConfig) *BackpressureController {
	if cfg.MaxPending <= 0 {
		cfg.MaxPending = 1000
	}
	if cfg.YellowRatio <= 0 {
		cfg.YellowRatio = 0.7
	}
	if cfg.RedRatio <= 0 {
		cfg.RedRatio = 0.9
	}
	if cfg.MaxWorkerLoad <= 0 {
		cfg.MaxWorkerLoad = 0.85
	}
	return &BackpressureController{
		maxPending:    cfg.MaxPending,
		yellowRatio:   cfg.YellowRatio,
		redRatio:      cfg.RedRatio,
		maxWorkerLoad: cfg.MaxWorkerLoad,
		workerLoads:   make(map[string]int64),
		lastUpdate:    time.Now(),
	}
}

// UpdatePending refreshes the pending instance count and recalculates state.
func (b *BackpressureController) UpdatePending(ctx context.Context, count int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.pendingCount = count
	b.lastUpdate = time.Now()

	ratio := float64(count) / float64(b.maxPending)

	switch {
	case ratio >= b.redRatio:
		b.state = PressureRed
	case ratio >= b.yellowRatio:
		b.state = PressureYellow
	default:
		b.state = PressureGreen
	}
}

// UpdateWorkerLoad refreshes per-worker load and recalculates state.
func (b *BackpressureController) UpdateWorkerLoad(ctx context.Context, loads map[string]int64, maxConcurrency int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.workerLoads = loads
	b.lastUpdate = time.Now()

	if maxConcurrency <= 0 {
		return
	}

	var totalLoad float64
	for _, l := range loads {
		totalLoad += float64(l)
	}

	if len(loads) == 0 {
		return
	}
	avgLoad := totalLoad / float64(len(loads))
	avgRatio := avgLoad / float64(maxConcurrency)

	switch {
	case avgRatio >= b.maxWorkerLoad:
		b.state = PressureRed
	case avgRatio >= b.maxWorkerLoad*0.8:
		if b.state < PressureYellow {
			b.state = PressureYellow
		}
	}
}

// State returns the current backpressure state.
func (b *BackpressureController) State() PressureState {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// AllowDispatch returns true if dispatching is allowed under current pressure.
func (b *BackpressureController) AllowDispatch() bool {
	return b.State() != PressureRed
}

// ShouldThrottle returns true if the system should slow down dispatching.
func (b *BackpressureController) ShouldThrottle() bool {
	state := b.State()
	return state == PressureYellow || state == PressureRed
}

// ThrottleDelay returns the recommended delay before the next dispatch attempt.
func (b *BackpressureController) ThrottleDelay() time.Duration {
	b.mu.RLock()
	defer b.mu.RUnlock()

	switch b.state {
	case PressureYellow:
		return 500 * time.Millisecond
	case PressureRed:
		return 2 * time.Second
	default:
		return 0
	}
}
