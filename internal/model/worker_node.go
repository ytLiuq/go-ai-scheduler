package model

import "time"

// WorkerNode stores the latest state of a worker process.
type WorkerNode struct {
	ID              string
	Hostname        string
	IP              string
	CallbackURL     string
	GRPCAddr        string
	Protocol        string
	Status          string
	Labels          string
	MaxConcurrency  int
	CurrentLoad     int
	LastHeartbeatAt time.Time
}
