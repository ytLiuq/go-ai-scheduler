package app

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"time"

	"github.com/example/go-ai-scheduler/internal/config"
	"github.com/example/go-ai-scheduler/internal/pkg/xmysql"
	"github.com/example/go-ai-scheduler/internal/pkg/xredis"
	"github.com/example/go-ai-scheduler/internal/repo"
)

// Resources groups shared startup resources for one process.
type Resources struct {
	Repositories *repo.Bundle
	DB           *sql.DB
	Redis        *xredis.Client
}

// BuildResources initializes repositories, database, and Redis.
func BuildResources(cfg config.Config, l *slog.Logger) (*Resources, func()) {
	res := &Resources{}
	cleaners := make([]func(), 0)
	cleanup := func() {
		for i := len(cleaners) - 1; i >= 0; i-- {
			cleaners[i]()
		}
	}

	if !repo.IsMySQLBackend(cfg.RepoBackend) {
		l.Error("repository backend not supported", "backend", cfg.RepoBackend)
		os.Exit(1)
	}

	gdb, err := xmysql.OpenGorm(cfg.MySQLDSN)
	if err != nil {
		l.Error("init mysql repositories", "error", err)
		os.Exit(1)
	}
	db, err := gdb.DB()
	if err != nil {
		l.Error("get mysql sql db", "error", err)
		os.Exit(1)
	}
	if cfg.AutoMigrate {
		if err := xmysql.RunMigrations(db, cfg.MigrationDir); err != nil {
			l.Error("run migrations", "error", err)
			os.Exit(1)
		}
		l.Debug("migrations applied", "dir", cfg.MigrationDir)
	}
	bundle := repo.NewMySQLBundle(gdb)
	if err := repo.ValidateBundle(bundle); err != nil {
		l.Error("invalid mysql repository bundle", "error", err)
		os.Exit(1)
	}
	res.Repositories = bundle
	res.DB = db
	cleaners = append(cleaners, func() { _ = db.Close() })
	l.Debug("repository backend", "type", "mysql")

	if cfg.RedisAddr != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		redisClient, err := xredis.Open(ctx, cfg.RedisAddr)
		if err != nil {
			l.Warn("redis unavailable, running without cache", "addr", cfg.RedisAddr, "error", err)
		} else {
			res.Redis = redisClient
			cleaners = append(cleaners, func() { _ = redisClient.Close() })
			l.Debug("redis connected", "addr", cfg.RedisAddr)
		}
	} else {
		l.Info("redis not configured, running without cache")
	}

	return res, cleanup
}
