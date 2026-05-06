package repo

import (
	"fmt"
	"strings"

	mysqlrepo "github.com/example/go-ai-scheduler/internal/repo/mysql"
	"gorm.io/gorm"
)

// Bundle groups repository implementations used by the scheduler runtime.
type Bundle struct {
	Task         TaskRepository
	TaskInstance TaskInstanceRepository
	Worker       WorkerRepository
	WorkerLoad   WorkerLoadRepository
	AIAnalysis   AIAnalysisRepository
}

// NewMySQLBundle builds repositories backed by one MySQL connection pool.
func NewMySQLBundle(db *gorm.DB) *Bundle {
	return &Bundle{
		Task:         mysqlrepo.NewTaskRepository(db),
		TaskInstance: mysqlrepo.NewTaskInstanceRepository(db),
		Worker:       mysqlrepo.NewWorkerRepository(db),
		WorkerLoad:   mysqlrepo.NewWorkerLoadRepository(db),
		AIAnalysis:   mysqlrepo.NewAIAnalysisRepository(db),
	}
}

// IsMySQLBackend reports whether mysql repositories should be used.
func IsMySQLBackend(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "mysql")
}

// ValidateBundle ensures all repositories are wired.
func ValidateBundle(bundle *Bundle) error {
	if bundle == nil || bundle.Task == nil || bundle.TaskInstance == nil || bundle.Worker == nil || bundle.WorkerLoad == nil || bundle.AIAnalysis == nil {
		return fmt.Errorf("repository bundle is incomplete")
	}
	return nil
}
