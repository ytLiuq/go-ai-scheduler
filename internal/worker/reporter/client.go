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
	schedulerv1 "github.com/example/go-ai-scheduler/proto/gen/scheduler/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client reports task execution results back to scheduler.
type Client struct {
	protocol      string
	httpClient    *http.Client
	grpcConn      *grpc.ClientConn
	controlClient schedulerv1.WorkerControlServiceClient
}

// NewClient creates a scheduler report client.
func NewClient(protocol string, grpcAddr string) *Client {
	c := &Client{
		protocol:   strings.ToLower(strings.TrimSpace(protocol)),
		httpClient: &http.Client{Timeout: 3 * time.Second},
	}
	if c.protocol == "grpc" {
		conn, err := grpc.Dial(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return c
		}
		c.grpcConn = conn
		c.controlClient = schedulerv1.NewWorkerControlServiceClient(conn)
	}
	return c
}

// Ack sends a task receipt acknowledgement to the scheduler.
func (c *Client) Ack(ctx context.Context, schedulerURL, scheduleInstanceID, workerID string) error {
	if c.protocol == "grpc" && c.controlClient != nil {
		resp, err := c.controlClient.AckTask(ctx, &schedulerv1.AckTaskRequest{
			ScheduleInstanceId: scheduleInstanceID,
			WorkerId:           workerID,
		})
		if err != nil {
			return err
		}
		if !resp.GetOk() {
			return fmt.Errorf("grpc ack rejected")
		}
		return nil
	}
	// HTTP fallback: POST /api/v1/task-instances/ack
	body, _ := json.Marshal(map[string]string{"schedule_instance_id": scheduleInstanceID, "worker_id": workerID})
	url := strings.TrimRight(schedulerURL, "/") + "/api/v1/task-instances/ack"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// ReportLog sends a log entry from the worker to the scheduler.
func (c *Client) ReportLog(ctx context.Context, schedulerURL, scheduleInstanceID, workerID, level, message string) error {
	if c.protocol == "grpc" && c.controlClient != nil {
		_, err := c.controlClient.ReportTaskLog(ctx, &schedulerv1.ReportTaskLogRequest{
			ScheduleInstanceId:  scheduleInstanceID,
			WorkerId:            workerID,
			LogLevel:            level,
			LogMessage:          message,
			TimestampUnixSeconds: time.Now().Unix(),
		})
		return err
	}
	return nil // HTTP workers: log reporting is optional
}

// Close releases the underlying gRPC connection.
func (c *Client) Close() error {
	if c.grpcConn != nil {
		return c.grpcConn.Close()
	}
	return nil
}

// Report sends one execution status update.
func (c *Client) Report(ctx context.Context, schedulerURL string, req apiservice.TaskStatusReportRequest) error {
	if c.protocol == "grpc" {
		return c.reportGRPC(ctx, req)
	}
	return c.reportHTTP(ctx, schedulerURL, req)
}

func (c *Client) reportGRPC(ctx context.Context, req apiservice.TaskStatusReportRequest) error {
	if c.controlClient == nil {
		return fmt.Errorf("grpc control client is not initialized")
	}
	resp, err := c.controlClient.ReportTaskStatus(ctx, &schedulerv1.ReportTaskStatusRequest{
		ScheduleInstanceId: req.ScheduleInstanceID,
		WorkerId:           req.WorkerID,
		Status:             req.Status,
		ErrorCode:          req.ErrorCode,
		ErrorMessage:       req.ErrorMessage,
		StartedAt:          req.StartedAt,
		FinishedAt:         req.FinishedAt,
	})
	if err != nil {
		return err
	}
	if !resp.GetOk() {
		return fmt.Errorf("grpc status report rejected")
	}
	return nil
}

func (c *Client) reportHTTP(ctx context.Context, schedulerURL string, req apiservice.TaskStatusReportRequest) error {
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
