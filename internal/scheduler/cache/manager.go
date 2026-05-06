package cache

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/example/go-ai-scheduler/internal/pkg/xredis"
	"github.com/example/go-ai-scheduler/internal/repo"
)

// Manager coordinates Redis caching of task due-times and worker state.
type Manager struct {
	redis      *xredis.Client
	taskRepo   repo.TaskRepository
	workerRepo repo.WorkerRepository
	logger     *slog.Logger
}

// NewManager creates a cache manager. redis may be nil (no-op mode).
func NewManager(redis *xredis.Client, taskRepo repo.TaskRepository, workerRepo repo.WorkerRepository, l *slog.Logger) *Manager {
	return &Manager{
		redis:      redis,
		taskRepo:   taskRepo,
		workerRepo: workerRepo,
		logger:     l,
	}
}

// Enabled reports whether Redis is available.
func (m *Manager) Enabled() bool {
	return m.redis != nil
}

// StartWarmLoop periodically refreshes the due-task and worker caches.
func (m *Manager) StartWarmLoop(ctx context.Context, interval time.Duration) {
	if !m.Enabled() {
		return
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	m.warm(ctx)
	for {
		select {
		case <-ctx.Done():
			m.logger.Debug("cache warm loop stopped")
			return
		case <-ticker.C:
			m.warm(ctx)
		}
	}
}

func (m *Manager) warm(ctx context.Context) {
	m.warmDueTasks(ctx)
	m.warmWorkerCache(ctx)
}

func (m *Manager) warmDueTasks(ctx context.Context) {
	tasks, err := m.taskRepo.ListDueTasks(ctx, 500)
	if err != nil {
		m.logger.Warn("cache warm: list due tasks failed", "error", err)
		return
	}
	ids := make([]int64, 0, len(tasks))
	scores := make(map[int64]float64, len(tasks))
	now := time.Now()
	for _, t := range tasks {
		ids = append(ids, t.ID)
		scores[t.ID] = float64(t.NextTriggerTime.Unix())
		if t.NextTriggerTime.After(now) && t.NextTriggerTime.Before(now.Add(60*time.Second)) {
			scores[t.ID] = float64(t.NextTriggerTime.Unix())
		}
	}
	if err := m.redis.WarmDueTasks(ctx, ids, scores); err != nil {
		m.logger.Warn("cache warm: write due tasks failed", "error", err)
	}
}

func (m *Manager) warmWorkerCache(ctx context.Context) {
	workers, err := m.workerRepo.ListAvailableWorkers(ctx)
	if err != nil {
		m.logger.Warn("cache warm: list workers failed", "error", err)
		return
	}
	ids := make([]string, 0, len(workers))
	data := make(map[string]map[string]any, len(workers))
	for _, w := range workers {
		ids = append(ids, w.ID)
		data[w.ID] = map[string]any{
			"hostname":        w.Hostname,
			"ip":              w.IP,
			"status":          w.Status,
			"max_concurrency": fmt.Sprintf("%d", w.MaxConcurrency),
			"current_load":    fmt.Sprintf("%d", w.CurrentLoad),
			"labels":          w.Labels,
			"protocol":        w.Protocol,
			"callback_url":    w.CallbackURL,
			"grpc_addr":       w.GRPCAddr,
			"heartbeat":       fmt.Sprintf("%d", w.LastHeartbeatAt.Unix()),
		}
	}
	if err := m.redis.WarmWorkerCache(ctx, ids, data); err != nil {
		m.logger.Warn("cache warm: write worker cache failed", "error", err)
	}
}

// GetCachedDueTaskIDs returns due task IDs within the cutoff from the sorted set.
func (m *Manager) GetCachedDueTaskIDs(ctx context.Context, cutoff time.Time) ([]int64, error) {
	if !m.Enabled() {
		return nil, fmt.Errorf("cache disabled")
	}
	return m.redis.GetDueTaskIDs(ctx, float64(cutoff.Unix()))
}

// IncrWorkerLoad increments the cached load for a worker.
func (m *Manager) IncrWorkerLoad(ctx context.Context, workerID string) (int64, error) {
	if !m.Enabled() {
		return 0, fmt.Errorf("cache disabled")
	}
	return m.redis.IncrWorkerLoad(ctx, workerID)
}

// DecrWorkerLoad decrements the cached load for a worker.
func (m *Manager) DecrWorkerLoad(ctx context.Context, workerID string) (int64, error) {
	if !m.Enabled() {
		return 0, fmt.Errorf("cache disabled")
	}
	return m.redis.DecrWorkerLoad(ctx, workerID)
}

// GetCachedWorkerLoad returns the cached load for a worker from Redis.
func (m *Manager) GetCachedWorkerLoad(ctx context.Context, workerID string) (int64, error) {
	if !m.Enabled() {
		return 0, fmt.Errorf("cache disabled")
	}
	val, err := m.redis.GetCachedWorkerField(ctx, workerID, "current_load")
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(val, 10, 64)
}
