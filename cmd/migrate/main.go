package main

import (
	"log"

	"github.com/example/go-ai-scheduler/internal/config"
	"github.com/example/go-ai-scheduler/internal/pkg/logger"
	"github.com/example/go-ai-scheduler/internal/pkg/xmysql"
	"github.com/example/go-ai-scheduler/internal/repo"
)

func main() {
	cfg := config.Default("migrate")
	l := logger.New("migrate")
	if !repo.IsMySQLBackend(cfg.RepoBackend) {
		log.Fatalf("migrate requires REPO_BACKEND=mysql")
	}

	db, err := xmysql.Open(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	defer db.Close()

	if err := xmysql.RunMigrations(db, cfg.MigrationDir); err != nil {
		log.Fatalf("run migrations: %v", err)
	}
	l.Printf("migrations applied from %s", cfg.MigrationDir)
}
