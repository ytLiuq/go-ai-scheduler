package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	"github.com/example/go-ai-scheduler/internal/rpc"
	"github.com/example/go-ai-scheduler/internal/worker/executor"
	"github.com/example/go-ai-scheduler/internal/worker/reporter"
)

// Handler processes task execution requests on the worker.
type Handler struct {
	workerID string
	reporter *reporter.Client
	logger   *log.Logger
	running  atomic.Int64
}

// NewHandler creates a worker execution handler.
func NewHandler(workerID string, reporter *reporter.Client, logger *log.Logger) *Handler {
	return &Handler{
		workerID: workerID,
		reporter: reporter,
		logger:   logger,
	}
}

// RunningTasks returns the number of in-flight executions on this worker.
func (h *Handler) RunningTasks() int {
	return int(h.running.Load())
}

// Execute accepts one dispatch request and runs it asynchronously.
func (h *Handler) Execute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req rpc.ExecuteTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid json body"})
		return
	}

	go h.run(req)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

// ExecuteAsync runs one dispatch request asynchronously.
func (h *Handler) ExecuteAsync(_ context.Context, req rpc.ExecuteTaskRequest) {
	go h.run(req)
}

func (h *Handler) run(req rpc.ExecuteTaskRequest) {
	h.running.Add(1)
	defer h.running.Add(-1)

	// Report running status before starting execution.
	runningReq := apiservice.TaskStatusReportRequest{
		ScheduleInstanceID: req.ScheduleInstanceID,
		WorkerID:           h.workerID,
		Status:             "running",
	}
	if err := h.reporter.Report(context.Background(), req.SchedulerURL, runningReq); err != nil {
		h.logger.Printf("report running status failed schedule_instance_id=%s err=%v", req.ScheduleInstanceID, err)
	}

	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	statusReq := apiservice.TaskStatusReportRequest{
		ScheduleInstanceID: req.ScheduleInstanceID,
		WorkerID:           h.workerID,
		Status:             "success",
	}

	if err := executor.Execute(ctx, req.TaskType, req.Payload); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			statusReq.Status = "timeout"
			statusReq.ErrorCode = "timeout"
		} else {
			statusReq.Status = "failed"
			statusReq.ErrorCode = "execute_failed"
		}
		statusReq.ErrorMessage = err.Error()
		metrics.DefaultRegistry.IncCounter("worker_executions_total", map[string]string{"status": statusReq.Status})
		h.logger.Printf("task execution failed schedule_instance_id=%s err=%v", req.ScheduleInstanceID, err)
	} else {
		metrics.DefaultRegistry.IncCounter("worker_executions_total", map[string]string{"status": statusReq.Status})
		h.logger.Printf("task execution succeeded schedule_instance_id=%s", req.ScheduleInstanceID)
	}

	if err := h.reporter.Report(context.Background(), req.SchedulerURL, statusReq); err != nil {
		h.logger.Printf("report task status failed schedule_instance_id=%s err=%v", req.ScheduleInstanceID, err)
	}
}
