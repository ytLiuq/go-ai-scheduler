package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	"github.com/example/go-ai-scheduler/internal/rpc"
	"github.com/example/go-ai-scheduler/internal/worker/executor"
	"github.com/example/go-ai-scheduler/internal/worker/localstore"
	"github.com/example/go-ai-scheduler/internal/worker/reporter"
	"github.com/example/go-ai-scheduler/internal/worker/sandbox"
)

// Handler processes task execution requests on the worker.
type Handler struct {
	workerID    string
	reporter    *reporter.Client
	logger      *log.Logger
	running     atomic.Int64
	sandboxDir  string
	maxMemBytes int64
	localStore  *localstore.Store
	dedupMap    sync.Map     // map[string]time.Time for schedule_instance_id -> first-seen timestamp
	cancels     sync.Map     // map[string]context.CancelFunc for in-flight cancellation
}

// HandlerConfig holds optional configuration for Handler.
type HandlerConfig struct {
	SandboxDir     string
	MaxMemoryBytes int64
	LocalStoreDir  string
}

// NewHandler creates a worker execution handler.
func NewHandler(workerID string, rep *reporter.Client, logger *log.Logger, cfg HandlerConfig) *Handler {
	if cfg.SandboxDir == "" {
		cfg.SandboxDir = ""
	}
	var ls *localstore.Store
	if cfg.LocalStoreDir != "" {
		var err error
		ls, err = localstore.New(cfg.LocalStoreDir, rep, logger)
		if err != nil {
			logger.Printf("localstore init failed, buffering disabled: %v", err)
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

// LocalStore returns the handler's local store, or nil if not configured.
func (h *Handler) LocalStore() *localstore.Store {
	return h.localStore
}

// StartDedupEviction starts a periodic goroutine that evicts old entries
// from the dedup map to prevent unbounded memory growth.
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

// isDuplicate checks if this schedule_instance_id was already seen recently.
func (h *Handler) isDuplicate(scheduleID string) bool {
	if _, loaded := h.dedupMap.LoadOrStore(scheduleID, time.Now()); loaded {
		return true
	}
	return false
}

// Cancel attempts to cancel an in-flight task by schedule_instance_id.
func (h *Handler) Cancel(scheduleID string) error {
	if cancel, ok := h.cancels.Load(scheduleID); ok {
		if fn, ok := cancel.(context.CancelFunc); ok {
			fn()
			return nil
		}
	}
	return fmt.Errorf("task not found or already completed: %s", scheduleID)
}

// CancelHTTP handles POST /internal/tasks/cancel.
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
	// Local dedup: reject duplicate schedule_instance_id.
	if h.isDuplicate(req.ScheduleInstanceID) {
		h.logger.Printf("duplicate task rejected schedule_instance_id=%s", req.ScheduleInstanceID)
		return
	}

	h.running.Add(1)
	defer h.running.Add(-1)

	// Acknowledge task receipt.
	_ = h.reporter.Ack(context.Background(), req.SchedulerURL, req.ScheduleInstanceID, h.workerID)

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
	defer h.cancels.Delete(req.ScheduleInstanceID)
	h.cancels.Store(req.ScheduleInstanceID, cancel)

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
		output, execErr = executor.Execute(ctx, req.TaskType, req.Payload, req.Image, map[string]string{
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
		h.logger.Printf("task execution failed schedule_instance_id=%s err=%v", req.ScheduleInstanceID, execErr)
	} else {
		statusReq.ErrorMessage = output
		metrics.DefaultRegistry.IncCounter("worker_executions_total", map[string]string{"status": statusReq.Status})
		h.logger.Printf("task execution succeeded schedule_instance_id=%s", req.ScheduleInstanceID)
	}

	if err := h.reporter.Report(context.Background(), req.SchedulerURL, statusReq); err != nil {
		h.logger.Printf("report task status failed schedule_instance_id=%s err=%v", req.ScheduleInstanceID, err)
		if h.localStore != nil {
			h.localStore.Buffer(req.SchedulerURL, statusReq)
		}
	} else if h.localStore != nil {
		h.localStore.Remove(req.ScheduleInstanceID)
	}
}

func (h *Handler) executeInSandbox(ctx context.Context, req rpc.ExecuteTaskRequest) (string, error) {
	sb, err := sandbox.New(h.sandboxDir, sandbox.Config{
		MaxMemoryBytes: h.maxMemBytes,
		Timeout:        time.Duration(req.TimeoutSeconds) * time.Second,
	})
	if err != nil {
		return "", err
	}
	defer func() {
		if cleanErr := sb.Cleanup(); cleanErr != nil {
			h.logger.Printf("sandbox cleanup failed schedule_instance_id=%s err=%v", req.ScheduleInstanceID, cleanErr)
		}
	}()

	h.logger.Printf("sandbox created for schedule_instance_id=%s workdir=%s", req.ScheduleInstanceID, sb.WorkDir())
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
