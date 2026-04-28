package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/example/go-ai-scheduler/internal/api/service"
)

// TaskRuntimeHandler handles worker execution reports.
type TaskRuntimeHandler struct {
	service *service.TaskRuntimeService
}

// NewTaskRuntimeHandler creates a TaskRuntimeHandler.
func NewTaskRuntimeHandler(service *service.TaskRuntimeService) *TaskRuntimeHandler {
	return &TaskRuntimeHandler{service: service}
}

// Report handles worker status callbacks.
func (h *TaskRuntimeHandler) Report(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r.Method)
		return
	}

	var req service.TaskStatusReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if err := h.service.ReportStatus(r.Context(), req); err != nil {
		writeTaskRuntimeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ack handles POST /api/v1/task-instances/ack.
func (h *TaskRuntimeHandler) Ack(w http.ResponseWriter, r *http.Request) {
	// No-op: worker acknowledgement is informational only.
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Cancel handles POST /api/v1/task-instances/cancel.
func (h *TaskRuntimeHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r.Method)
		return
	}
	var req struct {
		ScheduleInstanceID string `json:"schedule_instance_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if err := h.service.CancelInstance(r.Context(), req.ScheduleInstanceID); err != nil {
		writeTaskRuntimeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func writeTaskRuntimeServiceError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, service.ErrScheduleInstanceIDRequired):
		status = http.StatusBadRequest
	case strings.Contains(err.Error(), "not found"):
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

