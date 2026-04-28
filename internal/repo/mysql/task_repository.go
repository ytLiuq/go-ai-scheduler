package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/example/go-ai-scheduler/internal/model"
)

// TaskRepository persists tasks in MySQL.
type TaskRepository struct {
	db *sql.DB
}

// NewTaskRepository creates a TaskRepository.
func NewTaskRepository(db *sql.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

// CreateTask inserts a task row.
func (r *TaskRepository) CreateTask(ctx context.Context, task *model.Task) error {
	const query = `
		INSERT INTO task (
			name, type, cron_expr, payload, status, timeout_seconds,
			max_retry, retry_policy, retry_on_errors, route_strategy, labels, next_trigger_time, tenant_id, version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.ExecContext(ctx, query,
		task.Name,
		task.Type,
		task.CronExpr,
		task.Payload,
		task.Status,
		task.TimeoutSeconds,
		task.MaxRetry,
		task.RetryPolicy,
		task.RetryOnErrors,
		task.RouteStrategy,
		task.Labels,
		timeOrNull(task.NextTriggerTime),
		task.TenantID,
		1,
	)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read inserted task id: %w", err)
	}
	fresh, err := r.GetTask(ctx, id)
	if err != nil {
		return err
	}
	*task = *fresh
	return nil
}

// UpdateTask updates one task by id.
func (r *TaskRepository) UpdateTask(ctx context.Context, task *model.Task) error {
	const query = `
		UPDATE task
		SET name = ?, type = ?, cron_expr = ?, payload = ?, status = ?, timeout_seconds = ?,
		    max_retry = ?, retry_policy = ?, retry_on_errors = ?, route_strategy = ?, labels = ?,
		    next_trigger_time = ?, tenant_id = ?, version = version + 1
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, query,
		task.Name,
		task.Type,
		task.CronExpr,
		task.Payload,
		task.Status,
		task.TimeoutSeconds,
		task.MaxRetry,
		task.RetryPolicy,
		task.RetryOnErrors,
		task.RouteStrategy,
		task.Labels,
		timeOrNull(task.NextTriggerTime),
		task.TenantID,
		task.ID,
	)
	if err != nil {
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
	const query = `DELETE FROM task WHERE id = ?`
	if _, err := r.db.ExecContext(ctx, query, id); err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}

// GetTask loads one task by id.
func (r *TaskRepository) GetTask(ctx context.Context, id int64) (*model.Task, error) {
	const query = `
		SELECT id, name, type, cron_expr, payload, status, timeout_seconds, max_retry,
		       retry_policy, retry_on_errors, route_strategy, labels, next_trigger_time, tenant_id, version,
		       created_at, updated_at
		FROM task
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanTask(row)
}

// ListTasks loads all tasks ordered by id.
func (r *TaskRepository) ListTasks(ctx context.Context) ([]*model.Task, error) {
	const query = `
		SELECT id, name, type, cron_expr, payload, status, timeout_seconds, max_retry,
		       retry_policy, retry_on_errors, route_strategy, labels, next_trigger_time, tenant_id, version,
		       created_at, updated_at
		FROM task
		ORDER BY id
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// ListTasksByTenant loads tasks filtered by tenant.
func (r *TaskRepository) ListTasksByTenant(ctx context.Context, tenantID int64) ([]*model.Task, error) {
	const query = `
		SELECT id, name, type, cron_expr, payload, status, timeout_seconds, max_retry,
		       retry_policy, retry_on_errors, route_strategy, labels, next_trigger_time, tenant_id, version,
		       created_at, updated_at
		FROM task
		WHERE tenant_id = ?
		ORDER BY id
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list tasks by tenant: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// ListDueTasks loads enabled tasks that should be triggered.
func (r *TaskRepository) ListDueTasks(ctx context.Context, limit int) ([]*model.Task, error) {
	const query = `
		SELECT id, name, type, cron_expr, payload, status, timeout_seconds, max_retry,
		       retry_policy, retry_on_errors, route_strategy, labels, next_trigger_time, tenant_id, version,
		       created_at, updated_at
		FROM task
		WHERE status = 'enabled' AND next_trigger_time IS NOT NULL AND next_trigger_time <= NOW()
		ORDER BY next_trigger_time ASC
		LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list due tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

func scanTask(scanner interface{ Scan(dest ...any) error }) (*model.Task, error) {
	var task model.Task
	var nextTrigger sql.NullTime
	if err := scanner.Scan(
		&task.ID,
		&task.Name,
		&task.Type,
		&task.CronExpr,
		&task.Payload,
		&task.Status,
		&task.TimeoutSeconds,
		&task.MaxRetry,
		&task.RetryPolicy,
		&task.RetryOnErrors,
		&task.RouteStrategy,
		&task.Labels,
		&nextTrigger,
		&task.TenantID,
		&task.Version,
		&task.CreatedAt,
		&task.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan task: %w", err)
	}
	if nextTrigger.Valid {
		task.NextTriggerTime = nextTrigger.Time
	}
	return &task, nil
}

func scanTasks(rows *sql.Rows) ([]*model.Task, error) {
	tasks := make([]*model.Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return tasks, nil
	}

	// AddDependency records a dependency edge.
	func (r *TaskRepository) AddDependency(ctx context.Context, taskID, dependsOnTaskID int64) error {
		_, err := r.db.ExecContext(ctx, "INSERT INTO task_dependency (task_id, depends_on_task_id) VALUES (?, ?)", taskID, dependsOnTaskID)
		return err
	}

	// RemoveDependency removes a dependency edge.
	func (r *TaskRepository) RemoveDependency(ctx context.Context, taskID, dependsOnTaskID int64) error {
		_, err := r.db.ExecContext(ctx, "DELETE FROM task_dependency WHERE task_id = ? AND depends_on_task_id = ?", taskID, dependsOnTaskID)
		return err
	}

	// ListDownstreamTasks returns task IDs that depend on taskID.
	func (r *TaskRepository) ListDownstreamTasks(ctx context.Context, taskID int64) ([]int64, error) {
		rows, err := r.db.QueryContext(ctx, "SELECT task_id FROM task_dependency WHERE depends_on_task_id = ?", taskID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	}

	// ListUpstreamDeps returns task IDs that taskID depends on.
	func (r *TaskRepository) ListUpstreamDeps(ctx context.Context, taskID int64) ([]int64, error) {
		rows, err := r.db.QueryContext(ctx, "SELECT depends_on_task_id FROM task_dependency WHERE task_id = ?", taskID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	}
