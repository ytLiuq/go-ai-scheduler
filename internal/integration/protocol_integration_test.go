package integration

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/example/go-ai-scheduler/internal/api/handler"
	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/pkg/logger"
	_ "github.com/example/go-ai-scheduler/internal/pkg/xgrpc"
	"github.com/example/go-ai-scheduler/internal/repo/memory"
	schedulerdispatch "github.com/example/go-ai-scheduler/internal/scheduler/dispatch"
	schedulergrpc "github.com/example/go-ai-scheduler/internal/scheduler/grpcserver"
	"github.com/example/go-ai-scheduler/internal/scheduler/route"
	"github.com/example/go-ai-scheduler/internal/scheduler/trigger"
	workerapp "github.com/example/go-ai-scheduler/internal/worker"
	workergrpc "github.com/example/go-ai-scheduler/internal/worker/grpcserver"
	"github.com/example/go-ai-scheduler/internal/worker/heartbeat"
	"github.com/example/go-ai-scheduler/internal/worker/reporter"
	"google.golang.org/grpc"
)

func TestHTTPInternalProtocolEndToEnd(t *testing.T) {
	t.Parallel()

	taskRepo := memory.NewTaskRepository()
	instanceRepo := memory.NewTaskInstanceRepository()
	workerRepo := memory.NewWorkerRepository()
	logr := logger.New("test-http")

	router := route.NewRouter(workerRepo)
	dispatcher := schedulerdispatch.NewClient()
	workerService := apiservice.NewWorkerService(workerRepo)
	runtimeService := apiservice.NewTaskRuntimeService(taskRepo, instanceRepo, workerRepo, router, dispatcher, "", logr)

	schedulerServer := httptest.NewServer(handler.NewSchedulerRouter(
		handler.NewWorkerHandler(workerService),
		handler.NewTaskRuntimeHandler(runtimeService),
	))
	defer schedulerServer.Close()

	successTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer successTarget.Close()

	reportClient := reporter.NewClient("http", "")
	workerHandler := workerapp.NewHandler("worker-http-1", reportClient, logr)
	workerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/tasks/execute":
			workerHandler.Execute(w, r)
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer workerServer.Close()

	heartbeatClient := heartbeat.NewClient(schedulerServer.URL, "", "http")
	err := heartbeatClient.Register(context.Background(), apiservice.WorkerRegistrationRequest{
		WorkerID:       "worker-http-1",
		Hostname:       "worker-http-host",
		IP:             "127.0.0.1",
		CallbackURL:    workerServer.URL,
		Protocol:       "http",
		MaxConcurrency: 10,
	})
	if err != nil {
		t.Fatalf("register worker over http: %v", err)
	}

	task := &model.Task{
		Name:            "http-task",
		Type:            "http",
		Payload:         successTarget.URL,
		Status:          "enabled",
		TimeoutSeconds:  5,
		MaxRetry:        1,
		RetryPolicy:     "fixed_interval",
		RouteStrategy:   "round_robin",
		NextTriggerTime: time.Now().Add(-time.Second),
	}
	if err := taskRepo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go trigger.NewLoop(taskRepo, instanceRepo, router, dispatcher, logr, 50*time.Millisecond, schedulerServer.URL).Start(loopCtx)

	waitForStatus(t, instanceRepo, "success")
}

func TestGRPCInternalProtocolEndToEnd(t *testing.T) {
	t.Parallel()

	taskRepo := memory.NewTaskRepository()
	instanceRepo := memory.NewTaskInstanceRepository()
	workerRepo := memory.NewWorkerRepository()
	logr := logger.New("test-grpc")

	router := route.NewRouter(workerRepo)
	dispatcher := schedulerdispatch.NewClient()
	workerService := apiservice.NewWorkerService(workerRepo)
	runtimeService := apiservice.NewTaskRuntimeService(taskRepo, instanceRepo, workerRepo, router, dispatcher, "", logr)

	schedulerLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen scheduler grpc: %v", err)
	}
	defer schedulerLis.Close()

	schedulerServer := grpc.NewServer()
	schedulergrpc.Register(schedulerServer, schedulergrpc.NewServer(workerService, runtimeService))
	go func() {
		_ = schedulerServer.Serve(schedulerLis)
	}()
	defer schedulerServer.Stop()

	successTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer successTarget.Close()

	reportClient := reporter.NewClient("grpc", schedulerLis.Addr().String())
	workerHandler := workerapp.NewHandler("worker-grpc-1", reportClient, logr)

	workerLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen worker grpc: %v", err)
	}
	defer workerLis.Close()

	workerServer := grpc.NewServer()
	workergrpc.Register(workerServer, workergrpc.NewServer(workerHandler))
	go func() {
		_ = workerServer.Serve(workerLis)
	}()
	defer workerServer.Stop()

	heartbeatClient := heartbeat.NewClient("", schedulerLis.Addr().String(), "grpc")
	err = heartbeatClient.Register(context.Background(), apiservice.WorkerRegistrationRequest{
		WorkerID:       "worker-grpc-1",
		Hostname:       "worker-grpc-host",
		IP:             "127.0.0.1",
		GRPCAddr:       workerLis.Addr().String(),
		Protocol:       "grpc",
		MaxConcurrency: 10,
	})
	if err != nil {
		t.Fatalf("register worker over grpc: %v", err)
	}

	task := &model.Task{
		Name:            "grpc-task",
		Type:            "http",
		Payload:         successTarget.URL,
		Status:          "enabled",
		TimeoutSeconds:  5,
		MaxRetry:        1,
		RetryPolicy:     "fixed_interval",
		RouteStrategy:   "round_robin",
		NextTriggerTime: time.Now().Add(-time.Second),
	}
	if err := taskRepo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go trigger.NewLoop(taskRepo, instanceRepo, router, dispatcher, logr, 50*time.Millisecond, "").Start(loopCtx)

	waitForStatus(t, instanceRepo, "success")
}

func TestHTTPInternalProtocolRetryThenSuccess(t *testing.T) {
	t.Parallel()

	taskRepo := memory.NewTaskRepository()
	instanceRepo := memory.NewTaskInstanceRepository()
	workerRepo := memory.NewWorkerRepository()
	logr := logger.New("test-http-retry")

	router := route.NewRouter(workerRepo)
	dispatcher := schedulerdispatch.NewClient()
	workerService := apiservice.NewWorkerService(workerRepo)
	schedulerServer := httptest.NewServer(handler.NewSchedulerRouter(
		handler.NewWorkerHandler(workerService),
		nil,
	))
	defer schedulerServer.Close()
	runtimeService := apiservice.NewTaskRuntimeService(taskRepo, instanceRepo, workerRepo, router, dispatcher, schedulerServer.URL, logr)
	schedulerServer.Config.Handler = handler.NewSchedulerRouter(
		handler.NewWorkerHandler(workerService),
		handler.NewTaskRuntimeHandler(runtimeService),
	)

	var requestCount atomic.Int32
	flakyTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requestCount.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer flakyTarget.Close()

	reportClient := reporter.NewClient("http", "")
	workerHandler := workerapp.NewHandler("worker-http-retry-1", reportClient, logr)
	workerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/tasks/execute":
			workerHandler.Execute(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer workerServer.Close()

	heartbeatClient := heartbeat.NewClient(schedulerServer.URL, "", "http")
	err := heartbeatClient.Register(context.Background(), apiservice.WorkerRegistrationRequest{
		WorkerID:       "worker-http-retry-1",
		Hostname:       "worker-http-retry-host",
		IP:             "127.0.0.1",
		CallbackURL:    workerServer.URL,
		Protocol:       "http",
		MaxConcurrency: 10,
	})
	if err != nil {
		t.Fatalf("register worker over http: %v", err)
	}

	task := &model.Task{
		Name:            "http-retry-task",
		Type:            "http",
		Payload:         flakyTarget.URL,
		Status:          "enabled",
		TimeoutSeconds:  5,
		MaxRetry:        1,
		RetryPolicy:     "fixed_interval",
		RouteStrategy:   "round_robin",
		NextTriggerTime: time.Now().Add(-time.Second),
	}
	if err := taskRepo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go trigger.NewLoop(taskRepo, instanceRepo, router, dispatcher, logr, 50*time.Millisecond, schedulerServer.URL).Start(loopCtx)

	waitForRetrySuccess(t, instanceRepo)
}

func TestGRPCInternalProtocolRetryThenSuccess(t *testing.T) {
	t.Parallel()

	taskRepo := memory.NewTaskRepository()
	instanceRepo := memory.NewTaskInstanceRepository()
	workerRepo := memory.NewWorkerRepository()
	logr := logger.New("test-grpc-retry")

	router := route.NewRouter(workerRepo)
	dispatcher := schedulerdispatch.NewClient()
	workerService := apiservice.NewWorkerService(workerRepo)

	schedulerLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen scheduler grpc: %v", err)
	}
	defer schedulerLis.Close()

	runtimeService := apiservice.NewTaskRuntimeService(taskRepo, instanceRepo, workerRepo, router, dispatcher, "", logr)
	schedulerServer := grpc.NewServer()
	schedulergrpc.Register(schedulerServer, schedulergrpc.NewServer(workerService, runtimeService))
	go func() {
		_ = schedulerServer.Serve(schedulerLis)
	}()
	defer schedulerServer.Stop()

	var requestCount atomic.Int32
	flakyTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requestCount.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer flakyTarget.Close()

	reportClient := reporter.NewClient("grpc", schedulerLis.Addr().String())
	workerHandler := workerapp.NewHandler("worker-grpc-retry-1", reportClient, logr)

	workerLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen worker grpc: %v", err)
	}
	defer workerLis.Close()

	workerServer := grpc.NewServer()
	workergrpc.Register(workerServer, workergrpc.NewServer(workerHandler))
	go func() {
		_ = workerServer.Serve(workerLis)
	}()
	defer workerServer.Stop()

	heartbeatClient := heartbeat.NewClient("", schedulerLis.Addr().String(), "grpc")
	err = heartbeatClient.Register(context.Background(), apiservice.WorkerRegistrationRequest{
		WorkerID:       "worker-grpc-retry-1",
		Hostname:       "worker-grpc-retry-host",
		IP:             "127.0.0.1",
		GRPCAddr:       workerLis.Addr().String(),
		Protocol:       "grpc",
		MaxConcurrency: 10,
	})
	if err != nil {
		t.Fatalf("register worker over grpc: %v", err)
	}

	task := &model.Task{
		Name:            "grpc-retry-task",
		Type:            "http",
		Payload:         flakyTarget.URL,
		Status:          "enabled",
		TimeoutSeconds:  5,
		MaxRetry:        1,
		RetryPolicy:     "fixed_interval",
		RouteStrategy:   "round_robin",
		NextTriggerTime: time.Now().Add(-time.Second),
	}
	if err := taskRepo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go trigger.NewLoop(taskRepo, instanceRepo, router, dispatcher, logr, 50*time.Millisecond, "").Start(loopCtx)

	waitForRetrySuccess(t, instanceRepo)
}

func waitForStatus(t *testing.T, instanceRepo *memory.TaskInstanceRepository, expected string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		instances, err := instanceRepo.ListInstances(context.Background())
		if err != nil {
			t.Fatalf("list instances: %v", err)
		}
		for _, instance := range instances {
			if instance.Status == expected {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	instances, _ := instanceRepo.ListInstances(context.Background())
	t.Fatalf("did not observe instance status=%s, instances=%+v", expected, instances)
}

func waitForRetrySuccess(t *testing.T, instanceRepo *memory.TaskInstanceRepository) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		instances, err := instanceRepo.ListInstances(context.Background())
		if err != nil {
			t.Fatalf("list instances: %v", err)
		}

		var failedFound bool
		var successFound bool
		var retrySuccessFound bool
		for _, instance := range instances {
			if instance.Status == "failed" && instance.RetryCount == 0 {
				failedFound = true
			}
			if instance.Status == "success" {
				successFound = true
				if instance.RetryCount == 1 {
					retrySuccessFound = true
				}
			}
		}

		if failedFound && successFound && retrySuccessFound && len(instances) >= 2 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	instances, _ := instanceRepo.ListInstances(context.Background())
	t.Fatalf("did not observe failed->retry->success flow, instances=%+v", instances)
}
