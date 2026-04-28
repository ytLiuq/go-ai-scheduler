package app

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/example/go-ai-scheduler/internal/config"
	"github.com/example/go-ai-scheduler/internal/pkg/xmysql"
	"github.com/example/go-ai-scheduler/internal/pkg/xredis"
	"github.com/example/go-ai-scheduler/internal/repo"
	"github.com/example/go-ai-scheduler/internal/repo/memory"
)

// Resources groups shared startup resources for one process.
type Resources struct {
	Repositories *repo.Bundle
	DB           *sql.DB
	Redis        *xredis.Client
}

// BuildResources initializes repositories, database, and Redis.
func BuildResources(cfg config.Config, l *log.Logger) (*Resources, func()) {
	res := &Resources{}
	cleaners := make([]func(), 0)
	cleanup := func() {
		for i := len(cleaners) - 1; i >= 0; i-- {
			cleaners[i]()
		}
	}

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
		res.Repositories = bundle
		res.DB = db
		cleaners = append(cleaners, func() { _ = db.Close() })
		l.Printf("repository backend=mysql")
	} else {
		bundle := &repo.Bundle{
			Task:         memory.NewTaskRepository(),
			TaskInstance: memory.NewTaskInstanceRepository(),
			Worker:       memory.NewWorkerRepository(),
		}
		if err := repo.ValidateBundle(bundle); err != nil {
			log.Fatalf("invalid memory repository bundle: %v", err)
		}
		res.Repositories = bundle
		l.Printf("repository backend=memory")
	}

	// Redis is optional — only connect if an address is configured.
	if cfg.RedisAddr != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		redisClient, err := xredis.Open(ctx, cfg.RedisAddr)
		if err != nil {
			l.Printf("WARNING: redis unavailable at %s: %v — running without cache", cfg.RedisAddr, err)
		} else {
			res.Redis = redisClient
			cleaners = append(cleaners, func() { _ = redisClient.Close() })
			l.Printf("redis connected at %s", cfg.RedisAddr)
		}
	} else {
		l.Printf("redis not configured — running without cache")
	}

	return res, cleanup
}
