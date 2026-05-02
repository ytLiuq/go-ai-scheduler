package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
)

// AIAnalysisRepository persists AI analysis records to MySQL.
type AIAnalysisRepository struct {
	db *sql.DB
}

// NewAIAnalysisRepository creates an AIAnalysisRepository.
func NewAIAnalysisRepository(db *sql.DB) *AIAnalysisRepository {
	return &AIAnalysisRepository{db: db}
}

// CreateRecord inserts an AI analysis record.
func (r *AIAnalysisRepository) CreateRecord(ctx context.Context, record *model.AIAnalysisRecord) error {
	const query = `
		INSERT INTO ai_analysis_record (instance_id, analysis_type, input_snapshot, output_json, confidence)
		VALUES (?, ?, ?, ?, ?)
	`
	result, err := r.db.ExecContext(ctx, query,
		record.InstanceID,
		record.AnalysisType,
		record.InputJSON,
		record.OutputJSON,
		record.Confidence,
	)
	if err != nil {
		return fmt.Errorf("insert ai analysis record: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	record.ID = id
	return nil
}

// DeleteOldRecords removes records older than the given time.
func (r *AIAnalysisRepository) DeleteOldRecords(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM ai_analysis_record WHERE created_at < ?`, before,
	)
	if err != nil {
		return 0, fmt.Errorf("delete old ai analysis records: %w", err)
	}
	return result.RowsAffected()
}
