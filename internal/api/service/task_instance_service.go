package service

import (
	"context"
	"errors"
	"sort"

	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/repo"
)

var ErrTaskInstanceIDRequired = errors.New("task instance id is required")

// TaskInstanceService provides read access to scheduled instances.
type TaskInstanceService struct {
	repo repo.TaskInstanceRepository
}

// NewTaskInstanceService creates a TaskInstanceService.
func NewTaskInstanceService(repo repo.TaskInstanceRepository) *TaskInstanceService {
	return &TaskInstanceService{repo: repo}
}

// GetInstance returns one instance by id.
func (s *TaskInstanceService) GetInstance(ctx context.Context, id int64) (*model.TaskInstance, error) {
	if id <= 0 {
		return nil, ErrTaskInstanceIDRequired
	}
	return s.repo.GetInstance(ctx, id)
}

// ListInstancesParams holds optional filters for listing instances.
type ListInstancesParams struct {
	Status string
	TaskID int64
	Limit  int
	Offset int
}

// ListInstances returns all generated instances.
func (s *TaskInstanceService) ListInstances(ctx context.Context) ([]*model.TaskInstance, error) {
	return s.repo.ListInstances(ctx)
}

// InstanceListResult wraps instance list with total count for pagination.
type InstanceListResult struct {
	Instances []*model.TaskInstance `json:"instances"`
	Total     int                   `json:"total"`
}

// ListInstancesWithParams returns instances filtered by the given params.
func (s *TaskInstanceService) ListInstancesWithParams(ctx context.Context, params ListInstancesParams) (*InstanceListResult, error) {
	if params.TaskID > 0 {
		instances, err := s.repo.ListInstancesByTaskID(ctx, params.TaskID)
		if err != nil {
			return nil, err
		}
		if params.Status != "" {
			filtered := make([]*model.TaskInstance, 0)
			for _, inst := range instances {
				if inst.Status == params.Status {
					filtered = append(filtered, inst)
				}
			}
			instances = filtered
		}
		total := len(instances)
		return &InstanceListResult{
			Instances: paginateInstances(instances, params.Limit, params.Offset),
			Total:     total,
		}, nil
	}
	if params.Status != "" {
		instances, err := s.repo.ListInstancesByStatus(ctx, params.Status, params.Limit)
		if err != nil {
			return nil, err
		}
		return &InstanceListResult{Instances: instances, Total: len(instances)}, nil
	}
	// List all instances and paginate in memory.
	all, err := s.repo.ListInstances(ctx)
	if err != nil {
		return nil, err
	}
	return &InstanceListResult{
		Instances: paginateInstances(all, params.Limit, params.Offset),
		Total:     len(all),
	}, nil
}

func paginateInstances(instances []*model.TaskInstance, limit, offset int) []*model.TaskInstance {
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].ID > instances[j].ID
	})
	if offset > 0 && offset < len(instances) {
		instances = instances[offset:]
	}
	if limit > 0 && limit < len(instances) {
		instances = instances[:limit]
	}
	return instances
}

