package handler

import (
	"net/http"

	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
)

// NewSchedulerRouter wires internal scheduler-facing routes.
func NewSchedulerRouter(workerHandler *WorkerHandler, taskRuntimeHandler *TaskRuntimeHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", Health)
	mux.Handle("/metrics", metrics.DefaultRegistry.Handler())
	mux.HandleFunc("/api/v1/workers/register", workerHandler.Register)
	mux.HandleFunc("/api/v1/workers/heartbeat", workerHandler.Heartbeat)
	mux.HandleFunc("/api/v1/task-instances/report", taskRuntimeHandler.Report)
	return metrics.Instrument("scheduler", mux)
}

// NewAPIRouter wires external management and query routes.
func NewAPIRouter(workerHandler *WorkerHandler, taskHandler *TaskHandler, taskInstanceHandler *TaskInstanceHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", Health)
	mux.Handle("/metrics", metrics.DefaultRegistry.Handler())
	mux.HandleFunc("/api/v1/workers", workerHandler.List)
	mux.HandleFunc("/api/v1/workers/", workerHandler.Get)
	mux.HandleFunc("/api/v1/tasks", taskHandler.List)
	mux.HandleFunc("/api/v1/tasks/", taskHandler.GetOrUpdate)
	mux.HandleFunc("/api/v1/task-instances", taskInstanceHandler.List)
	mux.HandleFunc("/api/v1/task-instances/", taskInstanceHandler.Get)
	return metrics.Instrument("api", mux)
}
