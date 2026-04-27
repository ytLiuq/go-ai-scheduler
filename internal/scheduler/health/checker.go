package health

import (
	"context"
	"log"
	"time"

	"github.com/example/go-ai-scheduler/internal/api/service"
)

// Checker periodically evicts workers that have stopped sending heartbeats.
type Checker struct {
	workers *service.WorkerService
	logger  *log.Logger
	timeout time.Duration
	tick    time.Duration
}

// NewChecker creates a health Checker.
func NewChecker(workers *service.WorkerService, logger *log.Logger, timeout, tick time.Duration) *Checker {
	return &Checker{
		workers: workers,
		logger:  logger,
		timeout: timeout,
		tick:    tick,
	}
}

// Start runs the health check loop until ctx is cancelled.
func (c *Checker) Start(ctx context.Context) {
	ticker := time.NewTicker(c.tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := c.workers.EvictStaleWorkers(ctx, c.timeout)
			if err != nil {
				c.logger.Printf("health check failed: %v", err)
			} else if count > 0 {
				c.logger.Printf("health check evicted %d stale worker(s)", count)
			}
		}
	}
}
