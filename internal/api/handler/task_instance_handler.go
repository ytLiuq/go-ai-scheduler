package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/go-ai-scheduler/internal/api/service"
)

// TaskInstanceHandler exposes read-only task instance endpoints.
type TaskInstanceHandler struct {
	service *service.TaskInstanceService
}

// NewTaskInstanceHandler creates a TaskInstanceHandler.
func NewTaskInstanceHandler(service *service.TaskInstanceService) *TaskInstanceHandler {
	return &TaskInstanceHandler{service: service}
}

// List handles task instance listing.
func (h *TaskInstanceHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r.Method)
		return
	}

	instances, err := h.service.ListInstances(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, instances)
}

// Get handles task instance detail lookup.
func (h *TaskInstanceHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r.Method)
		return
	}

	id, err := parseTaskInstanceID(r.URL.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task instance id"})
		return
	}

	instance, err := h.service.GetInstance(r.Context(), id)
	if err != nil {
		writeTaskInstanceServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, instance)
}

func parseTaskInstanceID(path string) (int64, error) {
	idText := strings.TrimPrefix(path, "/api/v1/task-instances/")
	return strconv.ParseInt(idText, 10, 64)
}

func writeTaskInstanceServiceError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, service.ErrTaskInstanceIDRequired):
		status = http.StatusBadRequest
	case strings.Contains(err.Error(), "not found"):
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

