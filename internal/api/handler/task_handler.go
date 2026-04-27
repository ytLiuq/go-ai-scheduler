package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/go-ai-scheduler/internal/api/service"
)

// TaskHandler exposes task CRUD endpoints.
type TaskHandler struct {
	service *service.TaskService
}

// NewTaskHandler creates a TaskHandler.
func NewTaskHandler(service *service.TaskService) *TaskHandler {
	return &TaskHandler{service: service}
}

// Create handles task creation.
func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r.Method)
		return
	}

	var req service.TaskUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	task, err := h.service.CreateTask(r.Context(), req)
	if err != nil {
		writeTaskServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

// List handles task listing.
func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.Create(w, r)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r.Method)
		return
	}

	tasks, err := h.service.ListTasks(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

// GetOrUpdate handles task detail, updates, and deletion.
func (h *TaskHandler) GetOrUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseTaskID(r.URL.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task id"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		task, getErr := h.service.GetTask(r.Context(), id)
		if getErr != nil {
			writeTaskServiceError(w, getErr)
			return
		}
		writeJSON(w, http.StatusOK, task)
	case http.MethodPut:
		var req service.TaskUpsertRequest
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		task, updateErr := h.service.UpdateTask(r.Context(), id, req)
		if updateErr != nil {
			writeTaskServiceError(w, updateErr)
			return
		}
		writeJSON(w, http.StatusOK, task)
	case http.MethodDelete:
		if deleteErr := h.service.DeleteTask(r.Context(), id); deleteErr != nil {
			writeTaskServiceError(w, deleteErr)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, r.Method)
	}
}

func parseTaskID(path string) (int64, error) {
	idText := strings.TrimPrefix(path, "/api/v1/tasks/")
	return strconv.ParseInt(idText, 10, 64)
}

func writeTaskServiceError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, service.ErrTaskNameRequired), errors.Is(err, service.ErrTaskTypeRequired), errors.Is(err, service.ErrTaskIDRequired), errors.Is(err, service.ErrInvalidCronExpr):
		status = http.StatusBadRequest
	case strings.Contains(err.Error(), "not found"):
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
