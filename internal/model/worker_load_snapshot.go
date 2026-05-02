package model

import "time"

// WorkerLoadSnapshot records a point-in-time worker load sample.
type WorkerLoadSnapshot struct {
	ID             int64
	WorkerID       string
	CurrentLoad    int
	MaxConcurrency int
	Status         string
	RecordedAt     time.Time
}
