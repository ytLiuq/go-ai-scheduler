package memory

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
)

var errWorkerNotFound = errors.New("worker not found")

// WorkerRepository is an in-memory implementation used for local bootstrapping.
type WorkerRepository struct {
	mu      sync.RWMutex
	workers map[string]*model.WorkerNode
}

// NewWorkerRepository creates an empty worker repository.
func NewWorkerRepository() *WorkerRepository {
	return &WorkerRepository{
		workers: make(map[string]*model.WorkerNode),
	}
}

// UpsertWorker inserts or updates a worker snapshot.
func (r *WorkerRepository) UpsertWorker(_ context.Context, worker *model.WorkerNode) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	copyWorker := *worker
	if copyWorker.LastHeartbeatAt.IsZero() {
		copyWorker.LastHeartbeatAt = time.Now()
	}
	r.workers[copyWorker.ID] = &copyWorker
	return nil
}

// GetWorker returns one worker by id.
func (r *WorkerRepository) GetWorker(_ context.Context, id string) (*model.WorkerNode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	worker, ok := r.workers[id]
	if !ok {
		return nil, errWorkerNotFound
	}
	copyWorker := *worker
	return &copyWorker, nil
}

// ListWorkers returns all workers ordered by id for stable output.
func (r *WorkerRepository) ListWorkers(_ context.Context) ([]*model.WorkerNode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	workers := make([]*model.WorkerNode, 0, len(r.workers))
	for _, worker := range r.workers {
		copyWorker := *worker
		workers = append(workers, &copyWorker)
	}
	sort.Slice(workers, func(i, j int) bool {
		return workers[i].ID < workers[j].ID
	})
	return workers, nil
}

// ListAvailableWorkers returns online workers that still have remaining capacity.
func (r *WorkerRepository) ListAvailableWorkers(ctx context.Context) ([]*model.WorkerNode, error) {
	workers, err := r.ListWorkers(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*model.WorkerNode, 0, len(workers))
	for _, worker := range workers {
		if worker.Status != "online" {
			continue
		}
		if worker.CurrentLoad >= worker.MaxConcurrency {
			continue
		}
		result = append(result, worker)
	}
	return result, nil
}

