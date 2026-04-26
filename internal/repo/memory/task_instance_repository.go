package memory

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
)

var errTaskInstanceNotFound = errors.New("task instance not found")

// TaskInstanceRepository stores task instances in memory for local bootstrapping.
type TaskInstanceRepository struct {
	mu        sync.RWMutex
	nextID    int64
	instances map[int64]*model.TaskInstance
	bySchedule map[string]int64
}

// NewTaskInstanceRepository creates an empty task instance repository.
func NewTaskInstanceRepository() *TaskInstanceRepository {
	return &TaskInstanceRepository{
		nextID:    1,
		instances: make(map[int64]*model.TaskInstance),
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
