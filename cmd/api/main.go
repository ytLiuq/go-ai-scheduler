package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/example/go-ai-scheduler/internal/app"
	"github.com/example/go-ai-scheduler/internal/api/handler"
	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/config"
)

func main() {
	cfg := config.Default("api")
	l := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}).WithAttrs([]slog.Attr{slog.String("service", cfg.ServiceName)}))
	resources, cleanup := app.BuildResources(cfg, l)
	defer cleanup()

	workerService := apiservice.NewWorkerService(resources.Repositories.Worker)
	auditor := apiservice.NewAuditor(l)
	taskService := apiservice.NewTaskService(resources.Repositories.Task, auditor)
	taskInstanceService := apiservice.NewTaskInstanceService(resources.Repositories.TaskInstance)

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: handler.NewAPIRouter(
			handler.NewAuthHandler(),
			handler.NewWorkerHandler(workerService),
			handler.NewTaskHandler(taskService),
			handler.NewTaskInstanceHandler(taskInstanceService),
		),
	}

	l.Info("starting api http server", "addr", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		l.Error("api exited with error", "error", err)
		os.Exit(1)
	}
}
