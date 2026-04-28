package model

import "time"

// Task defines the static configuration of a schedulable task.
type Task struct {
	ID              int64
	Name            string
	Type            string
	CronExpr        string
	Payload         string
	Status          string
	TimeoutSeconds  int
	MaxRetry             int
	RetryPolicy          string
	RetryOnErrors        string
	RetryIntervalSeconds int // fixed_interval delay in seconds
	RetryWindowSeconds   int // total retry window in seconds (0 = unlimited)
	RouteStrategy        string
	Labels          string
	NextTriggerTime time.Time
	TenantID        int64
	Version         int64
	TotalShards     int    // number of shards (0 or 1 = no sharding)
	IdempotencyKey  string // optional business idempotency key
	TriggerType     string // "cron" (default), "event"
	EventName       string // event name for event-triggered tasks
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

