package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
)

// Handler processes task execution requests on the worker.
type Handler struct {
	workerID    string
	reporter    *ReportClient
	logger      *slog.Logger
	running     atomic.Int64
	sandboxDir  string
	maxMemBytes int64
	localStore  *Store
	dedupMap    sync.Map
	cancels     sync.Map
}

// HandlerConfig holds optional configuration for Handler.
type HandlerConfig struct {
	SandboxDir     string
	MaxMemoryBytes int64
	LocalStoreDir  string
}

// NewHandler creates a worker execution handler.
func NewHandler(workerID string, rep *ReportClient, logger *slog.Logger, cfg HandlerConfig) *Handler {
	if cfg.SandboxDir == "" {
		cfg.SandboxDir = ""
	}
	var ls *Store
	if cfg.LocalStoreDir != "" {
		var err error
		ls, err = NewStore(cfg.LocalStoreDir, rep, logger)
		if err != nil {
			logger.Warn("localstore init failed, buffering disabled", "error", err)
		}
	}
	return &Handler{
		workerID:    workerID,
		reporter:    rep,
		logger:      logger,
		sandboxDir:  cfg.SandboxDir,
		maxMemBytes: cfg.MaxMemoryBytes,
		localStore:  ls,
	}
}

func (h *Handler) LocalStore() *Store {
	return h.localStore
}

func (h *Handler) StartDedupEviction(ctx context.Context, ttl, interval time.Duration) {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-ttl)
				h.dedupMap.Range(func(key, value any) bool {
					if seen, ok := value.(time.Time); ok && seen.Before(cutoff) {
						h.dedupMap.Delete(key)
					}
					return true
				})
			}
		}
	}()
}

func (h *Handler) isDuplicate(scheduleID string) bool {
	if _, loaded := h.dedupMap.LoadOrStore(scheduleID, time.Now()); loaded {
		return true
	}
	return false
}

func (h *Handler) Cancel(scheduleID string) error {
	if cancel, ok := h.cancels.Load(scheduleID); ok {
		if fn, ok := cancel.(context.CancelFunc); ok {
			fn()
			return nil
		}
	}
	return fmt.Errorf("task not found or already completed: %s", scheduleID)
}

func (h *Handler) CancelHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ScheduleInstanceID string `json:"schedule_instance_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid body"})
		return
	}
	if err := h.Cancel(req.ScheduleInstanceID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}

func (h *Handler) RunningTasks() int {
	return int(h.running.Load())
}

func (h *Handler) Execute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req model.ExecuteTaskRequest
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

func (h *Handler) ExecuteAsync(_ context.Context, req model.ExecuteTaskRequest) {
	go h.run(req)
}

func (h *Handler) run(req model.ExecuteTaskRequest) {
	if h.isDuplicate(req.ScheduleInstanceID) {
		h.logger.Warn("duplicate task rejected", "schedule_instance_id", req.ScheduleInstanceID)
		return
	}

	h.running.Add(1)
	defer h.running.Add(-1)

	_ = h.reporter.Ack(context.Background(), req.SchedulerURL, req.ScheduleInstanceID, h.workerID)

	runningReq := apiservice.TaskStatusReportRequest{
		ScheduleInstanceID: req.ScheduleInstanceID,
		WorkerID:           h.workerID,
		Status:             "running",
	}
	if err := h.reporter.Report(context.Background(), req.SchedulerURL, runningReq); err != nil {
		h.logger.Warn("report running status failed", "schedule_instance_id", req.ScheduleInstanceID, "error", err)
	}

	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	defer h.cancels.Delete(req.ScheduleInstanceID)
	h.cancels.Store(req.ScheduleInstanceID, cancel)

	startedAt := time.Now().UTC().Format(time.RFC3339Nano)
	statusReq := apiservice.TaskStatusReportRequest{
		ScheduleInstanceID: req.ScheduleInstanceID,
		WorkerID:           h.workerID,
		Status:             "success",
	}

	var output string
	var execErr error
	if req.TaskType == "shell" && h.sandboxDir != "" {
		output, execErr = h.executeInSandbox(ctx, req)
	} else {
		output, execErr = Execute(ctx, req.TaskType, req.Payload, req.Image, map[string]string{
			"IDEMPOTENCY_KEY":      req.IdempotencyKey,
			"SHARD_NO":             fmt.Sprintf("%d", req.ShardNo),
			"SHARD_TOTAL":          fmt.Sprintf("%d", req.ShardTotal),
			"SCHEDULE_INSTANCE_ID": req.ScheduleInstanceID,
		})
	}
	output = trimExecutionMessage(output)

	if execErr != nil {
		if errors.Is(execErr, context.DeadlineExceeded) {
			statusReq.Status = "timeout"
			statusReq.ErrorCode = "timeout"
		} else {
			statusReq.Status = "failed"
			statusReq.ErrorCode = "execute_failed"
		}
		statusReq.ErrorMessage = execErr.Error()
		metrics.DefaultRegistry.IncCounter("worker_executions_total", map[string]string{"status": statusReq.Status})
		h.logger.Warn("task execution failed", "schedule_instance_id", req.ScheduleInstanceID, "error", execErr)
	} else {
		statusReq.ErrorMessage = output
		metrics.DefaultRegistry.IncCounter("worker_executions_total", map[string]string{"status": statusReq.Status})
		h.logger.Debug("task execution succeeded", "schedule_instance_id", req.ScheduleInstanceID)
	}

	statusReq.StartedAt = startedAt
	statusReq.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)

	if err := h.reporter.Report(context.Background(), req.SchedulerURL, statusReq); err != nil {
		h.logger.Warn("report task status failed", "schedule_instance_id", req.ScheduleInstanceID, "error", err)
		if h.localStore != nil {
			h.localStore.Buffer(req.SchedulerURL, statusReq)
		}
	} else if h.localStore != nil {
		h.localStore.Remove(req.ScheduleInstanceID)
	}
}

func (h *Handler) executeInSandbox(ctx context.Context, req model.ExecuteTaskRequest) (string, error) {
	sb, err := NewSandbox(h.sandboxDir, SandboxConfig{
		MaxMemoryBytes: h.maxMemBytes,
		Timeout:        time.Duration(req.TimeoutSeconds) * time.Second,
	})
	if err != nil {
		return "", err
	}
	defer func() {
		if cleanErr := sb.Cleanup(); cleanErr != nil {
			h.logger.Warn("sandbox cleanup failed", "schedule_instance_id", req.ScheduleInstanceID, "error", cleanErr)
		}
	}()

	h.logger.Debug("sandbox created", "schedule_instance_id", req.ScheduleInstanceID, "workdir", sb.WorkDir())
	out, err := sb.ShellExec(ctx, req.Payload, map[string]string{
		"IDEMPOTENCY_KEY":      req.IdempotencyKey,
		"SHARD_NO":             fmt.Sprintf("%d", req.ShardNo),
		"SHARD_TOTAL":          fmt.Sprintf("%d", req.ShardTotal),
		"SCHEDULE_INSTANCE_ID": req.ScheduleInstanceID,
	})
	return string(out), err
}

func trimExecutionMessage(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1024 {
		return value[:1024]
	}
	return value
}
