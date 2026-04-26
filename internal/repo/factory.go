package repo

import (
	"database/sql"
	"fmt"
	"strings"

	mysqlrepo "github.com/example/go-ai-scheduler/internal/repo/mysql"
)

// Bundle groups repository implementations used by the scheduler runtime.
type Bundle struct {
	Task         TaskRepository
	TaskInstance TaskInstanceRepository
	Worker       WorkerRepository
}

// NewMySQLBundle builds repositories backed by one MySQL connection pool.
func NewMySQLBundle(db *sql.DB) *Bundle {
	return &Bundle{
		Task:         mysqlrepo.NewTaskRepository(db),
		TaskInstance: mysqlrepo.NewTaskInstanceRepository(db),
		Worker:       mysqlrepo.NewWorkerRepository(db),
	}
}

// IsMySQLBackend reports whether mysql repositories should be used.
func IsMySQLBackend(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "mysql")
}

// ValidateBundle ensures all repositories are wired.
func ValidateBundle(bundle *Bundle) error {
	if bundle == nil || bundle.Task == nil || bundle.TaskInstance == nil || bundle.Worker == nil {
		return fmt.Errorf("repository bundle is incomplete")
	}
	return nil
}
