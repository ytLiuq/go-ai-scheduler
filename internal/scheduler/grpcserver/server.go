package grpcserver

import (
	"context"

	"github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/pkg/xgrpc"
	"google.golang.org/grpc"
)

// WorkerControlService is the gRPC-facing scheduler control contract.
type WorkerControlService interface {
	RegisterWorker(context.Context, service.WorkerRegistrationRequest) (*xgrpc.AckResponse, error)
	Heartbeat(context.Context, service.WorkerHeartbeatRequest) (*xgrpc.AckResponse, error)
	ReportTaskStatus(context.Context, service.TaskStatusReportRequest) (*xgrpc.AckResponse, error)
}

// Server exposes internal scheduler control RPCs.
type Server struct {
	workers *service.WorkerService
	runtime *service.TaskRuntimeService
}

// NewServer creates a scheduler gRPC server adapter.
func NewServer(workers *service.WorkerService, runtime *service.TaskRuntimeService) *Server {
	return &Server{workers: workers, runtime: runtime}
}

// Register binds service descriptions to a gRPC server.
func Register(s *grpc.Server, impl *Server) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "scheduler.v1.WorkerControlService",
		HandlerType: (*WorkerControlService)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "RegisterWorker",
				Handler:    impl.handleRegisterWorker,
			},
			{
				MethodName: "Heartbeat",
				Handler:    impl.handleHeartbeat,
			},
			{
				MethodName: "ReportTaskStatus",
				Handler:    impl.handleReportTaskStatus,
			},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "manual",
	}, impl)
}

// RegisterWorker handles one registration RPC.
func (s *Server) RegisterWorker(ctx context.Context, req service.WorkerRegistrationRequest) (*xgrpc.AckResponse, error) {
	if _, err := s.workers.RegisterWorker(ctx, req); err != nil {
		return &xgrpc.AckResponse{OK: false, Message: err.Error()}, nil
	}
	return &xgrpc.AckResponse{OK: true}, nil
}

// Heartbeat handles one heartbeat RPC.
func (s *Server) Heartbeat(ctx context.Context, req service.WorkerHeartbeatRequest) (*xgrpc.AckResponse, error) {
	if _, err := s.workers.Heartbeat(ctx, req); err != nil {
		return &xgrpc.AckResponse{OK: false, Message: err.Error()}, nil
	}
	return &xgrpc.AckResponse{OK: true}, nil
}

// ReportTaskStatus handles one runtime status RPC.
func (s *Server) ReportTaskStatus(ctx context.Context, req service.TaskStatusReportRequest) (*xgrpc.AckResponse, error) {
	if err := s.runtime.ReportStatus(ctx, req); err != nil {
		return &xgrpc.AckResponse{OK: false, Message: err.Error()}, nil
	}
	return &xgrpc.AckResponse{OK: true}, nil
}

func (s *Server) handleRegisterWorker(_ any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	var req service.WorkerRegistrationRequest
	if err := dec(&req); err != nil {
		return nil, err
	}
	return s.RegisterWorker(ctx, req)
}

func (s *Server) handleHeartbeat(_ any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	var req service.WorkerHeartbeatRequest
	if err := dec(&req); err != nil {
		return nil, err
	}
	return s.Heartbeat(ctx, req)
}

func (s *Server) handleReportTaskStatus(_ any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	var req service.TaskStatusReportRequest
	if err := dec(&req); err != nil {
		return nil, err
	}
	return s.ReportTaskStatus(ctx, req)
}
