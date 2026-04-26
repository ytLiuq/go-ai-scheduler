package repo

import (
	"context"

	"github.com/example/go-ai-scheduler/internal/model"
)

// TaskRepository persists task definitions.
type TaskRepository interface {
	CreateTask(ctx context.Context, task *model.Task) error
	UpdateTask(ctx context.Context, task *model.Task) error
	GetTask(ctx context.Context, id int64) (*model.Task, error)
	ListTasks(ctx context.Context) ([]*model.Task, error)
	ListDueTasks(ctx context.Context, limit int) ([]*model.Task, error)
}

// TaskInstanceRepository persists generated task instances.
type TaskInstanceRepository interface {
	CreateInstance(ctx context.Context, instance *model.TaskInstance) error
	GetInstance(ctx context.Context, instanceID int64) (*model.TaskInstance, error)
	GetInstanceByScheduleID(ctx context.Context, scheduleID string) (*model.TaskInstance, error)
	ListInstances(ctx context.Context) ([]*model.TaskInstance, error)
	ListInstancesByStatus(ctx context.Context, status string, limit int) ([]*model.TaskInstance, error)
	UpdateInstanceStatus(ctx context.Context, instanceID int64, status string) error
	UpdateInstanceDispatch(ctx context.Context, instanceID int64, workerID string, dispatchTime string) error
	UpdateInstanceResult(ctx context.Context, scheduleID string, status string, errorCode string, errorMessage string) error
}

// WorkerRepository persists worker liveness and runtime metadata.
type WorkerRepository interface {
	UpsertWorker(ctx context.Context, worker *model.WorkerNode) error
	GetWorker(ctx context.Context, id string) (*model.WorkerNode, error)
	ListWorkers(ctx context.Context) ([]*model.WorkerNode, error)
	ListAvailableWorkers(ctx context.Context) ([]*model.WorkerNode, error)
}
