package memory

import (
	"context"
	"sync"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
)

// AIAnalysisRepository stores AI analysis records in memory.
type AIAnalysisRepository struct {
	mu      sync.Mutex
	nextID  int64
	records map[int64]*model.AIAnalysisRecord
}

// NewAIAnalysisRepository creates an empty analysis repository.
func NewAIAnalysisRepository() *AIAnalysisRepository {
	return &AIAnalysisRepository{
		nextID:  1,
		records: make(map[int64]*model.AIAnalysisRecord),
	}
}

// CreateRecord stores an analysis record.
func (r *AIAnalysisRepository) CreateRecord(_ context.Context, record *model.AIAnalysisRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	record.ID = r.nextID
	record.CreatedAt = time.Now()
	r.records[record.ID] = record
	r.nextID++
	return nil
}
