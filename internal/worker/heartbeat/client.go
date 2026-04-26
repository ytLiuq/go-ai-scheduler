package heartbeat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/rpc"
	"github.com/example/go-ai-scheduler/internal/pkg/xgrpc"
)

// Client sends worker registration and heartbeat payloads to scheduler.
type Client struct {
	baseURL    string
	grpcAddr   string
	protocol   string
	httpClient *http.Client
}

// NewClient creates a scheduler heartbeat client.
func NewClient(baseURL string, grpcAddr string, protocol string) *Client {
	return &Client{
		baseURL: baseURL,
		grpcAddr: grpcAddr,
		protocol: strings.ToLower(strings.TrimSpace(protocol)),
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

// Register registers a worker with the scheduler.
func (c *Client) Register(ctx context.Context, req service.WorkerRegistrationRequest) error {
	if c.protocol == "grpc" {
		var resp xgrpc.AckResponse
		if err := xgrpc.InvokeJSON(ctx, c.grpcAddr, rpc.RegisterWorkerMethod, req, &resp); err != nil {
			return err
		}
		if !resp.OK {
			return fmt.Errorf("grpc register worker rejected: %s", resp.Message)
		}
		return nil
	}
	return c.post(ctx, "/api/v1/workers/register", req)
}

// Heartbeat reports worker liveness to the scheduler.
func (c *Client) Heartbeat(ctx context.Context, req service.WorkerHeartbeatRequest) error {
	if c.protocol == "grpc" {
		var resp xgrpc.AckResponse
		if err := xgrpc.InvokeJSON(ctx, c.grpcAddr, rpc.HeartbeatMethod, req, &resp); err != nil {
			return err
		}
		if !resp.OK {
			return fmt.Errorf("grpc heartbeat rejected: %s", resp.Message)
		}
		return nil
	}
	return c.post(ctx, "/api/v1/workers/heartbeat", req)
}

func (c *Client) post(ctx context.Context, path string, payload any) error {
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
