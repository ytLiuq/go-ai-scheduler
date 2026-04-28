package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/example/go-ai-scheduler/internal/alert"
	"github.com/example/go-ai-scheduler/internal/api/handler"
	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/app"
	"github.com/example/go-ai-scheduler/internal/config"
	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/pkg/cronexpr"
	"github.com/example/go-ai-scheduler/internal/pkg/logger"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	"github.com/example/go-ai-scheduler/internal/rpc"
	schedulercache "github.com/example/go-ai-scheduler/internal/scheduler/cache"
	"github.com/example/go-ai-scheduler/internal/scheduler/dispatch"
	"github.com/example/go-ai-scheduler/internal/scheduler/engine"
	schedulergrpc "github.com/example/go-ai-scheduler/internal/scheduler/grpcserver"
	"github.com/example/go-ai-scheduler/internal/scheduler/health"
	"github.com/example/go-ai-scheduler/internal/scheduler/leader"
	"github.com/example/go-ai-scheduler/internal/scheduler/ratelimit"
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

	bpCfg := ratelimit.DefaultBackpressureConfig()
	if cfg.MaxPending > 0 {
		bpCfg.MaxPending = cfg.MaxPending
	}
	bp := ratelimit.NewBackpressureController(bpCfg)

	cacheMgr := schedulercache.NewManager(resources.Redis, resources.Repositories.Task, resources.Repositories.Worker, l)

	workerService := apiservice.NewWorkerService(resources.Repositories.Worker)
	workerHandler := handler.NewWorkerHandler(workerService)
	router := route.NewRouter(resources.Repositories.Worker)
	dispatcher := dispatch.NewClientWithRateLimiter(3000) // 3000 dispatch/sec
	alerter := alert.New(cfg.AlertWebhookURL, l)
	taskRuntimeService := apiservice.NewTaskRuntimeService(resources.Repositories.Task, resources.Repositories.TaskInstance, resources.Repositories.Worker, router, dispatcher, alerter, cfg.SchedulerURL, l)
	taskRuntimeHandler := handler.NewTaskRuntimeHandler(taskRuntimeService)
	eventHandler := handler.NewEventHandler(resources.Repositories.Task, resources.Repositories.TaskInstance, router, dispatcher, l)
	leaderCtx := context.Background()
	elector := leader.New(resources.DB, cfg.EtcdAddrs, l)
	if err := elector.Acquire(leaderCtx); err != nil {
		l.Fatalf("acquire leadership: %v", err)
	}

	schedEngine := engine.New(resources.Repositories.Task, resources.Repositories.TaskInstance, l)
	schedEngine.OnTrigger = func(taskID int64) {
		if !bp.AllowDispatch() {
			return
		}
		task, err := resources.Repositories.Task.GetTask(leaderCtx, taskID)
		if err != nil {
			l.Printf("engine trigger: get task %d failed: %v", taskID, err)
			return
		}
		if task.Status != "enabled" {
			return
		}
		instance := &model.TaskInstance{
			TaskID:             task.ID,
			ScheduleInstanceID: newScheduleInstanceID(task.ID),
			TriggerTime:        time.Now(),
			Status:             "pending",
		}
		if err := resources.Repositories.TaskInstance.CreateInstance(leaderCtx, instance); err != nil {
			l.Printf("engine trigger: create instance for task %d failed: %v", taskID, err)
			return
		}
		worker, err := router.Pick(leaderCtx, route.SelectOptions{
			Labels:   model.DecodeLabels(task.Labels),
			Strategy: task.RouteStrategy,
		})
		if err != nil {
			if err == route.ErrNoAvailableWorker {
				task.NextTriggerTime = time.Now().Add(10 * time.Second)
				_ = resources.Repositories.Task.UpdateTask(leaderCtx, task)
				return
			}
			l.Printf("engine trigger: pick worker for task %d failed: %v", taskID, err)
			return
		}
		dispatchTime := time.Now()
		_ = resources.Repositories.TaskInstance.UpdateInstanceDispatch(leaderCtx, instance.ID, worker.ID, dispatchTime.Format(time.RFC3339Nano))
		if err := dispatcher.Dispatch(leaderCtx, worker, rpc.ExecuteTaskRequest{
			ScheduleInstanceID: instance.ScheduleInstanceID,
			TaskID:             task.ID,
			TaskType:           task.Type,
			Payload:            task.Payload,
			TimeoutSeconds:     task.TimeoutSeconds,
			RetryCount:         instance.RetryCount,
			IdempotencyKey:     task.IdempotencyKey,
			SchedulerURL:       cfg.SchedulerURL,
		}); err != nil {
			metrics.DefaultRegistry.IncCounter("scheduler_dispatch_total", map[string]string{"result": "error"})
			_ = resources.Repositories.TaskInstance.UpdateInstanceResult(leaderCtx, instance.ScheduleInstanceID, "failed", "dispatch_failed", err.Error())
			_ = router.Release(leaderCtx, worker)
			return
		}
		metrics.DefaultRegistry.IncCounter("scheduler_dispatch_total", map[string]string{"result": "success"})
		nextTrigger, err := cronexpr.NextAfter(time.Now(), task.CronExpr)
		if err == nil && !nextTrigger.IsZero() {
			task.NextTriggerTime = nextTrigger
			_ = resources.Repositories.Task.UpdateTask(leaderCtx, task)
			schedEngine.AddToWheel(task.ID, nextTrigger)
		}
		l.Printf("engine dispatched task_id=%d instance_id=%d worker_id=%s bp=%s",
			task.ID, instance.ID, worker.ID, bp.State().String())
	}

	if err := schedEngine.Warm(leaderCtx); err != nil {
		l.Printf("engine initial warm: %v", err)
	}
	go schedEngine.Start(leaderCtx)
	go schedEngine.WarmPeriodically(leaderCtx, 10*time.Second)

	triggerLoop := trigger.NewLoop(resources.Repositories.Task, resources.Repositories.TaskInstance, router, dispatcher, l, 5*time.Second, cfg.SchedulerURL, cfg.MaxPending, cacheMgr, bp)
	retryLoop := retry.NewLoop(resources.Repositories.Task, resources.Repositories.TaskInstance, router, dispatcher, l, 3*time.Second, cfg.SchedulerURL)
	go triggerLoop.Start(leaderCtx)
	go retryLoop.Start(leaderCtx)
	go cacheMgr.StartWarmLoop(leaderCtx, 10*time.Second)

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
		Handler: handler.NewSchedulerRouter(workerHandler, taskRuntimeHandler, eventHandler),
	}

	l.Printf("starting scheduler http server on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		l.Fatalf("scheduler exited with error: %v", err)
	}
}

func newScheduleInstanceID(taskID int64) string {
	return fmt.Sprintf("task-%d-%d", taskID, time.Now().UnixNano())
}
