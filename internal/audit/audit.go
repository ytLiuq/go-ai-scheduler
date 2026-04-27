package audit

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

// Entry represents one audited operation.
type Entry struct {
	Timestamp    time.Time `json:"timestamp"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	TenantID     int64     `json:"tenant_id"`
	Operator     string    `json:"operator"`
	Detail       string    `json:"detail"`
	Result       string    `json:"result"`
}

// Auditor writes audit entries as structured JSON to a logger.
type Auditor struct {
	logger *log.Logger
}

// New creates an Auditor.
func New(logger *log.Logger) *Auditor {
	return &Auditor{logger: logger}
}

// Record emits one audit entry.
func (a *Auditor) Record(_ context.Context, entry Entry) {
	entry.Timestamp = time.Now()
	if entry.Result == "" {
		entry.Result = "success"
	}
	data, err := json.Marshal(entry)
	if err != nil {
		a.logger.Printf("audit marshal error: %v", err)
		return
	}
	a.logger.Printf("AUDIT %s", string(data))
}
