package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/pkg/cronexpr"
	"github.com/example/go-ai-scheduler/internal/repo"
)

var (
	ErrTaskNameRequired = errors.New("task name is required")
	ErrTaskTypeRequired = errors.New("task type is required")
	ErrTaskIDRequired   = errors.New("task id is required")
	ErrInvalidCronExpr  = errors.New("invalid cron expression")
)

// TaskUpsertRequest contains task create and update input.
type TaskUpsertRequest struct {
	Name            string    `json:"name"`
	Type            string    `json:"type"`
	CronExpr        string    `json:"cron_expr"`
	Payload         string    `json:"payload"`
	Status          string    `json:"status"`
	TimeoutSeconds  int       `json:"timeout_seconds"`
	MaxRetry        int       `json:"max_retry"`
	RetryPolicy     string    `json:"retry_policy"`
	RouteStrategy   string    `json:"route_strategy"`
	NextTriggerTime time.Time `json:"next_trigger_time"`
	TenantID        int64     `json:"tenant_id"`
}

// TaskService manages task definitions.
type TaskService struct {
	repo repo.TaskRepository
}

// NewTaskService creates a TaskService.
func NewTaskService(repo repo.TaskRepository) *TaskService {
	return &TaskService{repo: repo}
}

// CreateTask validates and stores a new task.
func (s *TaskService) CreateTask(ctx context.Context, req TaskUpsertRequest) (*model.Task, error) {
	task, err := buildTask(0, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// UpdateTask validates and updates an existing task.
func (s *TaskService) UpdateTask(ctx context.Context, id int64, req TaskUpsertRequest) (*model.Task, error) {
	if id <= 0 {
		return nil, ErrTaskIDRequired
	}
	current, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	task, err := buildTask(id, req)
	if err != nil {
		return nil, err
	}
	task.CreatedAt = current.CreatedAt
	task.Version = current.Version
	if task.NextTriggerTime.IsZero() {
		task.NextTriggerTime = current.NextTriggerTime
		if task.CronExpr != current.CronExpr && task.CronExpr != "" {
			nextTrigger, nextErr := cronexpr.NextAfter(time.Now(), task.CronExpr)
			if nextErr != nil {
				return nil, ErrInvalidCronExpr
			}
			task.NextTriggerTime = nextTrigger
		}
	}
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// DeleteTask removes one task.
func (s *TaskService) DeleteTask(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrTaskIDRequired
	}
	return s.repo.DeleteTask(ctx, id)
}

// GetTask returns one task.
func (s *TaskService) GetTask(ctx context.Context, id int64) (*model.Task, error) {
	if id <= 0 {
		return nil, ErrTaskIDRequired
	}
	return s.repo.GetTask(ctx, id)
}

// ListTasks returns all tasks.
func (s *TaskService) ListTasks(ctx context.Context) ([]*model.Task, error) {
	return s.repo.ListTasks(ctx)
}

func buildTask(id int64, req TaskUpsertRequest) (*model.Task, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, ErrTaskNameRequired
	}
	if strings.TrimSpace(req.Type) == "" {
		return nil, ErrTaskTypeRequired
	}
	if req.Status == "" {
		req.Status = "enabled"
	}
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 300
	}
	if req.MaxRetry < 0 {
		req.MaxRetry = 0
	}
	if req.RetryPolicy == "" {
		req.RetryPolicy = "fixed_interval"
	}
	if req.RouteStrategy == "" {
		req.RouteStrategy = "round_robin"
	}
	if req.CronExpr != "" {
		if err := cronexpr.Validate(req.CronExpr); err != nil {
			return nil, ErrInvalidCronExpr
		}
		if req.NextTriggerTime.IsZero() {
			nextTrigger, nextErr := cronexpr.NextAfter(time.Now(), req.CronExpr)
			if nextErr != nil {
				return nil, ErrInvalidCronExpr
			}
			req.NextTriggerTime = nextTrigger
		}
	}

	return &model.Task{
		ID:              id,
		Name:            req.Name,
		Type:            req.Type,
		CronExpr:        req.CronExpr,
		Payload:         req.Payload,
		Status:          req.Status,
		TimeoutSeconds:  req.TimeoutSeconds,
		MaxRetry:        req.MaxRetry,
		RetryPolicy:     req.RetryPolicy,
		RouteStrategy:   req.RouteStrategy,
		NextTriggerTime: req.NextTriggerTime,
		TenantID:        req.TenantID,
	}, nil
}
