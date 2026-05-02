package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
)

// TaskInstanceRepository persists task instances in MySQL.
type TaskInstanceRepository struct {
	db *sql.DB
}

// NewTaskInstanceRepository creates a TaskInstanceRepository.
func NewTaskInstanceRepository(db *sql.DB) *TaskInstanceRepository {
	return &TaskInstanceRepository{db: db}
}

// CreateInstance inserts a task instance row.
func (r *TaskInstanceRepository) CreateInstance(ctx context.Context, instance *model.TaskInstance) error {
	const query = `
		INSERT INTO task_instance (
			task_id, schedule_instance_id, trigger_time, dispatch_time, worker_id,
			status, retry_count, error_code, error_message, trace_id, next_retry_time
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.ExecContext(ctx, query,
		instance.TaskID,
		instance.ScheduleInstanceID,
		instance.TriggerTime,
		timeOrNull(instance.DispatchTime),
		instance.WorkerID,
		instance.Status,
		instance.RetryCount,
		instance.ErrorCode,
		instance.ErrorMessage,
		instance.TraceID,
		timeOrNull(instance.NextRetryTime),
	)
	if err != nil {
		return fmt.Errorf("insert task instance: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read inserted task instance id: %w", err)
	}
	fresh, err := r.GetInstance(ctx, id)
	if err != nil {
		return err
	}
	*instance = *fresh
	return nil
}

// GetInstance loads one task instance by id.
func (r *TaskInstanceRepository) GetInstance(ctx context.Context, instanceID int64) (*model.TaskInstance, error) {
	const query = `
		SELECT id, task_id, schedule_instance_id, trigger_time, dispatch_time, worker_id,
		       status, retry_count, error_code, error_message, analysis_json, trace_id, next_retry_time, created_at, updated_at
		FROM task_instance
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, instanceID)
	return scanTaskInstance(row)
}

// GetInstanceByScheduleID loads one task instance by schedule id.
func (r *TaskInstanceRepository) GetInstanceByScheduleID(ctx context.Context, scheduleID string) (*model.TaskInstance, error) {
	const query = `
		SELECT id, task_id, schedule_instance_id, trigger_time, dispatch_time, worker_id,
		       status, retry_count, error_code, error_message, analysis_json, trace_id, next_retry_time, created_at, updated_at
		FROM task_instance
		WHERE schedule_instance_id = ?
	`
	row := r.db.QueryRowContext(ctx, query, scheduleID)
	return scanTaskInstance(row)
}

// ListInstances returns all task instances.
func (r *TaskInstanceRepository) ListInstances(ctx context.Context) ([]*model.TaskInstance, error) {
	const query = `
		SELECT id, task_id, schedule_instance_id, trigger_time, dispatch_time, worker_id,
		       status, retry_count, error_code, error_message, analysis_json, trace_id, next_retry_time, created_at, updated_at
		FROM task_instance
		ORDER BY id
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list task instances: %w", err)
	}
	defer rows.Close()
	return scanTaskInstances(rows)
}

// ListInstancesByTaskID returns all task instances for a given task.
func (r *TaskInstanceRepository) ListInstancesByTaskID(ctx context.Context, taskID int64) ([]*model.TaskInstance, error) {
	const query = `
		SELECT id, task_id, schedule_instance_id, trigger_time, dispatch_time, worker_id,
		       status, retry_count, error_code, error_message, analysis_json, trace_id, next_retry_time, created_at, updated_at
		FROM task_instance
		WHERE task_id = ?
		ORDER BY created_at
	`
	rows, err := r.db.QueryContext(ctx, query, taskID)
	if err != nil {
		return nil, fmt.Errorf("list instances by task id: %w", err)
	}
	defer rows.Close()
	return scanTaskInstances(rows)
}

// ListInstancesByStatus returns task instances filtered by status.
func (r *TaskInstanceRepository) ListInstancesByStatus(ctx context.Context, status string, limit int) ([]*model.TaskInstance, error) {
	const query = `
		SELECT id, task_id, schedule_instance_id, trigger_time, dispatch_time, worker_id,
		       status, retry_count, error_code, error_message, analysis_json, trace_id, next_retry_time, created_at, updated_at
		FROM task_instance
		WHERE status = ?
		ORDER BY created_at
		LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list task instances by status: %w", err)
	}
	defer rows.Close()
	return scanTaskInstances(rows)
}

// ListDueRetryInstances returns retry_waiting instances whose next_retry_time has passed.
func (r *TaskInstanceRepository) ListDueRetryInstances(ctx context.Context, cutoff time.Time, limit int) ([]*model.TaskInstance, error) {
	const query = `
		SELECT id, task_id, schedule_instance_id, trigger_time, dispatch_time, worker_id,
		       status, retry_count, error_code, error_message, analysis_json, trace_id, next_retry_time, created_at, updated_at
		FROM task_instance
		WHERE status = 'retry_waiting' AND (next_retry_time IS NULL OR next_retry_time <= ?)
		ORDER BY created_at
		LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("list due retry instances: %w", err)
	}
	defer rows.Close()
	return scanTaskInstances(rows)
}

// CountInstancesByStatus returns the count of instances with the given status.
func (r *TaskInstanceRepository) CountInstancesByStatus(ctx context.Context, status string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM task_instance WHERE status = ?`, status,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count instances by status: %w", err)
	}
	return count, nil
}

// UpdateInstanceStatus updates only the status field.
func (r *TaskInstanceRepository) UpdateInstanceStatus(ctx context.Context, instanceID int64, status string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE task_instance SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, instanceID,
	)
	if err != nil {
		return fmt.Errorf("update task instance status: %w", err)
	}
	return nil
}

// UpdateInstanceDispatch updates dispatch metadata.
func (r *TaskInstanceRepository) UpdateInstanceDispatch(ctx context.Context, instanceID int64, workerID string, dispatchTime string) error {
	parsedDispatchTime, err := parseDispatchTime(dispatchTime)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE task_instance SET worker_id = ?, dispatch_time = ?, status = 'dispatched', updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		workerID, parsedDispatchTime, instanceID,
	)
	if err != nil {
		return fmt.Errorf("update task instance dispatch: %w", err)
	}
	return nil
}

// UpdateInstanceResult updates the final result state.
func (r *TaskInstanceRepository) UpdateInstanceResult(ctx context.Context, scheduleID string, status string, errorCode string, errorMessage string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE task_instance SET status = ?, error_code = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP WHERE schedule_instance_id = ?`,
		status, errorCode, errorMessage, scheduleID,
	)
	if err != nil {
		return fmt.Errorf("update task instance result: %w", err)
	}
	return nil
}

func scanTaskInstance(scanner interface{ Scan(dest ...any) error }) (*model.TaskInstance, error) {
	var instance model.TaskInstance
	var dispatchTime sql.NullTime
	var nextRetryTime sql.NullTime
	if err := scanner.Scan(
		&instance.ID,
		&instance.TaskID,
		&instance.ScheduleInstanceID,
		&instance.TriggerTime,
		&dispatchTime,
		&instance.WorkerID,
		&instance.Status,
		&instance.RetryCount,
		&instance.ErrorCode,
		&instance.ErrorMessage,
		&instance.AnalysisJSON,
		&instance.TraceID,
		&nextRetryTime,
		&instance.CreatedAt,
		&instance.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan task instance: %w", err)
	}
	if dispatchTime.Valid {
		instance.DispatchTime = dispatchTime.Time
	}
	if nextRetryTime.Valid {
		instance.NextRetryTime = nextRetryTime.Time
	}
	return &instance, nil
}

func scanTaskInstances(rows *sql.Rows) ([]*model.TaskInstance, error) {
	instances := make([]*model.TaskInstance, 0)
	for rows.Next() {
		instance, err := scanTaskInstance(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, instance)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task instances: %w", err)
	}
	return instances, nil
}

// UpdateInstanceAnalysis stores the AI analysis result for a failed instance.
func (r *TaskInstanceRepository) UpdateInstanceAnalysis(ctx context.Context, scheduleID string, analysisJSON string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE task_instance SET analysis_json = ?, updated_at = CURRENT_TIMESTAMP WHERE schedule_instance_id = ?`,
		analysisJSON, scheduleID,
	)
	if err != nil {
		return fmt.Errorf("update instance analysis: %w", err)
	}
	return nil
}

func parseDispatchTime(value string) (sql.NullTime, error) {
	if value == "" {
		return sql.NullTime{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return sql.NullTime{}, fmt.Errorf("parse dispatch time: %w", err)
	}
	return sql.NullTime{Time: parsed, Valid: true}, nil
}
