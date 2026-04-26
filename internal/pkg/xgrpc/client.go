package xgrpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
)

// InvokeJSON performs one unary gRPC call using the registered JSON codec.
func InvokeJSON(ctx context.Context, target string, method string, req any, resp any) error {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		dialCtx,
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(encoding.GetCodec(JSONCodecName))),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("dial grpc target %s: %w", target, err)
	}
	defer conn.Close()

	if err := conn.Invoke(ctx, method, req, resp); err != nil {
		return fmt.Errorf("invoke grpc method %s: %w", method, err)
	}
	return nil
}

