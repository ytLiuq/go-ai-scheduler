package memory

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
)

var errTaskNotFound = errors.New("task not found")

// TaskRepository is an in-memory task store for local development.
type TaskRepository struct {
	mu     sync.RWMutex
	nextID int64
	tasks  map[int64]*model.Task
}

// NewTaskRepository creates an empty task repository.
func NewTaskRepository() *TaskRepository {
	return &TaskRepository{
		nextID: 1,
		tasks:  make(map[int64]*model.Task),
	}
}

// CreateTask stores a new task.
func (r *TaskRepository) CreateTask(_ context.Context, task *model.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	copyTask := *task
	copyTask.ID = r.nextID
	copyTask.CreatedAt = now
	copyTask.UpdatedAt = now
	copyTask.Version = 1
	r.tasks[copyTask.ID] = &copyTask
	r.nextID++

	task.ID = copyTask.ID
	task.CreatedAt = copyTask.CreatedAt
	task.UpdatedAt = copyTask.UpdatedAt
	task.Version = copyTask.Version
	return nil
}

// UpdateTask updates one task by id.
func (r *TaskRepository) UpdateTask(_ context.Context, task *model.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.tasks[task.ID]
	if !ok {
		return errTaskNotFound
	}

	copyTask := *task
	copyTask.CreatedAt = current.CreatedAt
	copyTask.UpdatedAt = time.Now()
	copyTask.Version = current.Version + 1
	r.tasks[copyTask.ID] = &copyTask

	task.CreatedAt = copyTask.CreatedAt
	task.UpdatedAt = copyTask.UpdatedAt
	task.Version = copyTask.Version
	return nil
}

// GetTask returns one task.
func (r *TaskRepository) GetTask(_ context.Context, id int64) (*model.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	task, ok := r.tasks[id]
	if !ok {
		return nil, errTaskNotFound
	}
	copyTask := *task
	return &copyTask, nil
}

// ListTasks returns all tasks ordered by id.
func (r *TaskRepository) ListTasks(_ context.Context) ([]*model.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tasks := make([]*model.Task, 0, len(r.tasks))
	for _, task := range r.tasks {
		copyTask := *task
		tasks = append(tasks, &copyTask)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})
	return tasks, nil
}

// ListDueTasks returns enabled tasks whose next trigger time has arrived.
func (r *TaskRepository) ListDueTasks(_ context.Context, limit int) ([]*model.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	tasks := make([]*model.Task, 0)
	for _, task := range r.tasks {
		if task.Status != "enabled" {
			continue
		}
		if task.NextTriggerTime.IsZero() || task.NextTriggerTime.After(now) {
			continue
		}
		copyTask := *task
		tasks = append(tasks, &copyTask)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].NextTriggerTime.Before(tasks[j].NextTriggerTime)
	})
	if limit > 0 && len(tasks) > limit {
		tasks = tasks[:limit]
	}
	return tasks, nil
}

