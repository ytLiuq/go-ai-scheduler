package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/example/go-ai-scheduler/internal/alert"
	"github.com/example/go-ai-scheduler/internal/api/handler"
	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/app"
	"github.com/example/go-ai-scheduler/internal/config"
	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/pkg/cronexpr"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	schedulercache "github.com/example/go-ai-scheduler/internal/scheduler/cache"
	"github.com/example/go-ai-scheduler/internal/scheduler/dispatch"
	"github.com/example/go-ai-scheduler/internal/scheduler/engine"
	schedulergrpc "github.com/example/go-ai-scheduler/internal/scheduler/grpcserver"
	"github.com/example/go-ai-scheduler/internal/scheduler/health"
	trendPkg "github.com/example/go-ai-scheduler/internal/ai/trend"
	"github.com/example/go-ai-scheduler/internal/scheduler/leader"
	"github.com/example/go-ai-scheduler/internal/scheduler/ratelimit"
	"github.com/example/go-ai-scheduler/internal/scheduler/retry"
	"github.com/example/go-ai-scheduler/internal/scheduler/route"
	"github.com/example/go-ai-scheduler/internal/scheduler/trigger"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.Default("scheduler")
	l := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}).WithAttrs([]slog.Attr{slog.String("service", cfg.ServiceName)}))
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
	dispatcher := dispatch.NewClientWithRateLimiter(3000)
	alerter := alert.New(cfg.AlertWebhookURL, l)
	aiClient := apiservice.NewAIClient(os.Getenv("AI_SERVICE_URL"))
	taskRuntimeService := apiservice.NewTaskRuntimeService(resources.Repositories.Task, resources.Repositories.TaskInstance, resources.Repositories.Worker, router, dispatcher, alerter, cfg.SchedulerURL, l, aiClient)
	taskRuntimeHandler := handler.NewTaskRuntimeHandler(taskRuntimeService)
	eventHandler := handler.NewEventHandler(resources.Repositories.Task, resources.Repositories.TaskInstance, router, dispatcher, l)
	leaderCtx := context.Background()
	elector := leader.New(resources.DB, cfg.EtcdAddrs, l)
	if err := elector.Acquire(leaderCtx); err != nil {
		l.Error("acquire leadership", "error", err)
		os.Exit(1)
	}

	schedEngine := engine.New(resources.Repositories.Task, resources.Repositories.TaskInstance, l)
	schedEngine.OnTrigger = func(taskID int64) {
		if !bp.AllowDispatch() {
			return
		}
		task, err := resources.Repositories.Task.GetTask(leaderCtx, taskID)
		if err != nil {
			l.Warn("engine trigger: get task failed", "task_id", taskID, "error", err)
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
			l.Warn("engine trigger: create instance failed", "task_id", taskID, "error", err)
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
			l.Warn("engine trigger: pick worker failed", "task_id", taskID, "error", err)
			return
		}
		dispatchTime := time.Now()
		_ = resources.Repositories.TaskInstance.UpdateInstanceDispatch(leaderCtx, instance.ID, worker.ID, dispatchTime.Format(time.RFC3339Nano))
		if err := dispatcher.Dispatch(leaderCtx, worker, model.ExecuteTaskRequest{
			ScheduleInstanceID: instance.ScheduleInstanceID,
			TaskID:             task.ID,
			TaskType:           task.Type,
			Payload:            task.Payload,
			Image:              task.Image,
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
		l.Debug("engine dispatched", "task_id", task.ID, "instance_id", instance.ID, "worker_id", worker.ID, "bp", bp.State().String())
	}

	if err := schedEngine.Warm(leaderCtx); err != nil {
		l.Warn("engine initial warm", "error", err)
	}
	go schedEngine.Start(leaderCtx)
	go schedEngine.WarmPeriodically(leaderCtx, 10*time.Second)

	triggerLoop := trigger.NewLoop(resources.Repositories.Task, resources.Repositories.TaskInstance, router, dispatcher, l, 5*time.Second, cfg.SchedulerURL, cfg.MaxPending, cacheMgr, bp)
	retryLoop := retry.NewLoop(resources.Repositories.Task, resources.Repositories.TaskInstance, router, dispatcher, l, 3*time.Second, cfg.SchedulerURL)
	go triggerLoop.Start(leaderCtx)
	go retryLoop.Start(leaderCtx)
	go cacheMgr.StartWarmLoop(leaderCtx, 10*time.Second)

	go func() {
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 8, 0, 0, 0, now.Location())
			time.Sleep(next.Sub(now))
			workers, _ := resources.Repositories.Worker.ListWorkers(context.Background())
			tasks, _ := resources.Repositories.Task.ListTasks(context.Background())
			instances, _ := resources.Repositories.TaskInstance.ListInstancesByTimeRange(context.Background(), time.Now().Add(-24*time.Hour), time.Now(), 0, 0)
			snap := trendPkg.ComputeSnapshot(workers, tasks, instances, 24)
			l.Debug("daily health summary",
				"online_workers", snap.OnlineWorkers, "worker_count", snap.WorkerCount,
				"avg_load_pct", snap.AvgWorkerLoad*100,
				"enabled_tasks", snap.EnabledTasks, "task_count", snap.TaskCount,
				"total_instances", snap.TotalInstances, "failed", snap.FailedInstances, "success", snap.SuccessInstances)
		}
	}()

	healthLoop := health.NewChecker(workerService, l, 30*time.Second, 10*time.Second)
	go healthLoop.Start(leaderCtx)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-leaderCtx.Done():
				return
			case <-ticker.C:
				workers, err := resources.Repositories.Worker.ListWorkers(leaderCtx)
				if err != nil {
					continue
				}
				for _, w := range workers {
					_ = resources.Repositories.WorkerLoad.CreateSnapshot(leaderCtx, &model.WorkerLoadSnapshot{
						WorkerID: w.ID, CurrentLoad: w.CurrentLoad, MaxConcurrency: w.MaxConcurrency, Status: w.Status,
					})
				}
				cutoff := time.Now().Add(-7 * 24 * time.Hour)
				if n, err := resources.Repositories.WorkerLoad.DeleteSnapshotsBefore(leaderCtx, cutoff); err == nil && n > 0 {
					l.Debug("worker load cleanup: deleted old snapshots", "count", n)
				}
			}
		}
	}()

	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		l.Error("listen grpc", "error", err)
		os.Exit(1)
	}
	grpcServer := grpc.NewServer()
	schedulergrpc.Register(grpcServer, schedulergrpc.NewServer(workerService, taskRuntimeService))
	go func() {
		l.Info("starting scheduler grpc server", "addr", cfg.GRPCAddr)
		if serveErr := grpcServer.Serve(grpcListener); serveErr != nil {
			l.Error("scheduler grpc exited with error", "error", serveErr)
			os.Exit(1)
		}
	}()

	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: handler.NewSchedulerRouter(workerHandler, taskRuntimeHandler, eventHandler, handler.NewWorkerLoadHandler(resources.Repositories.WorkerLoad)),
	}

	l.Info("starting scheduler http server", "addr", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		l.Error("scheduler exited with error", "error", err)
		os.Exit(1)
	}
}

func newScheduleInstanceID(taskID int64) string {
	return fmt.Sprintf("task-%d-%d", taskID, time.Now().UnixNano())
}
