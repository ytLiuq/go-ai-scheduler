package mysql

import (
	"context"
	"fmt"

	"github.com/example/go-ai-scheduler/internal/model"
	"gorm.io/gorm"
)

// TaskRepository persists tasks in MySQL.
type TaskRepository struct {
	db *gorm.DB
}

// NewTaskRepository creates a TaskRepository.
func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

// CreateTask inserts a task row.
func (r *TaskRepository) CreateTask(ctx context.Context, task *model.Task) error {
	row := taskToRow(task)
	row.ID = 0
	row.Version = 1
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("insert task: %w", err)
	}
	fresh, err := r.GetTask(ctx, row.ID)
	if err != nil {
		return err
	}
	*task = *fresh
	return nil
}

// UpdateTask updates one task by id.
func (r *TaskRepository) UpdateTask(ctx context.Context, task *model.Task) error {
	updates := map[string]any{
		"name":              task.Name,
		"type":              task.Type,
		"cron_expr":         task.CronExpr,
		"payload":           task.Payload,
		"image":             task.Image,
		"status":            task.Status,
		"timeout_seconds":   task.TimeoutSeconds,
		"max_retry":         task.MaxRetry,
		"retry_policy":      task.RetryPolicy,
		"retry_on_errors":   task.RetryOnErrors,
		"route_strategy":    task.RouteStrategy,
		"labels":            task.Labels,
		"next_trigger_time": timeOrNil(task.NextTriggerTime),
		"tenant_id":         task.TenantID,
		"version":           gorm.Expr("version + 1"),
	}
	if err := r.db.WithContext(ctx).Model(&taskRow{}).Where("id = ?", task.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	fresh, err := r.GetTask(ctx, task.ID)
	if err != nil {
		return err
	}
	*task = *fresh
	return nil
}

// DeleteTask removes one task by id.
func (r *TaskRepository) DeleteTask(ctx context.Context, id int64) error {
	if err := r.db.WithContext(ctx).Delete(&taskRow{}, id).Error; err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}

// GetTask loads one task by id.
func (r *TaskRepository) GetTask(ctx context.Context, id int64) (*model.Task, error) {
	var row taskRow
	if err := r.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	return rowToTask(&row), nil
}

// ListTasks loads all tasks ordered by id.
func (r *TaskRepository) ListTasks(ctx context.Context) ([]*model.Task, error) {
	var rows []taskRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	tasks := make([]*model.Task, 0, len(rows))
	for i := range rows {
		tasks = append(tasks, rowToTask(&rows[i]))
	}
	return tasks, nil
}

// ListTasksByTenant loads tasks filtered by tenant.
func (r *TaskRepository) ListTasksByTenant(ctx context.Context, tenantID int64) ([]*model.Task, error) {
	var rows []taskRow
	if err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list tasks by tenant: %w", err)
	}
	tasks := make([]*model.Task, 0, len(rows))
	for i := range rows {
		tasks = append(tasks, rowToTask(&rows[i]))
	}
	return tasks, nil
}

// ListDueTasks loads enabled tasks that should be triggered.
func (r *TaskRepository) ListDueTasks(ctx context.Context, limit int) ([]*model.Task, error) {
	query := r.db.WithContext(ctx).Where("status = ? AND next_trigger_time IS NOT NULL AND next_trigger_time <= NOW()", "enabled").Order("next_trigger_time ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []taskRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list due tasks: %w", err)
	}
	tasks := make([]*model.Task, 0, len(rows))
	for i := range rows {
		tasks = append(tasks, rowToTask(&rows[i]))
	}
	return tasks, nil
}

// AddDependency records a dependency edge.
func (r *TaskRepository) AddDependency(ctx context.Context, taskID, dependsOnTaskID int64) error {
	if err := r.db.WithContext(ctx).Create(&taskDependencyRow{TaskID: taskID, DependsOnTaskID: dependsOnTaskID}).Error; err != nil {
		return fmt.Errorf("add dependency: %w", err)
	}
	return nil
}

// RemoveDependency removes a dependency edge.
func (r *TaskRepository) RemoveDependency(ctx context.Context, taskID, dependsOnTaskID int64) error {
	if err := r.db.WithContext(ctx).Where("task_id = ? AND depends_on_task_id = ?", taskID, dependsOnTaskID).Delete(&taskDependencyRow{}).Error; err != nil {
		return fmt.Errorf("remove dependency: %w", err)
	}
	return nil
}

// ListDownstreamTasks returns task IDs that depend on taskID.
func (r *TaskRepository) ListDownstreamTasks(ctx context.Context, taskID int64) ([]int64, error) {
	var ids []int64
	if err := r.db.WithContext(ctx).Model(&taskDependencyRow{}).Where("depends_on_task_id = ?", taskID).Pluck("task_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("list downstream tasks: %w", err)
	}
	return ids, nil
}

// ListUpstreamDeps returns task IDs that taskID depends on.
func (r *TaskRepository) ListUpstreamDeps(ctx context.Context, taskID int64) ([]int64, error) {
	var ids []int64
	if err := r.db.WithContext(ctx).Model(&taskDependencyRow{}).Where("task_id = ?", taskID).Pluck("depends_on_task_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("list upstream deps: %w", err)
	}
	return ids, nil
}
