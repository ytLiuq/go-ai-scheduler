package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/config"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	workerapp "github.com/example/go-ai-scheduler/internal/worker"
	workergrpc "github.com/example/go-ai-scheduler/internal/worker/grpcserver"
	
	
	"google.golang.org/grpc"
)

func main() {
	cfg := config.Default("worker")
	l := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}).WithAttrs([]slog.Attr{slog.String("service", cfg.ServiceName)}))
	hostname, err := os.Hostname()
	if err != nil {
		l.Error("read hostname", "error", err)
		os.Exit(1)
	}

	workerID := os.Getenv("WORKER_ID")
	if workerID == "" {
		workerID = fmt.Sprintf("%s%s", hostname, cfg.HTTPAddr)
	}
	client := workerapp.NewHeartbeatClient(cfg.SchedulerURL, cfg.SchedulerGRPCAddr, cfg.InternalProtocol)
	reportClient := workerapp.NewReportClient(cfg.InternalProtocol, cfg.SchedulerGRPCAddr)
	workerHandler := workerapp.NewHandler(workerID, reportClient, l, workerapp.HandlerConfig{
		SandboxDir:     os.TempDir(),
		MaxMemoryBytes: 256 * 1024 * 1024,
		LocalStoreDir:  os.TempDir(),
	})

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: metrics.Instrument("worker", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/healthz":
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			case "/metrics":
				metrics.DefaultRegistry.Handler().ServeHTTP(w, r)
			case "/internal/tasks/execute":
				workerHandler.Execute(w, r)
			case "/internal/tasks/cancel":
				workerHandler.CancelHTTP(w, r)
			default:
				http.NotFound(w, r)
			}
		})),
	}
	go func() {
		l.Info("worker http server started", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			l.Error("worker http server", "error", err)
			os.Exit(1)
		}
	}()

	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		l.Error("listen worker grpc", "error", err)
		os.Exit(1)
	}
	grpcServer := grpc.NewServer()
	workergrpc.Register(grpcServer, workergrpc.NewServer(workerHandler))
	go func() {
		l.Info("worker grpc server started", "addr", cfg.GRPCAddr)
		if err := grpcServer.Serve(grpcListener); err != nil {
			l.Error("worker grpc server", "error", err)
			os.Exit(1)
		}
	}()

	ctx := context.Background()

	if ls := workerHandler.LocalStore(); ls != nil {
		go ls.StartFlushLoop(ctx, 30*time.Second)
		l.Info("local store enabled, flush loop started")
	}
	workerHandler.StartDedupEviction(ctx, 5*time.Minute, 30*time.Second)
	l.Debug("worker dedup eviction started")

	registerReq := apiservice.WorkerRegistrationRequest{
		WorkerID:       workerID,
		Hostname:       hostname,
		IP:             "127.0.0.1",
		CallbackURL:    "http://127.0.0.1" + cfg.HTTPAddr,
		GRPCAddr:       "127.0.0.1" + cfg.GRPCAddr,
		Protocol:       cfg.InternalProtocol,
		MaxConcurrency: 100,
		Labels: map[string]string{
			"env":  "local",
			"zone": "dev",
		},
	}
	if err := client.Register(ctx, registerReq); err != nil {
		l.Error("register worker", "error", err)
		os.Exit(1)
	}
	l.Debug("worker registered to scheduler", "scheduler_url", cfg.SchedulerURL, "worker_id", workerID)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		runningTasks := workerHandler.RunningTasks()
		heartbeatReq := apiservice.WorkerHeartbeatRequest{
			WorkerID:        workerID,
			CurrentLoad:     runningTasks,
			RunningTasks:    runningTasks,
			ReportUnixMilli: time.Now().UnixMilli(),
		}
		if err := client.Heartbeat(ctx, heartbeatReq); err != nil {
			l.Warn("heartbeat failed", "error", err)
		} else {
			l.Debug("heartbeat sent", "worker_id", workerID)
		}
		<-ticker.C
	}
}
