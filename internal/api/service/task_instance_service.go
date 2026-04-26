package service

import (
	"context"
	"errors"

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

// ListInstances returns all generated instances.
func (s *TaskInstanceService) ListInstances(ctx context.Context) ([]*model.TaskInstance, error) {
	return s.repo.ListInstances(ctx)
}

