package grpcserver

import (
	"context"

	"github.com/example/go-ai-scheduler/internal/pkg/xgrpc"
	"github.com/example/go-ai-scheduler/internal/rpc"
	workerapp "github.com/example/go-ai-scheduler/internal/worker"
	"google.golang.org/grpc"
)

// ExecutorService is the gRPC-facing worker execution contract.
type ExecutorService interface {
	ExecuteTask(context.Context, rpc.ExecuteTaskRequest) (*xgrpc.AckResponse, error)
}

// Server exposes worker execution RPCs.
type Server struct {
	handler *workerapp.Handler
}

// NewServer creates a worker gRPC server adapter.
func NewServer(handler *workerapp.Handler) *Server {
	return &Server{handler: handler}
}

// Register binds worker RPC definitions.
func Register(s *grpc.Server, impl *Server) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "worker.v1.ExecutorService",
		HandlerType: (*ExecutorService)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "ExecuteTask",
				Handler:    impl.handleExecuteTask,
			},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "manual",
	}, impl)
}

// ExecuteTask accepts one dispatch RPC.
func (s *Server) ExecuteTask(ctx context.Context, req rpc.ExecuteTaskRequest) (*xgrpc.AckResponse, error) {
	s.handler.ExecuteAsync(ctx, req)
	return &xgrpc.AckResponse{OK: true}, nil
}

func (s *Server) handleExecuteTask(_ any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	var req rpc.ExecuteTaskRequest
	if err := dec(&req); err != nil {
		return nil, err
	}
	return s.ExecuteTask(ctx, req)
}
