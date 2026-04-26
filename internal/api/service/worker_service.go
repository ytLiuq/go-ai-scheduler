package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/repo"
)

var (
	ErrWorkerIDRequired   = errors.New("worker_id is required")
	ErrWorkerHostRequired = errors.New("hostname is required")
)

// WorkerRegistrationRequest describes scheduler-side worker onboarding input.
type WorkerRegistrationRequest struct {
	WorkerID       string            `json:"worker_id"`
	Hostname       string            `json:"hostname"`
	IP             string            `json:"ip"`
	CallbackURL    string            `json:"callback_url"`
	GRPCAddr       string            `json:"grpc_addr"`
	Protocol       string            `json:"protocol"`
	MaxConcurrency int               `json:"max_concurrency"`
	Labels         map[string]string `json:"labels"`
}

// WorkerHeartbeatRequest describes runtime liveness updates from workers.
type WorkerHeartbeatRequest struct {
	WorkerID        string `json:"worker_id"`
	CurrentLoad     int    `json:"current_load"`
	RunningTasks    int    `json:"running_tasks"`
	ReportUnixMilli int64  `json:"report_unix_milli"`
}

// WorkerService manages worker lifecycle state.
type WorkerService struct {
	repo repo.WorkerRepository
}

// NewWorkerService creates a WorkerService.
func NewWorkerService(repo repo.WorkerRepository) *WorkerService {
	return &WorkerService{repo: repo}
}

// RegisterWorker adds or refreshes a worker record.
func (s *WorkerService) RegisterWorker(ctx context.Context, req WorkerRegistrationRequest) (*model.WorkerNode, error) {
	if strings.TrimSpace(req.WorkerID) == "" {
		return nil, ErrWorkerIDRequired
	}
	if strings.TrimSpace(req.Hostname) == "" {
		return nil, ErrWorkerHostRequired
	}
	if req.MaxConcurrency <= 0 {
		req.MaxConcurrency = 100
	}

	worker := &model.WorkerNode{
		ID:              req.WorkerID,
		Hostname:        req.Hostname,
		IP:              req.IP,
		CallbackURL:     req.CallbackURL,
		GRPCAddr:        req.GRPCAddr,
		Protocol:        defaultProtocol(req.Protocol),
		Status:          "online",
		Labels:          model.EncodeLabels(req.Labels),
		MaxConcurrency:  req.MaxConcurrency,
		CurrentLoad:     0,
		LastHeartbeatAt: time.Now(),
	}

	if err := s.repo.UpsertWorker(ctx, worker); err != nil {
		return nil, err
	}
	return s.repo.GetWorker(ctx, req.WorkerID)
}

// Heartbeat updates transient worker load and liveness status.
func (s *WorkerService) Heartbeat(ctx context.Context, req WorkerHeartbeatRequest) (*model.WorkerNode, error) {
	if strings.TrimSpace(req.WorkerID) == "" {
		return nil, ErrWorkerIDRequired
	}

	worker, err := s.repo.GetWorker(ctx, req.WorkerID)
	if err != nil {
		return nil, err
	}

	worker.Status = "online"
	worker.CurrentLoad = req.CurrentLoad
	if req.RunningTasks > worker.CurrentLoad {
		worker.CurrentLoad = req.RunningTasks
	}
	worker.LastHeartbeatAt = heartbeatTime(req.ReportUnixMilli)

	if err := s.repo.UpsertWorker(ctx, worker); err != nil {
		return nil, err
	}
	return s.repo.GetWorker(ctx, req.WorkerID)
}

// GetWorker returns one worker snapshot.
func (s *WorkerService) GetWorker(ctx context.Context, workerID string) (*model.WorkerNode, error) {
	return s.repo.GetWorker(ctx, workerID)
}

// ListWorkers returns all workers.
func (s *WorkerService) ListWorkers(ctx context.Context) ([]*model.WorkerNode, error) {
	return s.repo.ListWorkers(ctx)
}

func heartbeatTime(unixMilli int64) time.Time {
	if unixMilli <= 0 {
		return time.Now()
	}
	return time.UnixMilli(unixMilli)
}

func defaultProtocol(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "grpc") {
		return "grpc"
	}
	return "http"
}
