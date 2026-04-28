package main

import (
	"net/http"

	"github.com/example/go-ai-scheduler/internal/app"
	"github.com/example/go-ai-scheduler/internal/api/handler"
	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/audit"
	"github.com/example/go-ai-scheduler/internal/config"
	"github.com/example/go-ai-scheduler/internal/pkg/logger"
)

func main() {
	cfg := config.Default("api")
	l := logger.New(cfg.ServiceName)
	resources, cleanup := app.BuildResources(cfg, l)
	defer cleanup()

	workerService := apiservice.NewWorkerService(resources.Repositories.Worker)
	auditor := audit.New(l)
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

	l.Printf("starting api http server on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		l.Fatalf("api exited with error: %v", err)
	}
}
