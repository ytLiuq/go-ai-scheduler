package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
	"gorm.io/gorm"
)

// TaskInstanceRepository persists task instances in MySQL.
type TaskInstanceRepository struct {
	db *gorm.DB
}

// NewTaskInstanceRepository creates a TaskInstanceRepository.
func NewTaskInstanceRepository(db *gorm.DB) *TaskInstanceRepository {
	return &TaskInstanceRepository{db: db}
}

// CreateInstance inserts a task instance row.
func (r *TaskInstanceRepository) CreateInstance(ctx context.Context, instance *model.TaskInstance) error {
	row := taskInstanceToRow(instance)
	row.ID = 0
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("insert task instance: %w", err)
	}
	fresh, err := r.GetInstance(ctx, row.ID)
	if err != nil {
		return err
	}
	*instance = *fresh
	return nil
}

// GetInstance loads one task instance by id.
func (r *TaskInstanceRepository) GetInstance(ctx context.Context, instanceID int64) (*model.TaskInstance, error) {
	var row taskInstanceRow
	if err := r.db.WithContext(ctx).First(&row, instanceID).Error; err != nil {
		return nil, fmt.Errorf("get task instance: %w", err)
	}
	return rowToTaskInstance(&row), nil
}

// GetInstanceByScheduleID loads one task instance by schedule id.
func (r *TaskInstanceRepository) GetInstanceByScheduleID(ctx context.Context, scheduleID string) (*model.TaskInstance, error) {
	var row taskInstanceRow
	if err := r.db.WithContext(ctx).Where("schedule_instance_id = ?", scheduleID).First(&row).Error; err != nil {
		return nil, fmt.Errorf("get task instance by schedule id: %w", err)
	}
	return rowToTaskInstance(&row), nil
}

// ListInstances returns all task instances.
func (r *TaskInstanceRepository) ListInstances(ctx context.Context) ([]*model.TaskInstance, error) {
	var rows []taskInstanceRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list task instances: %w", err)
	}
	return mapTaskInstances(rows), nil
}

// ListInstancesByTaskID returns all task instances for a given task.
func (r *TaskInstanceRepository) ListInstancesByTaskID(ctx context.Context, taskID int64) ([]*model.TaskInstance, error) {
	var rows []taskInstanceRow
	if err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Order("created_at").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list instances by task id: %w", err)
	}
	return mapTaskInstances(rows), nil
}

// ListInstancesByTimeRange returns task instances within a time window.
func (r *TaskInstanceRepository) ListInstancesByTimeRange(ctx context.Context, from, to time.Time, limit, offset int) ([]*model.TaskInstance, error) {
	query := r.db.WithContext(ctx).Where("created_at >= ? AND created_at <= ?", from, to).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	var rows []taskInstanceRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list instances by time range: %w", err)
	}
	return mapTaskInstances(rows), nil
}

// ListInstancesByStatus returns task instances filtered by status.
func (r *TaskInstanceRepository) ListInstancesByStatus(ctx context.Context, status string, limit int) ([]*model.TaskInstance, error) {
	query := r.db.WithContext(ctx).Where("status = ?", status).Order("created_at")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []taskInstanceRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list task instances by status: %w", err)
	}
	return mapTaskInstances(rows), nil
}

// ListDueRetryInstances returns retry_waiting instances whose next_retry_time has passed.
func (r *TaskInstanceRepository) ListDueRetryInstances(ctx context.Context, cutoff time.Time, limit int) ([]*model.TaskInstance, error) {
	query := r.db.WithContext(ctx).
		Where("status = ? AND (next_retry_time IS NULL OR next_retry_time <= ?)", "retry_waiting", cutoff).
		Order("created_at")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []taskInstanceRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list due retry instances: %w", err)
	}
	return mapTaskInstances(rows), nil
}

// CountInstancesByStatus returns the count of instances with the given status.
func (r *TaskInstanceRepository) CountInstancesByStatus(ctx context.Context, status string) (int, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&taskInstanceRow{}).Where("status = ?", status).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count instances by status: %w", err)
	}
	return int(count), nil
}

// UpdateInstanceStatus updates only the status field.
func (r *TaskInstanceRepository) UpdateInstanceStatus(ctx context.Context, instanceID int64, status string) error {
	if err := r.db.WithContext(ctx).Model(&taskInstanceRow{}).Where("id = ?", instanceID).Updates(map[string]any{"status": status}).Error; err != nil {
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
	if err := r.db.WithContext(ctx).Model(&taskInstanceRow{}).Where("id = ?", instanceID).Updates(map[string]any{
		"worker_id":     workerID,
		"dispatch_time": parsedDispatchTime,
		"status":        "dispatched",
	}).Error; err != nil {
		return fmt.Errorf("update task instance dispatch: %w", err)
	}
	return nil
}

// UpdateInstanceResult updates the final result state.
func (r *TaskInstanceRepository) UpdateInstanceResult(ctx context.Context, scheduleID string, status string, errorCode string, errorMessage string) error {
	if err := r.db.WithContext(ctx).Model(&taskInstanceRow{}).Where("schedule_instance_id = ?", scheduleID).Updates(map[string]any{
		"status":        status,
		"error_code":    errorCode,
		"error_message": errorMessage,
	}).Error; err != nil {
		return fmt.Errorf("update task instance result: %w", err)
	}
	return nil
}

// UpdateInstanceAnalysis stores the AI analysis result for a failed instance.
func (r *TaskInstanceRepository) UpdateInstanceAnalysis(ctx context.Context, scheduleID string, analysisJSON string) error {
	if err := r.db.WithContext(ctx).Model(&taskInstanceRow{}).Where("schedule_instance_id = ?", scheduleID).Update("analysis_json", analysisJSON).Error; err != nil {
		return fmt.Errorf("update instance analysis: %w", err)
	}
	return nil
}

// UpdateInstanceTimestamps stores execution start and finish times.
func (r *TaskInstanceRepository) UpdateInstanceTimestamps(ctx context.Context, scheduleID string, startedAt, finishedAt time.Time) error {
	if err := r.db.WithContext(ctx).Model(&taskInstanceRow{}).Where("schedule_instance_id = ?", scheduleID).Updates(map[string]any{
		"started_at":  timeOrNil(startedAt),
		"finished_at": timeOrNil(finishedAt),
	}).Error; err != nil {
		return fmt.Errorf("update instance timestamps: %w", err)
	}
	return nil
}

func parseDispatchTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, fmt.Errorf("parse dispatch time: %w", err)
	}
	return &parsed, nil
}

func mapTaskInstances(rows []taskInstanceRow) []*model.TaskInstance {
	instances := make([]*model.TaskInstance, 0, len(rows))
	for i := range rows {
		instances = append(instances, rowToTaskInstance(&rows[i]))
	}
	return instances
}
