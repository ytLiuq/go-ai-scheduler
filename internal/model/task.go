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
	MaxRetry        int
	RetryPolicy     string
	RouteStrategy   string
	NextTriggerTime time.Time
	TenantID        int64
	Version         int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

