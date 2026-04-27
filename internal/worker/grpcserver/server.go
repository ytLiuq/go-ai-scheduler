package grpcserver

import (
	"context"

	schedulerv1 "github.com/example/go-ai-scheduler/proto/gen/scheduler/v1"
	"github.com/example/go-ai-scheduler/internal/rpc"
	workerapp "github.com/example/go-ai-scheduler/internal/worker"
	"google.golang.org/grpc"
)

// Server exposes worker execution RPCs to the scheduler.
type Server struct {
	schedulerv1.UnimplementedExecutorServiceServer
	handler *workerapp.Handler
}

// NewServer creates a worker gRPC server adapter.
func NewServer(handler *workerapp.Handler) *Server {
	return &Server{handler: handler}
}

// Register binds the generated executor service stub to a gRPC server.
func Register(s *grpc.Server, impl *Server) {
	schedulerv1.RegisterExecutorServiceServer(s, impl)
}

// ExecuteTask accepts one dispatch RPC.
func (s *Server) ExecuteTask(ctx context.Context, req *schedulerv1.ExecuteTaskRequest) (*schedulerv1.ExecuteTaskResponse, error) {
	s.handler.ExecuteAsync(ctx, rpc.ExecuteTaskRequest{
		ScheduleInstanceID: req.GetScheduleInstanceId(),
		TaskID:             req.GetTaskId(),
		TaskType:           req.GetTaskType(),
		Payload:            req.GetPayload(),
		TimeoutSeconds:     int(req.GetTimeoutSeconds()),
		RetryCount:         int(req.GetRetryCount()),
		SchedulerURL:       req.GetSchedulerUrl(),
	})
	return &schedulerv1.ExecuteTaskResponse{Accepted: true}, nil
}
