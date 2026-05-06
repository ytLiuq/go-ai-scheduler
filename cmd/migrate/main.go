package main

import (
	"log/slog"
	"os"

	"github.com/example/go-ai-scheduler/internal/config"
	"github.com/example/go-ai-scheduler/internal/pkg/xmysql"
	"github.com/example/go-ai-scheduler/internal/repo"
)

func main() {
	cfg := config.Default("migrate")
	l := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}).WithAttrs([]slog.Attr{slog.String("service", "migrate")}))
	if !repo.IsMySQLBackend(cfg.RepoBackend) {
		l.Error("migrate requires REPO_BACKEND=mysql")
		os.Exit(1)
	}

	db, err := xmysql.Open(cfg.MySQLDSN)
	if err != nil {
		l.Error("open mysql", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := xmysql.RunMigrations(db, cfg.MigrationDir); err != nil {
		l.Error("run migrations", "error", err)
		os.Exit(1)
	}
	l.Info("migrations applied", "dir", cfg.MigrationDir)
}
