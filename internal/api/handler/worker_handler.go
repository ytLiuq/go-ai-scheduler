package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/example/go-ai-scheduler/internal/api/service"
)

// WorkerHandler exposes worker lifecycle endpoints.
type WorkerHandler struct {
	service *service.WorkerService
}

// NewWorkerHandler creates a WorkerHandler.
func NewWorkerHandler(service *service.WorkerService) *WorkerHandler {
	return &WorkerHandler{service: service}
}

// Register handles worker registration.
func (h *WorkerHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r.Method)
		return
	}

	var req service.WorkerRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	worker, err := h.service.RegisterWorker(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, worker)
}

// Heartbeat handles worker heartbeat updates.
func (h *WorkerHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r.Method)
		return
	}

	var req service.WorkerHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	worker, err := h.service.Heartbeat(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, worker)
}

// List handles worker listing.
func (h *WorkerHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r.Method)
		return
	}

	workers, err := h.service.ListWorkers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, workers)
}

// Get handles worker detail lookup.
func (h *WorkerHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r.Method)
		return
	}

	workerID := strings.TrimPrefix(r.URL.Path, "/api/v1/workers/")
	if strings.TrimSpace(workerID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "worker id is required"})
		return
	}

	worker, err := h.service.GetWorker(r.Context(), workerID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, worker)
}

func writeServiceError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, service.ErrWorkerIDRequired), errors.Is(err, service.ErrWorkerHostRequired):
		status = http.StatusBadRequest
	case strings.Contains(err.Error(), "not found"):
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

