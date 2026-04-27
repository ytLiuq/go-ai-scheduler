package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/example/go-ai-scheduler/internal/alert"
	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	"github.com/example/go-ai-scheduler/internal/repo"
	"github.com/example/go-ai-scheduler/internal/rpc"
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
}

// TaskRuntimeService updates runtime execution state.
type TaskRuntimeService struct {
	tasks        repo.TaskRepository
	instances    repo.TaskInstanceRepository
	workers      repo.WorkerRepository
	router       *route.Router
	dispatch     *dispatch.Client
	alerter      *alert.Alerter
	logger       *log.Logger
	schedulerURL string
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
	logger *log.Logger,
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
		s.logger.Printf("duplicate task status ignored schedule_instance_id=%s current_status=%s incoming_status=%s",
			req.ScheduleInstanceID, instance.Status, req.Status)
		return nil
	}
	if err := s.instances.UpdateInstanceResult(ctx, req.ScheduleInstanceID, req.Status, req.ErrorCode, req.ErrorMessage); err != nil {
		return err
	}
	metrics.DefaultRegistry.IncCounter("task_runtime_reports_total", map[string]string{"status": req.Status})
	if req.Status == "running" {
		s.logger.Printf("task running schedule_instance_id=%s worker_id=%s", req.ScheduleInstanceID, req.WorkerID)
		return nil
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

func (s *TaskRuntimeService) retryIfNeeded(ctx context.Context, instance *model.TaskInstance, req TaskStatusReportRequest) error {
	if !isRetryableStatus(req.Status) {
		return nil
	}

	task, err := s.tasks.GetTask(ctx, instance.TaskID)
	if err != nil {
		return err
	}
	if instance.RetryCount >= task.MaxRetry {
		s.logger.Printf("retry skipped task_id=%d schedule_instance_id=%s retry_count=%d max_retry=%d",
			task.ID, instance.ScheduleInstanceID, instance.RetryCount, task.MaxRetry)
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
		s.logger.Printf("retry skipped by error_code policy task_id=%d schedule_instance_id=%s error_code=%s",
			task.ID, instance.ScheduleInstanceID, req.ErrorCode)
		return nil
	}

	retryCount := instance.RetryCount + 1
	retryInstance := &model.TaskInstance{
		TaskID:             task.ID,
		ScheduleInstanceID: retryScheduleInstanceID(task.ID, retryCount),
		TriggerTime:        time.Now(),
		Status:             "pending",
		RetryCount:         retryCount,
	}

	if delay := retryDelay(task.RetryPolicy, retryCount); delay > 0 {
		retryInstance.Status = "retry_waiting"
		retryInstance.NextRetryTime = time.Now().Add(delay)
		if err := s.instances.CreateInstance(ctx, retryInstance); err != nil {
			return err
		}
		s.logger.Printf("retry deferred task_id=%d retry_instance_id=%d retry_count=%d delay=%v",
			task.ID, retryInstance.ID, retryCount, delay)
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
			s.logger.Printf("retry waiting for worker task_id=%d retry_instance_id=%d retry_count=%d",
				task.ID, retryInstance.ID, retryCount)
			return nil
		}
		return err
	}

	dispatchTime := time.Now()
	if err := s.instances.UpdateInstanceDispatch(ctx, retryInstance.ID, worker.ID, dispatchTime.Format(time.RFC3339Nano)); err != nil {
		_ = s.router.Release(ctx, worker)
		return err
	}
	if err := s.dispatch.Dispatch(ctx, worker, rpc.ExecuteTaskRequest{
		ScheduleInstanceID: retryInstance.ScheduleInstanceID,
		TaskID:             task.ID,
		TaskType:           task.Type,
		Payload:            task.Payload,
		TimeoutSeconds:     task.TimeoutSeconds,
		RetryCount:         retryCount,
		SchedulerURL:       s.schedulerURL,
	}); err != nil {
		_ = s.instances.UpdateInstanceStatus(ctx, retryInstance.ID, "retry_waiting")
		_ = s.router.Release(ctx, worker)
		s.logger.Printf("retry dispatch deferred task_id=%d retry_instance_id=%d err=%v",
			task.ID, retryInstance.ID, err)
		return nil
	}

	s.logger.Printf("retry dispatched task_id=%d retry_instance_id=%d retry_count=%d worker_id=%s",
		task.ID, retryInstance.ID, retryCount, worker.ID)
	return nil
}

func retryDelay(retryPolicy string, retryCount int) time.Duration {
	switch retryPolicy {
	case "exponential_backoff":
		d := time.Duration(1<<retryCount) * time.Second
		if d > 10*time.Minute {
			d = 10 * time.Minute
		}
		return d
	default:
		return 0
	}
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
