package model

import "time"

// AIAnalysisRecord stores structured AI output for audit.
type AIAnalysisRecord struct {
	ID           int64
	InstanceID   int64  // optional, 0 if no instance
	AnalysisType string // log_analysis, schedule_advice, task_parse
	InputJSON    string // snapshot of the input
	OutputJSON   string // structured AI output
	Confidence   float64
	CreatedAt    time.Time
}
