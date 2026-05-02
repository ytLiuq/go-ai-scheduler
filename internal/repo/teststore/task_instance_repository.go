package teststore

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
)

var errTaskInstanceNotFound = errors.New("task instance not found")

// TaskInstanceRepository stores task instances for tests.
type TaskInstanceRepository struct {
	mu         sync.RWMutex
	nextID     int64
	instances  map[int64]*model.TaskInstance
	bySchedule map[string]int64
}

// NewTaskInstanceRepository creates an empty task instance repository.
func NewTaskInstanceRepository() *TaskInstanceRepository {
	return &TaskInstanceRepository{
		nextID:     1,
		instances:  make(map[int64]*model.TaskInstance),
		bySchedule: make(map[string]int64),
	}
}

// CreateInstance stores a new task instance.
func (r *TaskInstanceRepository) CreateInstance(_ context.Context, instance *model.TaskInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	copyInstance := *instance
	copyInstance.ID = r.nextID
	copyInstance.CreatedAt = now
	copyInstance.UpdatedAt = now
	r.instances[copyInstance.ID] = &copyInstance
	r.bySchedule[copyInstance.ScheduleInstanceID] = copyInstance.ID
	r.nextID++

	instance.ID = copyInstance.ID
	instance.CreatedAt = copyInstance.CreatedAt
	instance.UpdatedAt = copyInstance.UpdatedAt
	return nil
}

// GetInstanceByScheduleID returns one task instance by schedule id.
func (r *TaskInstanceRepository) GetInstanceByScheduleID(_ context.Context, scheduleID string) (*model.TaskInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.bySchedule[scheduleID]
	if !ok {
		return nil, errTaskInstanceNotFound
	}
	instance, ok := r.instances[id]
	if !ok {
		return nil, errTaskInstanceNotFound
	}
	copyInstance := *instance
	return &copyInstance, nil
}

// GetInstance returns one task instance.
func (r *TaskInstanceRepository) GetInstance(_ context.Context, instanceID int64) (*model.TaskInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instance, ok := r.instances[instanceID]
	if !ok {
		return nil, errTaskInstanceNotFound
	}
	copyInstance := *instance
	return &copyInstance, nil
}

// ListInstances returns all task instances.
func (r *TaskInstanceRepository) ListInstances(_ context.Context) ([]*model.TaskInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instances := make([]*model.TaskInstance, 0, len(r.instances))
	for _, instance := range r.instances {
		copyInstance := *instance
		instances = append(instances, &copyInstance)
	}
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].ID < instances[j].ID
	})
	return instances, nil
}

// ListInstancesByTaskID returns all task instances for a given task.
func (r *TaskInstanceRepository) ListInstancesByTaskID(_ context.Context, taskID int64) ([]*model.TaskInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instances := make([]*model.TaskInstance, 0)
	for _, instance := range r.instances {
		if instance.TaskID != taskID {
			continue
		}
		copyInstance := *instance
		instances = append(instances, &copyInstance)
	}
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].CreatedAt.Before(instances[j].CreatedAt)
	})
	return instances, nil
}

// ListInstancesByTimeRange returns task instances within a time window.
func (r *TaskInstanceRepository) ListInstancesByTimeRange(_ context.Context, from, to time.Time, limit, offset int) ([]*model.TaskInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instances := make([]*model.TaskInstance, 0)
	for _, instance := range r.instances {
		if instance.CreatedAt.Before(from) || instance.CreatedAt.After(to) {
			continue
		}
		copyInstance := *instance
		instances = append(instances, &copyInstance)
	}
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].CreatedAt.After(instances[j].CreatedAt)
	})
	if offset > 0 && offset < len(instances) {
		instances = instances[offset:]
	}
	if limit > 0 && limit < len(instances) {
		instances = instances[:limit]
	}
	return instances, nil
}

// ListInstancesByStatus returns instances filtered by status.
func (r *TaskInstanceRepository) ListInstancesByStatus(_ context.Context, status string, limit int) ([]*model.TaskInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instances := make([]*model.TaskInstance, 0)
	for _, instance := range r.instances {
		if instance.Status != status {
			continue
		}
		copyInstance := *instance
		instances = append(instances, &copyInstance)
	}
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].CreatedAt.Before(instances[j].CreatedAt)
	})
	if limit > 0 && len(instances) > limit {
		instances = instances[:limit]
	}
	return instances, nil
}

// CountInstancesByStatus returns the count of instances with the given status.
func (r *TaskInstanceRepository) CountInstancesByStatus(_ context.Context, status string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var count int
	for _, instance := range r.instances {
		if instance.Status == status {
			count++
		}
	}
	return count, nil
}

// UpdateInstanceStatus updates only the status field.
func (r *TaskInstanceRepository) UpdateInstanceStatus(_ context.Context, instanceID int64, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	instance, ok := r.instances[instanceID]
	if !ok {
		return errTaskInstanceNotFound
	}
	instance.Status = status
	instance.UpdatedAt = time.Now()
	return nil
}

// UpdateInstanceDispatch updates assignment metadata after routing.
func (r *TaskInstanceRepository) UpdateInstanceDispatch(_ context.Context, instanceID int64, workerID string, dispatchTime string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	instance, ok := r.instances[instanceID]
	if !ok {
		return errTaskInstanceNotFound
	}
	instance.WorkerID = workerID
	instance.Status = "dispatched"
	instance.UpdatedAt = time.Now()
	if dispatchTime != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, dispatchTime); err == nil {
			instance.DispatchTime = parsed
		}
	}
	return nil
}

// ListDueRetryInstances returns retry_waiting instances whose next_retry_time has passed.
func (r *TaskInstanceRepository) ListDueRetryInstances(_ context.Context, cutoff time.Time, limit int) ([]*model.TaskInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var instances []*model.TaskInstance
	for _, instance := range r.instances {
		if instance.Status != "retry_waiting" {
			continue
		}
		if !instance.NextRetryTime.IsZero() && instance.NextRetryTime.After(cutoff) {
			continue
		}
		copyInstance := *instance
		instances = append(instances, &copyInstance)
	}
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].CreatedAt.Before(instances[j].CreatedAt)
	})
	if limit > 0 && len(instances) > limit {
		instances = instances[:limit]
	}
	return instances, nil
}

// UpdateInstanceResult updates the final status and error fields.
func (r *TaskInstanceRepository) UpdateInstanceResult(_ context.Context, scheduleID string, status string, errorCode string, errorMessage string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id, ok := r.bySchedule[scheduleID]
	if !ok {
		return errTaskInstanceNotFound
	}
	instance, ok := r.instances[id]
	if !ok {
		return errTaskInstanceNotFound
	}
	instance.Status = status
	instance.ErrorCode = errorCode
	instance.ErrorMessage = errorMessage
	instance.UpdatedAt = time.Now()
	return nil
}

// UpdateInstanceAnalysis stores the AI analysis result.
func (r *TaskInstanceRepository) UpdateInstanceAnalysis(_ context.Context, scheduleID string, analysisJSON string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id, ok := r.bySchedule[scheduleID]
	if !ok {
		return errTaskInstanceNotFound
	}
	instance, ok := r.instances[id]
	if !ok {
		return errTaskInstanceNotFound
	}
	instance.AnalysisJSON = analysisJSON
	instance.UpdatedAt = time.Now()
	return nil
}

// UpdateInstanceTimestamps stores execution start and finish times.
func (r *TaskInstanceRepository) UpdateInstanceTimestamps(_ context.Context, scheduleID string, startedAt, finishedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id, ok := r.bySchedule[scheduleID]
	if !ok {
		return errTaskInstanceNotFound
	}
	instance, ok := r.instances[id]
	if !ok {
		return errTaskInstanceNotFound
	}
	if !startedAt.IsZero() {
		instance.StartedAt = startedAt
	}
	if !finishedAt.IsZero() {
		instance.FinishedAt = finishedAt
	}
	instance.UpdatedAt = time.Now()
	return nil
}
