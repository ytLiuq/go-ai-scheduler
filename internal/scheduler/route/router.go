package route

import (
	"context"
	"errors"

	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/repo"
)

var ErrNoAvailableWorker = errors.New("no available worker")

// Router selects a target worker for one task instance.
type Router struct {
	workers repo.WorkerRepository
}

// NewRouter creates a Router.
func NewRouter(workers repo.WorkerRepository) *Router {
	return &Router{workers: workers}
}

// PickAndReserveWorker returns the least-loaded worker and increments its local load.
func (r *Router) PickAndReserveWorker(ctx context.Context) (*model.WorkerNode, error) {
	workers, err := r.workers.ListAvailableWorkers(ctx)
	if err != nil {
		return nil, err
	}
	if len(workers) == 0 {
		return nil, ErrNoAvailableWorker
	}

	best := workers[0]
	for _, worker := range workers[1:] {
		if worker.CurrentLoad < best.CurrentLoad {
			best = worker
		}
	}

	best.CurrentLoad++
	if err := r.workers.UpsertWorker(ctx, best); err != nil {
		return nil, err
	}
	return best, nil
}

// Release decrements local worker load after execution completes or dispatch fails.
func (r *Router) Release(ctx context.Context, worker *model.WorkerNode) error {
	if worker.CurrentLoad > 0 {
		worker.CurrentLoad--
	}
	return r.workers.UpsertWorker(ctx, worker)
}
