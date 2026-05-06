package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/example/go-ai-scheduler/internal/alert"
	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	"github.com/example/go-ai-scheduler/internal/repo"
	"github.com/example/go-ai-scheduler/internal/scheduler/dispatch"
	"github.com/example/go-ai-scheduler/internal/scheduler/route"
)

var ErrScheduleInstanceIDRequired = errors.New("schedule_instance_id is required")

// TaskStatusReportRequest is posted by workers after execution.
type TaskStatusReportRequest struct {
	ScheduleInstanceID string `json:"schedule_instance_id"`
	WorkerID           string `json:"worker_id"`
	Status             string `json:"status"`
	ErrorCode          string `json:"error_code"`
	ErrorMessage       string `json:"error_message"`
	StartedAt          string `json:"started_at,omitempty"`
	FinishedAt         string `json:"finished_at,omitempty"`
}

// TaskRuntimeService updates runtime execution state.
type TaskRuntimeService struct {
	tasks        repo.TaskRepository
	instances    repo.TaskInstanceRepository
	workers      repo.WorkerRepository
	router       *route.Router
	dispatch     *dispatch.Client
	alerter      *alert.Alerter
	logger       *slog.Logger
	schedulerURL string
	aiClient     *AIClient
}

// NewTaskRuntimeService creates a TaskRuntimeService.
func NewTaskRuntimeService(
	tasks repo.TaskRepository,
	instances repo.TaskInstanceRepository,
	workers repo.WorkerRepository,
	router *route.Router,
	dispatcher *dispatch.Client,
	alerter *alert.Alerter,
	schedulerURL string,
	logger *slog.Logger,
	aiClient *AIClient,
) *TaskRuntimeService {
	return &TaskRuntimeService{
		tasks:        tasks,
		instances:    instances,
		workers:      workers,
		router:       router,
		dispatch:     dispatcher,
		alerter:      alerter,
		logger:       logger,
		schedulerURL: schedulerURL,
		aiClient:     aiClient,
	}
}

// ReportStatus updates task instance state and releases worker load.
func (s *TaskRuntimeService) ReportStatus(ctx context.Context, req TaskStatusReportRequest) error {
	if req.ScheduleInstanceID == "" {
		return ErrScheduleInstanceIDRequired
	}
	instance, err := s.instances.GetInstanceByScheduleID(ctx, req.ScheduleInstanceID)
	if err != nil {
		return err
	}
	if isTerminalStatus(instance.Status) {
		s.logger.Debug("duplicate task status ignored", "schedule_instance_id", req.ScheduleInstanceID, "current_status", instance.Status, "incoming_status", req.Status)
		return nil
	}
	if err := s.instances.UpdateInstanceResult(ctx, req.ScheduleInstanceID, req.Status, req.ErrorCode, req.ErrorMessage); err != nil {
		return err
	}
	if req.StartedAt != "" || req.FinishedAt != "" {
		var startedAt, finishedAt time.Time
		if req.StartedAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, req.StartedAt); err == nil {
				startedAt = t
			}
		}
		if req.FinishedAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, req.FinishedAt); err == nil {
				finishedAt = t
			}
		}
		if err := s.instances.UpdateInstanceTimestamps(ctx, req.ScheduleInstanceID, startedAt, finishedAt); err != nil {
			s.logger.Warn("update timestamps failed", "schedule_instance_id", req.ScheduleInstanceID, "error", err)
		}
	}
	metrics.DefaultRegistry.IncCounter("task_runtime_reports_total", map[string]string{"status": req.Status})
	if req.Status == "running" {
		s.logger.Debug("task running", "schedule_instance_id", req.ScheduleInstanceID, "worker_id", req.WorkerID)
		return nil
	}
	if req.Status == "failed" && s.aiClient != nil {
		go s.analyzeFailedInstance(instance, req)
	}
	if req.Status == "success" {
		s.fireDownstream(ctx, instance)
	}
	if req.WorkerID == "" {
		return s.retryIfNeeded(ctx, instance, req)
	}
	worker, err := s.workers.GetWorker(ctx, req.WorkerID)
	if err != nil {
		return s.retryIfNeeded(ctx, instance, req)
	}
	if worker.CurrentLoad > 0 {
		worker.CurrentLoad--
	}
	if err := s.workers.UpsertWorker(ctx, worker); err != nil {
		return err
	}
	return s.retryIfNeeded(ctx, instance, req)
}

func (s *TaskRuntimeService) fireDownstream(ctx context.Context, instance *model.TaskInstance) {
	downstream, err := s.tasks.ListDownstreamTasks(ctx, instance.TaskID)
	if err != nil {
		s.logger.Warn("list downstream tasks failed", "task_id", instance.TaskID, "error", err)
		return
	}
	for _, dtID := range downstream {
		dt, err := s.tasks.GetTask(ctx, dtID)
		if err != nil {
			continue
		}
		if dt.Status != "enabled" {
			continue
		}
		dt.NextTriggerTime = time.Now()
		if err := s.tasks.UpdateTask(ctx, dt); err != nil {
			s.logger.Warn("update downstream task next_trigger failed", "task_id", dtID, "error", err)
		}
		s.logger.Debug("downstream task advanced", "task_id", dtID, "depends_on", instance.TaskID)
	}
}

func (s *TaskRuntimeService) retryIfNeeded(ctx context.Context, instance *model.TaskInstance, req TaskStatusReportRequest) error {
	if !isRetryableStatus(req.Status) {
		return nil
	}

	task, err := s.tasks.GetTask(ctx, instance.TaskID)
	if err != nil {
		return err
	}
	if instance.RetryCount >= task.MaxRetry {
		s.logger.Debug("retry skipped: max retries exhausted", "task_id", task.ID, "schedule_instance_id", instance.ScheduleInstanceID, "retry_count", instance.RetryCount, "max_retry", task.MaxRetry)
		if s.alerter != nil {
			s.alerter.Send(ctx, alert.Payload{
				TaskID:             task.ID,
				TaskName:           task.Name,
				InstanceID:         instance.ID,
				ScheduleInstanceID: instance.ScheduleInstanceID,
				RetryCount:         instance.RetryCount,
				MaxRetry:           task.MaxRetry,
				ErrorCode:          req.ErrorCode,
				ErrorMessage:       req.ErrorMessage,
			})
		}
		return nil
	}
	if !shouldRetryOnErrors(task.RetryPolicy, task.RetryOnErrors, req.ErrorCode) {
		s.logger.Debug("retry skipped: error code not in retry list", "task_id", task.ID, "schedule_instance_id", instance.ScheduleInstanceID, "error_code", req.ErrorCode)
		return nil
	}
	if retryWindowExceeded(instance, task.RetryWindowSeconds) {
		s.logger.Debug("retry skipped: total window exceeded", "task_id", task.ID, "schedule_instance_id", instance.ScheduleInstanceID, "window_seconds", task.RetryWindowSeconds)
		return nil
	}

	retryCount := instance.RetryCount + 1
	retryInstance := &model.TaskInstance{
		TaskID:             task.ID,
		ScheduleInstanceID: retryScheduleInstanceID(task.ID, retryCount),
		TriggerTime:        time.Now(),
		Status:             "pending",
		RetryCount:         retryCount,
		ShardNo:            instance.ShardNo,
		ShardTotal:         instance.ShardTotal,
	}

	if delay := retryDelay(task.RetryPolicy, retryCount, task.RetryIntervalSeconds); delay > 0 {
		retryInstance.Status = "retry_waiting"
		retryInstance.NextRetryTime = time.Now().Add(delay)
		if err := s.instances.CreateInstance(ctx, retryInstance); err != nil {
			return err
		}
		s.logger.Debug("retry deferred", "task_id", task.ID, "retry_instance_id", retryInstance.ID, "retry_count", retryCount, "delay", delay)
		return nil
	}

	if err := s.instances.CreateInstance(ctx, retryInstance); err != nil {
		return err
	}

	worker, err := s.router.Pick(ctx, route.SelectOptions{
		Labels:   model.DecodeLabels(task.Labels),
		Strategy: task.RouteStrategy,
	})
	if err != nil {
		if err == route.ErrNoAvailableWorker {
			_ = s.instances.UpdateInstanceStatus(ctx, retryInstance.ID, "retry_waiting")
			s.logger.Debug("retry waiting for worker", "task_id", task.ID, "retry_instance_id", retryInstance.ID, "retry_count", retryCount)
			return nil
		}
		return err
	}

	dispatchTime := time.Now()
	if err := s.instances.UpdateInstanceDispatch(ctx, retryInstance.ID, worker.ID, dispatchTime.Format(time.RFC3339Nano)); err != nil {
		_ = s.router.Release(ctx, worker)
		return err
	}
	if err := s.dispatch.Dispatch(ctx, worker, model.ExecuteTaskRequest{
		ScheduleInstanceID: retryInstance.ScheduleInstanceID,
		TaskID:             task.ID,
		TaskType:           task.Type,
		Payload:            task.Payload,
		TimeoutSeconds:     task.TimeoutSeconds,
		RetryCount:         retryCount,
		ShardNo:            instance.ShardNo,
		ShardTotal:         instance.ShardTotal,
		IdempotencyKey:     task.IdempotencyKey,
		SchedulerURL:       s.schedulerURL,
	}); err != nil {
		_ = s.instances.UpdateInstanceStatus(ctx, retryInstance.ID, "retry_waiting")
		_ = s.router.Release(ctx, worker)
		s.logger.Warn("retry dispatch deferred", "task_id", task.ID, "retry_instance_id", retryInstance.ID, "error", err)
		return nil
	}

	s.logger.Debug("retry dispatched", "task_id", task.ID, "retry_instance_id", retryInstance.ID, "retry_count", retryCount, "worker_id", worker.ID)
	return nil
}

// CancelInstance cancels an in-flight task instance and dispatches cancellation to the worker.
func (s *TaskRuntimeService) CancelInstance(ctx context.Context, scheduleInstanceID string) error {
	instance, err := s.instances.GetInstanceByScheduleID(ctx, scheduleInstanceID)
	if err != nil {
		return err
	}
	if isTerminalStatus(instance.Status) {
		return fmt.Errorf("instance already in terminal state: %s", instance.Status)
	}
	if instance.WorkerID != "" {
		worker, err := s.workers.GetWorker(ctx, instance.WorkerID)
		if err == nil {
			_ = s.dispatch.CancelDispatch(ctx, worker, scheduleInstanceID)
		}
	}
	return s.instances.UpdateInstanceResult(ctx, scheduleInstanceID, "cancelled", "cancelled", "cancelled by user")
}

func retryDelay(retryPolicy string, retryCount int, intervalSeconds int) time.Duration {
	switch retryPolicy {
	case "exponential_backoff":
		d := time.Duration(1<<retryCount) * time.Second
		if d > 10*time.Minute {
			d = 10 * time.Minute
		}
		return d
	case "fixed_interval":
		if intervalSeconds > 0 {
			return time.Duration(intervalSeconds) * time.Second
		}
		return 0
	default:
		return 0
	}
}

func retryWindowExceeded(instance *model.TaskInstance, windowSeconds int) bool {
	if windowSeconds <= 0 {
		return false
	}
	window := time.Duration(windowSeconds) * time.Second
	return time.Since(instance.TriggerTime) > window
}

func shouldRetryOnErrors(retryPolicy, retryOnErrors, errorCode string) bool {
	if retryPolicy != "error_code" || retryOnErrors == "" {
		return true
	}
	for _, code := range strings.Split(retryOnErrors, ",") {
		if strings.TrimSpace(code) == errorCode {
			return true
		}
	}
	return false
}

func (s *TaskRuntimeService) analyzeFailedInstance(instance *model.TaskInstance, req TaskStatusReportRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	task, err := s.tasks.GetTask(ctx, instance.TaskID)
	if err != nil {
		s.logger.Warn("ai analysis: get task failed", "task_id", instance.TaskID, "error", err)
		return
	}

	enrichedMsg := fmt.Sprintf("[Task: %s | Timeout: %ds | MaxRetry: %d | RetryPolicy: %s]\n%s",
		task.Name, task.TimeoutSeconds, task.MaxRetry, task.RetryPolicy, req.ErrorMessage)
	result, err := s.aiClient.AnalyzeLog(ctx, enrichedMsg, req.ErrorCode, task.Type, instance.RetryCount, instance.ID)
	if err != nil {
		s.logger.Warn("ai analysis failed", "schedule_instance_id", instance.ScheduleInstanceID, "error", err)
		return
	}

	analysisJSON, _ := json.Marshal(result)
	if err := s.instances.UpdateInstanceAnalysis(ctx, instance.ScheduleInstanceID, string(analysisJSON)); err != nil {
		s.logger.Warn("ai analysis persist failed", "schedule_instance_id", instance.ScheduleInstanceID, "error", err)
		return
	}
	s.logger.Debug("ai analysis completed", "schedule_instance_id", instance.ScheduleInstanceID, "severity", result.Severity)
}

func retryScheduleInstanceID(taskID int64, retryCount int) string {
	return fmt.Sprintf("task-%d-retry-%d-%d", taskID, retryCount, time.Now().UnixNano())
}

func isTerminalStatus(status string) bool {
	switch status {
	case "success", "failed", "timeout", "cancelled":
		return true
	default:
		return false
	}
}

func isRetryableStatus(status string) bool {
	switch status {
	case "failed", "timeout":
		return true
	default:
		return false
	}
}
