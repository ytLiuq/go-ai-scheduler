package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/example/go-ai-scheduler/internal/api/service"
	schedulerv1 "github.com/example/go-ai-scheduler/proto/gen/scheduler/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client sends worker registration and heartbeat payloads to scheduler.
type HeartbeatClient struct {
	baseURL       string
	protocol      string
	httpClient    *http.Client
	grpcConn      *grpc.ClientConn
	controlClient schedulerv1.WorkerControlServiceClient
}

// NewClient creates a scheduler heartbeat client.
func NewHeartbeatClient(baseURL string, grpcAddr string, protocol string) *HeartbeatClient {
	c := &HeartbeatClient{
		baseURL:    baseURL,
		protocol:   strings.ToLower(strings.TrimSpace(protocol)),
		httpClient: &http.Client{Timeout: 3 * time.Second},
	}
	if c.protocol == "grpc" {
		conn, err := grpc.Dial(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			// Connection creation failed; calls will fall through to error on first RPC.
			return c
		}
		c.grpcConn = conn
		c.controlClient = schedulerv1.NewWorkerControlServiceClient(conn)
	}
	return c
}

// Close releases the underlying gRPC connection.
func (c *HeartbeatClient) Close() error {
	if c.grpcConn != nil {
		return c.grpcConn.Close()
	}
	return nil
}

// Register registers a worker with the scheduler.
func (c *HeartbeatClient) Register(ctx context.Context, req service.WorkerRegistrationRequest) error {
	if c.protocol == "grpc" {
		return c.registerGRPC(ctx, req)
	}
	return c.post(ctx, "/api/v1/workers/register", req)
}

// Heartbeat reports worker liveness to the scheduler.
func (c *HeartbeatClient) Heartbeat(ctx context.Context, req service.WorkerHeartbeatRequest) error {
	if c.protocol == "grpc" {
		return c.heartbeatGRPC(ctx, req)
	}
	return c.post(ctx, "/api/v1/workers/heartbeat", req)
}

func (c *HeartbeatClient) registerGRPC(ctx context.Context, req service.WorkerRegistrationRequest) error {
	if c.controlClient == nil {
		return fmt.Errorf("grpc control client is not initialized")
	}
	resp, err := c.controlClient.RegisterWorker(ctx, &schedulerv1.RegisterWorkerRequest{
		WorkerId:       req.WorkerID,
		Hostname:       req.Hostname,
		Ip:             req.IP,
		CallbackUrl:    req.CallbackURL,
		GrpcAddr:       req.GRPCAddr,
		Protocol:       req.Protocol,
		MaxConcurrency: int32(req.MaxConcurrency),
		Labels:         req.Labels,
	})
	if err != nil {
		return err
	}
	if !resp.GetAccepted() {
		return fmt.Errorf("grpc register worker rejected: %s", resp.GetMessage())
	}
	return nil
}

func (c *HeartbeatClient) heartbeatGRPC(ctx context.Context, req service.WorkerHeartbeatRequest) error {
	if c.controlClient == nil {
		return fmt.Errorf("grpc control client is not initialized")
	}
	resp, err := c.controlClient.Heartbeat(ctx, &schedulerv1.HeartbeatRequest{
		WorkerId:        req.WorkerID,
		CurrentLoad:     int32(req.CurrentLoad),
		RunningTasks:    int32(req.RunningTasks),
		ReportUnixMilli: req.ReportUnixMilli,
	})
	if err != nil {
		return err
	}
	if !resp.GetOk() {
		return fmt.Errorf("grpc heartbeat rejected")
	}
	return nil
}

func (c *HeartbeatClient) post(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		return fmt.Errorf("unexpected scheduler status: %s", response.Status)
	}
	return nil
}
