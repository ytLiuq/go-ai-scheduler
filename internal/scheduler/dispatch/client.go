package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/pkg/xgrpc"
	"github.com/example/go-ai-scheduler/internal/rpc"
)

// Client sends tasks to worker callback endpoints.
type Client struct {
	httpClient *http.Client
}

// NewClient creates a dispatch client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Dispatch sends one task to the given worker.
func (c *Client) Dispatch(ctx context.Context, worker *model.WorkerNode, req rpc.ExecuteTaskRequest) error {
	if strings.EqualFold(worker.Protocol, "grpc") {
		var resp xgrpc.AckResponse
		if err := xgrpc.InvokeJSON(ctx, worker.GRPCAddr, rpc.ExecuteTaskMethod, req, &resp); err != nil {
			return err
		}
		if !resp.OK {
			return fmt.Errorf("grpc worker rejected dispatch: %s", resp.Message)
		}
		return nil
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal dispatch request: %w", err)
	}

	url := strings.TrimRight(worker.CallbackURL, "/") + "/internal/tasks/execute"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build dispatch request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("dispatch task: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("worker returned status %s", resp.Status)
	}
	return nil
}
