package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

// AuditEntry represents one audited operation.
type AuditEntry struct {
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
	logger *slog.Logger
}

// NewAuditor creates an Auditor.
func NewAuditor(logger *slog.Logger) *Auditor {
	return &Auditor{logger: logger}
}

// Record emits one audit entry.
func (a *Auditor) Record(_ context.Context, entry AuditEntry) {
	entry.Timestamp = time.Now()
	if entry.Result == "" {
		entry.Result = "success"
	}
	data, err := json.Marshal(entry)
	if err != nil {
		a.logger.Error("audit marshal error", "error", err)
		return
	}
	a.logger.Debug("AUDIT", "data", string(data))
}
