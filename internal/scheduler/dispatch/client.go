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
	"github.com/example/go-ai-scheduler/internal/rpc"
	"github.com/example/go-ai-scheduler/internal/scheduler/ratelimit"
	schedulerv1 "github.com/example/go-ai-scheduler/proto/gen/scheduler/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client sends tasks to worker callback endpoints.
type Client struct {
	httpClient  *http.Client
	rateLimiter *ratelimit.TokenBucket
}

// NewClient creates a dispatch client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// NewClientWithRateLimiter creates a dispatch client with rate limiting.
func NewClientWithRateLimiter(dispatchRatePerSec int) *Client {
	return &Client{
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		rateLimiter: ratelimit.NewTokenBucket(dispatchRatePerSec, dispatchRatePerSec*2),
	}
}

// Dispatch sends one task to the given worker. Honors rate limit if configured.
func (c *Client) Dispatch(ctx context.Context, worker *model.WorkerNode, req rpc.ExecuteTaskRequest) error {
	if c.rateLimiter != nil {
		if !c.rateLimiter.Allow() {
			return fmt.Errorf("dispatch rate limit exceeded, wait %s", c.rateLimiter.WaitTime(1))
		}
	}
	if strings.EqualFold(worker.Protocol, "grpc") {
		return c.dispatchGRPC(ctx, worker.GRPCAddr, req)
	}
	return c.dispatchHTTP(ctx, worker.CallbackURL, req)
}

// CancelDispatch sends a cancel request to a worker for the given schedule instance.
func (c *Client) CancelDispatch(ctx context.Context, worker *model.WorkerNode, scheduleInstanceID string) error {
	if strings.EqualFold(worker.Protocol, "grpc") {
		return c.cancelGRPC(ctx, worker.GRPCAddr, scheduleInstanceID)
	}
	// For HTTP workers, POST to a cancel endpoint.
	url := strings.TrimRight(worker.CallbackURL, "/") + "/internal/tasks/cancel"
	body := fmt.Sprintf(`{"schedule_instance_id":"%s"}`, scheduleInstanceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cancel dispatch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("worker returned status %s", resp.Status)
	}
	return nil
}

func (c *Client) cancelGRPC(ctx context.Context, target, scheduleInstanceID string) error {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(dialCtx, target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("dial grpc target %s: %w", target, err)
	}
	defer conn.Close()
	client := schedulerv1.NewExecutorServiceClient(conn)
	resp, err := client.CancelTask(ctx, &schedulerv1.CancelTaskRequest{
		ScheduleInstanceId: scheduleInstanceID,
	})
	if err != nil {
		return fmt.Errorf("grpc cancel task: %w", err)
	}
	if !resp.GetOk() {
		return fmt.Errorf("cancel rejected")
	}
	return nil
}

// RateLimiter returns the token bucket, or nil if not configured.
func (c *Client) RateLimiter() *ratelimit.TokenBucket {
	return c.rateLimiter
}

func (c *Client) dispatchGRPC(ctx context.Context, target string, req rpc.ExecuteTaskRequest) error {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("dial grpc target %s: %w", target, err)
	}
	defer conn.Close()

	client := schedulerv1.NewExecutorServiceClient(conn)
	resp, err := client.ExecuteTask(ctx, &schedulerv1.ExecuteTaskRequest{
		ScheduleInstanceId: req.ScheduleInstanceID,
		TaskId:             req.TaskID,
		TaskType:           req.TaskType,
		Payload:            req.Payload,
		Image:              req.Image,
		TimeoutSeconds:     int32(req.TimeoutSeconds),
		RetryCount:         int32(req.RetryCount),
		SchedulerUrl:       req.SchedulerURL,
	})
	if err != nil {
		return fmt.Errorf("grpc execute task: %w", err)
	}
	if !resp.GetAccepted() {
		return fmt.Errorf("grpc worker rejected dispatch: %s", resp.GetMessage())
	}
	return nil
}

func (c *Client) dispatchHTTP(ctx context.Context, callbackURL string, req rpc.ExecuteTaskRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal dispatch request: %w", err)
	}

	url := strings.TrimRight(callbackURL, "/") + "/internal/tasks/execute"
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
