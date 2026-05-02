package cron

import (
	"time"

	"github.com/example/go-ai-scheduler/internal/pkg/cronexpr"
)

// NextRunResponse describes the next scheduled run for one cron expression.
type NextRunResponse struct {
	Expression string    `json:"expression"`
	BaseTime   time.Time `json:"base_time"`
	NextRun    time.Time `json:"next_run"`
}

// NextRun computes the next run strictly after baseTime.
func NextRun(expression string, baseTime time.Time) (*NextRunResponse, error) {
	if baseTime.IsZero() {
		baseTime = time.Now()
	}
	nextRun, err := cronexpr.NextAfter(baseTime, expression)
	if err != nil {
		return nil, err
	}
	return &NextRunResponse{
		Expression: expression,
		BaseTime:   baseTime,
		NextRun:    nextRun,
	}, nil
}
