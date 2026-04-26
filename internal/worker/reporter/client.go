package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/rpc"
	"github.com/example/go-ai-scheduler/internal/pkg/xgrpc"
)

// Client reports task execution results back to scheduler.
type Client struct {
	protocol   string
	grpcAddr   string
	httpClient *http.Client
}

// NewClient creates a scheduler report client.
func NewClient(protocol string, grpcAddr string) *Client {
	return &Client{
		protocol: strings.ToLower(strings.TrimSpace(protocol)),
		grpcAddr: grpcAddr,
		httpClient: &http.Client{Timeout: 3 * time.Second},
	}
}

// Report sends one execution status update.
func (c *Client) Report(ctx context.Context, schedulerURL string, req apiservice.TaskStatusReportRequest) error {
	if c.protocol == "grpc" {
		var resp xgrpc.AckResponse
		if err := xgrpc.InvokeJSON(ctx, c.grpcAddr, rpc.ReportTaskStatusMethod, req, &resp); err != nil {
			return err
		}
		if !resp.OK {
			return fmt.Errorf("grpc status report rejected: %s", resp.Message)
		}
		return nil
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal status report: %w", err)
	}
	url := strings.TrimRight(schedulerURL, "/") + "/api/v1/task-instances/report"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build status report request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send status report: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("scheduler returned status %s", resp.Status)
	}
	return nil
}
