package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
	"gorm.io/gorm"
)

// AIAnalysisRepository persists AI analysis records to MySQL.
type AIAnalysisRepository struct {
	db *gorm.DB
}

// NewAIAnalysisRepository creates an AIAnalysisRepository.
func NewAIAnalysisRepository(db *gorm.DB) *AIAnalysisRepository {
	return &AIAnalysisRepository{db: db}
}

// CreateRecord inserts an AI analysis record.
func (r *AIAnalysisRepository) CreateRecord(ctx context.Context, record *model.AIAnalysisRecord) error {
	row := aiAnalysisRecordRow{
		InstanceID:   record.InstanceID,
		AnalysisType: record.AnalysisType,
		InputJSON:    record.InputJSON,
		OutputJSON:   record.OutputJSON,
		Confidence:   record.Confidence,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("insert ai analysis record: %w", err)
	}
	record.ID = row.ID
	record.CreatedAt = row.CreatedAt
	return nil
}

// DeleteOldRecords removes records older than the given time.
func (r *AIAnalysisRepository) DeleteOldRecords(ctx context.Context, before time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Where("created_at < ?", before).Delete(&aiAnalysisRecordRow{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete old ai analysis records: %w", result.Error)
	}
	return result.RowsAffected, nil
}
