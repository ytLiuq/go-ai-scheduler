package grpcserver

import (
	"context"
	"log/slog"

	"github.com/example/go-ai-scheduler/internal/api/service"
	schedulerv1 "github.com/example/go-ai-scheduler/proto/gen/scheduler/v1"
	"google.golang.org/grpc"
)

// Server exposes scheduler control RPCs to workers.
type Server struct {
	schedulerv1.UnimplementedWorkerControlServiceServer
	workers *service.WorkerService
	runtime *service.TaskRuntimeService
}

// NewServer creates a scheduler gRPC server adapter.
func NewServer(workers *service.WorkerService, runtime *service.TaskRuntimeService) *Server {
	return &Server{workers: workers, runtime: runtime}
}

// Register binds the generated service stub to a gRPC server.
func Register(s *grpc.Server, impl *Server) {
	schedulerv1.RegisterWorkerControlServiceServer(s, impl)
}

func (s *Server) RegisterWorker(ctx context.Context, req *schedulerv1.RegisterWorkerRequest) (*schedulerv1.RegisterWorkerResponse, error) {
	svcReq := service.WorkerRegistrationRequest{
		WorkerID:       req.GetWorkerId(),
		Hostname:       req.GetHostname(),
		IP:             req.GetIp(),
		CallbackURL:    req.GetCallbackUrl(),
		GRPCAddr:       req.GetGrpcAddr(),
		Protocol:       req.GetProtocol(),
		MaxConcurrency: int(req.GetMaxConcurrency()),
		Labels:         req.GetLabels(),
	}
	if _, err := s.workers.RegisterWorker(ctx, svcReq); err != nil {
		return &schedulerv1.RegisterWorkerResponse{Accepted: false, Message: err.Error()}, nil
	}
	return &schedulerv1.RegisterWorkerResponse{Accepted: true}, nil
}

func (s *Server) Heartbeat(ctx context.Context, req *schedulerv1.HeartbeatRequest) (*schedulerv1.HeartbeatResponse, error) {
	svcReq := service.WorkerHeartbeatRequest{
		WorkerID:        req.GetWorkerId(),
		CurrentLoad:     int(req.GetCurrentLoad()),
		RunningTasks:    int(req.GetRunningTasks()),
		ReportUnixMilli: req.GetReportUnixMilli(),
	}
	if _, err := s.workers.Heartbeat(ctx, svcReq); err != nil {
		return &schedulerv1.HeartbeatResponse{Ok: false}, nil
	}
	return &schedulerv1.HeartbeatResponse{Ok: true}, nil
}

func (s *Server) ReportTaskStatus(ctx context.Context, req *schedulerv1.ReportTaskStatusRequest) (*schedulerv1.ReportTaskStatusResponse, error) {
	svcReq := service.TaskStatusReportRequest{
		ScheduleInstanceID: req.GetScheduleInstanceId(),
		WorkerID:           req.GetWorkerId(),
		Status:             req.GetStatus(),
		ErrorCode:          req.GetErrorCode(),
		ErrorMessage:       req.GetErrorMessage(),
	}
	if err := s.runtime.ReportStatus(ctx, svcReq); err != nil {
		return &schedulerv1.ReportTaskStatusResponse{Ok: false}, nil
	}
	return &schedulerv1.ReportTaskStatusResponse{Ok: true}, nil
}

var ackLogger = slog.Default().With("component", "scheduler-grpc")

func (s *Server) AckTask(ctx context.Context, req *schedulerv1.AckTaskRequest) (*schedulerv1.AckTaskResponse, error) {
	ackLogger.Debug("task acked by worker", "schedule_instance_id", req.GetScheduleInstanceId(), "worker_id", req.GetWorkerId())
	return &schedulerv1.AckTaskResponse{Ok: true}, nil
}

func (s *Server) ReportTaskLog(ctx context.Context, req *schedulerv1.ReportTaskLogRequest) (*schedulerv1.ReportTaskLogResponse, error) {
	ackLogger.Debug("task log", "schedule_instance_id", req.GetScheduleInstanceId(), "worker_id", req.GetWorkerId(), "level", req.GetLogLevel(), "msg", req.GetLogMessage())
	return &schedulerv1.ReportTaskLogResponse{Ok: true}, nil
}
