package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/config"
	"github.com/example/go-ai-scheduler/internal/pkg/logger"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	workerapp "github.com/example/go-ai-scheduler/internal/worker"
	workergrpc "github.com/example/go-ai-scheduler/internal/worker/grpcserver"
	"github.com/example/go-ai-scheduler/internal/worker/heartbeat"
	"github.com/example/go-ai-scheduler/internal/worker/reporter"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.Default("worker")
	l := logger.New(cfg.ServiceName)
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("read hostname: %v", err)
	}

	workerID := fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano())
	client := heartbeat.NewClient(cfg.SchedulerURL, cfg.SchedulerGRPCAddr, cfg.InternalProtocol)
	reportClient := reporter.NewClient(cfg.InternalProtocol, cfg.SchedulerGRPCAddr)
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
			default:
				http.NotFound(w, r)
			}
		})),
	}
	go func() {
		l.Printf("worker http server started on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("worker http server: %v", err)
		}
	}()

	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatalf("listen worker grpc: %v", err)
	}
	grpcServer := grpc.NewServer()
	workergrpc.Register(grpcServer, workergrpc.NewServer(workerHandler))
	go func() {
		l.Printf("worker grpc server started on %s", cfg.GRPCAddr)
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatalf("worker grpc server: %v", err)
		}
	}()

	ctx := context.Background()

	if ls := workerHandler.LocalStore(); ls != nil {
		go ls.StartFlushLoop(ctx, 30*time.Second)
		l.Printf("local store enabled, flush loop started")
	}

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
		log.Fatalf("register worker: %v", err)
	}
	l.Printf("worker registered to scheduler=%s worker_id=%s", cfg.SchedulerURL, workerID)

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
			l.Printf("heartbeat failed: %v", err)
		} else {
			l.Printf("heartbeat sent worker_id=%s", workerID)
		}
		<-ticker.C
	}
}
