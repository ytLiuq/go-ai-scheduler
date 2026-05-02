package repo

import (
	"context"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
)

// TaskRepository persists task definitions and dependencies.
type TaskRepository interface {
	CreateTask(ctx context.Context, task *model.Task) error
	UpdateTask(ctx context.Context, task *model.Task) error
	DeleteTask(ctx context.Context, id int64) error
	GetTask(ctx context.Context, id int64) (*model.Task, error)
	ListTasks(ctx context.Context) ([]*model.Task, error)
	ListTasksByTenant(ctx context.Context, tenantID int64) ([]*model.Task, error)
	ListDueTasks(ctx context.Context, limit int) ([]*model.Task, error)

	// Dependency management.
	AddDependency(ctx context.Context, taskID, dependsOnTaskID int64) error
	RemoveDependency(ctx context.Context, taskID, dependsOnTaskID int64) error
	ListDownstreamTasks(ctx context.Context, taskID int64) ([]int64, error)
	ListUpstreamDeps(ctx context.Context, taskID int64) ([]int64, error)
}

// TaskInstanceRepository persists generated task instances.
type TaskInstanceRepository interface {
	CreateInstance(ctx context.Context, instance *model.TaskInstance) error
	GetInstance(ctx context.Context, instanceID int64) (*model.TaskInstance, error)
	GetInstanceByScheduleID(ctx context.Context, scheduleID string) (*model.TaskInstance, error)
	ListInstances(ctx context.Context) ([]*model.TaskInstance, error)
	ListInstancesByTaskID(ctx context.Context, taskID int64) ([]*model.TaskInstance, error)
	ListInstancesByStatus(ctx context.Context, status string, limit int) ([]*model.TaskInstance, error)
	ListDueRetryInstances(ctx context.Context, cutoff time.Time, limit int) ([]*model.TaskInstance, error)
	CountInstancesByStatus(ctx context.Context, status string) (int, error)
	UpdateInstanceStatus(ctx context.Context, instanceID int64, status string) error
	UpdateInstanceDispatch(ctx context.Context, instanceID int64, workerID string, dispatchTime string) error
	UpdateInstanceResult(ctx context.Context, scheduleID string, status string, errorCode string, errorMessage string) error
	UpdateInstanceAnalysis(ctx context.Context, scheduleID string, analysisJSON string) error
}

// AIAnalysisRepository persists AI analysis audit records.
type AIAnalysisRepository interface {
	CreateRecord(ctx context.Context, record *model.AIAnalysisRecord) error
}

// WorkerRepository persists worker liveness and runtime metadata.
type WorkerRepository interface {
	UpsertWorker(ctx context.Context, worker *model.WorkerNode) error
	GetWorker(ctx context.Context, id string) (*model.WorkerNode, error)
	ListWorkers(ctx context.Context) ([]*model.WorkerNode, error)
	ListAvailableWorkers(ctx context.Context) ([]*model.WorkerNode, error)
	ListStaleWorkers(ctx context.Context, cutoff time.Time) ([]*model.WorkerNode, error)
}
