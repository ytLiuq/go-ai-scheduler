package main

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/example/go-ai-scheduler/internal/alert"
	"github.com/example/go-ai-scheduler/internal/api/handler"
	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/app"
	"github.com/example/go-ai-scheduler/internal/config"
	"github.com/example/go-ai-scheduler/internal/pkg/logger"
	"github.com/example/go-ai-scheduler/internal/scheduler/dispatch"
	schedulergrpc "github.com/example/go-ai-scheduler/internal/scheduler/grpcserver"
	"github.com/example/go-ai-scheduler/internal/scheduler/health"
	"github.com/example/go-ai-scheduler/internal/scheduler/leader"
	"github.com/example/go-ai-scheduler/internal/scheduler/retry"
	"github.com/example/go-ai-scheduler/internal/scheduler/route"
	"github.com/example/go-ai-scheduler/internal/scheduler/trigger"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.Default("scheduler")
	l := logger.New(cfg.ServiceName)
	resources, cleanup := app.BuildResources(cfg, l)
	defer cleanup()

	workerService := apiservice.NewWorkerService(resources.Repositories.Worker)
	workerHandler := handler.NewWorkerHandler(workerService)
	router := route.NewRouter(resources.Repositories.Worker)
	dispatcher := dispatch.NewClient()
	alerter := alert.New(cfg.AlertWebhookURL, l)
	taskRuntimeService := apiservice.NewTaskRuntimeService(resources.Repositories.Task, resources.Repositories.TaskInstance, resources.Repositories.Worker, router, dispatcher, alerter, cfg.SchedulerURL, l)
	taskRuntimeHandler := handler.NewTaskRuntimeHandler(taskRuntimeService)
	leaderCtx := context.Background()
	elector := leader.New(resources.DB, cfg.EtcdAddrs, l)
	if err := elector.Acquire(leaderCtx); err != nil {
		l.Fatalf("acquire leadership: %v", err)
	}
	triggerLoop := trigger.NewLoop(resources.Repositories.Task, resources.Repositories.TaskInstance, router, dispatcher, l, time.Second, cfg.SchedulerURL, cfg.MaxPending)
	retryLoop := retry.NewLoop(resources.Repositories.Task, resources.Repositories.TaskInstance, router, dispatcher, l, 3*time.Second, cfg.SchedulerURL)
	go triggerLoop.Start(leaderCtx)
	go retryLoop.Start(leaderCtx)
	healthLoop := health.NewChecker(workerService, l, 30*time.Second, 10*time.Second)
	go healthLoop.Start(leaderCtx)

	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		l.Fatalf("listen grpc: %v", err)
	}
	grpcServer := grpc.NewServer()
	schedulergrpc.Register(grpcServer, schedulergrpc.NewServer(workerService, taskRuntimeService))
	go func() {
		l.Printf("starting scheduler grpc server on %s", cfg.GRPCAddr)
		if serveErr := grpcServer.Serve(grpcListener); serveErr != nil {
			l.Fatalf("scheduler grpc exited with error: %v", serveErr)
		}
	}()

	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: handler.NewSchedulerRouter(workerHandler, taskRuntimeHandler),
	}

	l.Printf("starting scheduler http server on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		l.Fatalf("scheduler exited with error: %v", err)
	}
}
