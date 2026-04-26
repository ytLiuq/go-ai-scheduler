package app

import (
	"database/sql"
	"log"

	"github.com/example/go-ai-scheduler/internal/config"
	"github.com/example/go-ai-scheduler/internal/pkg/xmysql"
	"github.com/example/go-ai-scheduler/internal/repo"
	"github.com/example/go-ai-scheduler/internal/repo/memory"
)

// Resources groups shared startup resources for one process.
type Resources struct {
	Repositories *repo.Bundle
	DB           *sql.DB
}

// BuildResources initializes repositories and optional migrations.
func BuildResources(cfg config.Config, l *log.Logger) (*Resources, func()) {
	if repo.IsMySQLBackend(cfg.RepoBackend) {
		db, err := xmysql.Open(cfg.MySQLDSN)
		if err != nil {
			log.Fatalf("init mysql repositories: %v", err)
		}
		if cfg.AutoMigrate {
			if err := xmysql.RunMigrations(db, cfg.MigrationDir); err != nil {
				log.Fatalf("run migrations: %v", err)
			}
			l.Printf("migrations applied from %s", cfg.MigrationDir)
		}
		bundle := repo.NewMySQLBundle(db)
		if err := repo.ValidateBundle(bundle); err != nil {
			log.Fatalf("invalid mysql repository bundle: %v", err)
		}
		l.Printf("repository backend=mysql")
		return &Resources{
			Repositories: bundle,
			DB:           db,
		}, func() {
			_ = db.Close()
		}
	}

	bundle := &repo.Bundle{
		Task:         memory.NewTaskRepository(),
		TaskInstance: memory.NewTaskInstanceRepository(),
		Worker:       memory.NewWorkerRepository(),
	}
	if err := repo.ValidateBundle(bundle); err != nil {
		log.Fatalf("invalid memory repository bundle: %v", err)
	}
	l.Printf("repository backend=memory")
	return &Resources{Repositories: bundle}, func() {}
}

