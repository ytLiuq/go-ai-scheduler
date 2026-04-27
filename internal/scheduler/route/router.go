package route

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/repo"
)

var ErrNoAvailableWorker = errors.New("no available worker")

// SelectOptions control worker selection.
type SelectOptions struct {
	Labels   map[string]string
	Strategy string
}

// Router selects a target worker for one task instance.
type Router struct {
	workers   repo.WorkerRepository
	rrCounter atomic.Int64
}

// NewRouter creates a Router.
func NewRouter(workers repo.WorkerRepository) *Router {
	return &Router{workers: workers}
}

// PickAndReserveWorker returns a worker using the default (least-loaded) strategy.
func (r *Router) PickAndReserveWorker(ctx context.Context) (*model.WorkerNode, error) {
	return r.Pick(ctx, SelectOptions{Strategy: "least_loaded"})
}

// Pick selects a worker matching the given options and reserves its capacity.
func (r *Router) Pick(ctx context.Context, opts SelectOptions) (*model.WorkerNode, error) {
	all, err := r.workers.ListAvailableWorkers(ctx)
	if err != nil {
		return nil, err
	}

	filtered := r.filterByLabels(all, opts.Labels)
	if len(filtered) == 0 {
		return nil, ErrNoAvailableWorker
	}

	var best *model.WorkerNode
	switch opts.Strategy {
	case "round_robin":
		best = r.pickRoundRobin(filtered)
	default:
		best = r.pickLeastLoaded(filtered)
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

func (r *Router) filterByLabels(workers []*model.WorkerNode, selector map[string]string) []*model.WorkerNode {
	if len(selector) == 0 {
		return workers
	}
	var filtered []*model.WorkerNode
	for _, w := range workers {
		if model.MatchLabels(model.DecodeLabels(w.Labels), selector) {
			filtered = append(filtered, w)
		}
	}
	return filtered
}

func (r *Router) pickLeastLoaded(workers []*model.WorkerNode) *model.WorkerNode {
	best := workers[0]
	for _, w := range workers[1:] {
		if w.CurrentLoad < best.CurrentLoad {
			best = w
		}
	}
	return best
}

func (r *Router) pickRoundRobin(workers []*model.WorkerNode) *model.WorkerNode {
	idx := int(r.rrCounter.Add(1)-1) % len(workers)
	return workers[idx]
}
