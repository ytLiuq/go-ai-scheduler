package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/tenant"
	"github.com/example/go-ai-scheduler/internal/pkg/cronexpr"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	"github.com/example/go-ai-scheduler/internal/repo"
)

var (
	ErrTaskNameRequired = errors.New("task name is required")
	ErrTaskTypeRequired = errors.New("task type is required")
	ErrTaskIDRequired   = errors.New("task id is required")
	ErrInvalidCronExpr  = errors.New("invalid cron expression")
	ErrTaskNotEnabled   = errors.New("task is not enabled")
	ErrTaskNotOwned     = errors.New("task does not belong to this tenant")
)

// TaskUpsertRequest contains task create and update input.
type TaskUpsertRequest struct {
	Name            string    `json:"name"`
	Type            string    `json:"type"`
	CronExpr        string    `json:"cron_expr"`
	Payload         string    `json:"payload"`
	Image           string    `json:"image"`
	Status          string    `json:"status"`
	TimeoutSeconds  int       `json:"timeout_seconds"`
	MaxRetry        int       `json:"max_retry"`
	RetryPolicy     string    `json:"retry_policy"`
	RetryOnErrors   string            `json:"retry_on_errors"`
	RouteStrategy   string            `json:"route_strategy"`
	Labels          map[string]string `json:"labels"`
	NextTriggerTime time.Time         `json:"next_trigger_time"`
	TenantID        int64             `json:"tenant_id"`
	DependsOn       []int64           `json:"depends_on"`
}

// TaskService manages task definitions.
type TaskService struct {
	repo    repo.TaskRepository
	auditor *Auditor
}

// NewTaskService creates a TaskService.
func NewTaskService(repo repo.TaskRepository, auditor *Auditor) *TaskService {
	return &TaskService{repo: repo, auditor: auditor}
}

func (s *TaskService) audit(ctx context.Context, action string, id int64, detail string) {
	if s.auditor == nil {
		return
	}
	s.auditor.Record(ctx, AuditEntry{
		Action:       action,
		ResourceType: "task",
		ResourceID:   strconv.FormatInt(id, 10),
		Detail:       detail,
	})
}

func (s *TaskService) auditErr(ctx context.Context, action string, id int64, err error) {
	if s.auditor == nil {
		return
	}
	s.auditor.Record(ctx, AuditEntry{
		Action:       action,
		ResourceType: "task",
		ResourceID:   strconv.FormatInt(id, 10),
		Detail:       err.Error(),
		Result:       "error",
	})
}

// CreateTask validates and stores a new task.
func (s *TaskService) CreateTask(ctx context.Context, req TaskUpsertRequest) (*model.Task, error) {
	task, err := buildTask(0, req)
	if err != nil {
		return nil, err
	}
	if task.TenantID == 0 {
		task.TenantID = tenant.FromContext(ctx)
	}
	if err := s.repo.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	if err := s.syncDependencies(ctx, task.ID, req.DependsOn); err != nil {
		return nil, err
	}
	metrics.DefaultRegistry.IncCounter("tasks_mutations_total", map[string]string{"action": "create"})
	s.audit(ctx, "task.create", task.ID, task.Name)
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
	if err := s.checkTenant(ctx, current); err != nil {
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
	if err := s.syncDependencies(ctx, task.ID, req.DependsOn); err != nil {
		return nil, err
	}
	metrics.DefaultRegistry.IncCounter("tasks_mutations_total", map[string]string{"action": "update"})
	s.audit(ctx, "task.update", task.ID, task.Name)
	return task, nil
}

// DeleteTask removes one task, enforcing tenant ownership.
func (s *TaskService) DeleteTask(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrTaskIDRequired
	}
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return err
	}
	if err := s.checkTenant(ctx, task); err != nil {
		return err
	}
	if err := s.repo.DeleteTask(ctx, id); err != nil {
		return err
	}
	metrics.DefaultRegistry.IncCounter("tasks_mutations_total", map[string]string{"action": "delete"})
	s.audit(ctx, "task.delete", id, "")
	return nil
}

// PauseTask sets a task status to disabled so it is no longer triggered.
func (s *TaskService) PauseTask(ctx context.Context, id int64) (*model.Task, error) {
	if id <= 0 {
		return nil, ErrTaskIDRequired
	}
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.checkTenant(ctx, task); err != nil {
		return nil, err
	}
	task.Status = "disabled"
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	metrics.DefaultRegistry.IncCounter("tasks_mutations_total", map[string]string{"action": "pause"})
	s.audit(ctx, "task.pause", id, task.Name)
	return task, nil
}

// ResumeTask re-enables a disabled task and recalculates the next trigger time.
func (s *TaskService) ResumeTask(ctx context.Context, id int64) (*model.Task, error) {
	if id <= 0 {
		return nil, ErrTaskIDRequired
	}
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.checkTenant(ctx, task); err != nil {
		return nil, err
	}
	task.Status = "enabled"
	if task.CronExpr != "" {
		nextTrigger, nextErr := cronexpr.NextAfter(time.Now(), task.CronExpr)
		if nextErr != nil {
			return nil, ErrInvalidCronExpr
		}
		task.NextTriggerTime = nextTrigger
	}
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	metrics.DefaultRegistry.IncCounter("tasks_mutations_total", map[string]string{"action": "resume"})
	s.audit(ctx, "task.resume", id, task.Name)
	return task, nil
}

// TriggerTask forces a one-off execution by setting the next trigger time to the past,
// causing the scheduler to pick it up on the next scan.
func (s *TaskService) TriggerTask(ctx context.Context, id int64) (*model.Task, error) {
	if id <= 0 {
		return nil, ErrTaskIDRequired
	}
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.checkTenant(ctx, task); err != nil {
		return nil, err
	}
	if task.Status != "enabled" {
		return nil, ErrTaskNotEnabled
	}
	task.NextTriggerTime = time.Now().Add(-time.Second)
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	metrics.DefaultRegistry.IncCounter("tasks_mutations_total", map[string]string{"action": "trigger"})
	s.audit(ctx, "task.trigger", id, task.Name)
	return task, nil
}

// GetTask returns one task, enforcing tenant ownership.
func (s *TaskService) GetTask(ctx context.Context, id int64) (*model.Task, error) {
	if id <= 0 {
		return nil, ErrTaskIDRequired
	}
	task, err := s.repo.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.checkTenant(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// ListTasks returns all tasks, optionally filtered by tenant.
func (s *TaskService) ListTasks(ctx context.Context) ([]*model.Task, error) {
	if tid := tenant.FromContext(ctx); tid != 0 {
		return s.repo.ListTasksByTenant(ctx, tid)
	}
	return s.repo.ListTasks(ctx)
}

// syncDependencies reconciles the dependency list for a task.
func (s *TaskService) syncDependencies(ctx context.Context, taskID int64, dependsOn []int64) error {
	existing, err := s.repo.ListUpstreamDeps(ctx, taskID)
	if err != nil {
		return err
	}
	existingSet := make(map[int64]bool)
	for _, id := range existing {
		existingSet[id] = true
	}
	wantedSet := make(map[int64]bool)
	for _, id := range dependsOn {
		wantedSet[id] = true
	}
	// Remove deps that are no longer wanted.
	for id := range existingSet {
		if !wantedSet[id] {
			if err := s.repo.RemoveDependency(ctx, taskID, id); err != nil {
				return err
			}
		}
	}
	// Add new deps.
	for id := range wantedSet {
		if !existingSet[id] {
			if err := s.repo.AddDependency(ctx, taskID, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *TaskService) checkTenant(ctx context.Context, task *model.Task) error {
	tid := tenant.FromContext(ctx)
	if tid == 0 || task.TenantID == 0 {
		return nil
	}
	if task.TenantID != tid {
		return ErrTaskNotOwned
	}
	return nil
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
		Image:           req.Image,
		Status:          req.Status,
		TimeoutSeconds:  req.TimeoutSeconds,
		MaxRetry:        req.MaxRetry,
		RetryPolicy:     req.RetryPolicy,
		RetryOnErrors:   req.RetryOnErrors,
		RouteStrategy:   req.RouteStrategy,
		Labels:          model.EncodeLabels(req.Labels),
		NextTriggerTime: req.NextTriggerTime,
		TenantID:        req.TenantID,
	}, nil
}

// DAGNode represents a task in the dependency graph.
type DAGNode struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Status  string `json:"status"`
	DependsOn []int64 `json:"depends_on"`
}

// GetDAG returns all tasks and their dependencies for DAG visualization.
func (s *TaskService) GetDAG(ctx context.Context) ([]DAGNode, error) {
	tasks, err := s.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]DAGNode, 0, len(tasks))
	for _, t := range tasks {
		deps, _ := s.repo.ListUpstreamDeps(ctx, t.ID)
		if deps == nil {
			deps = []int64{}
		}
		nodes = append(nodes, DAGNode{
			ID: t.ID, Name: t.Name, Type: t.Type, Status: t.Status, DependsOn: deps,
		})
	}
	return nodes, nil
}
