package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/example/go-ai-scheduler/internal/repo"
)

// WorkerLoadHandler exposes historical worker load data.
type WorkerLoadHandler struct {
	repo repo.WorkerLoadRepository
}

// NewWorkerLoadHandler creates a WorkerLoadHandler.
func NewWorkerLoadHandler(repo repo.WorkerLoadRepository) *WorkerLoadHandler {
	return &WorkerLoadHandler{repo: repo}
}

// List returns recent worker load snapshots.
func (h *WorkerLoadHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	workerID := q.Get("worker_id")
	hours := 24
	if v, err := strconv.Atoi(q.Get("hours")); err == nil && v > 0 && v <= 168 {
		hours = v
	}
	limit := 500
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 2000 {
		limit = v
	}

	from := time.Now().Add(-time.Duration(hours) * time.Hour)
	snapshots, err := h.repo.ListSnapshots(r.Context(), workerID, from, time.Now(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, snapshots)
}
