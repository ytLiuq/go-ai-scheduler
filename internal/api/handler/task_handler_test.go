package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
	"github.com/example/go-ai-scheduler/internal/repo/memory"
)

func setupTaskHandler() (*TaskHandler, *http.ServeMux) {
	repo := memory.NewTaskRepository()
	svc := apiservice.NewTaskService(repo, nil)
	handler := NewTaskHandler(svc)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tasks", handler.List)
	mux.HandleFunc("/api/v1/tasks/", handler.GetOrUpdate)
	mux.HandleFunc("POST /api/v1/tasks/{id}/pause", handler.Pause)
	mux.HandleFunc("POST /api/v1/tasks/{id}/resume", handler.Resume)
	mux.HandleFunc("POST /api/v1/tasks/{id}/trigger", handler.Trigger)
	return handler, mux
}

func createTestTask(mux *http.ServeMux) int64 {
	body, _ := json.Marshal(map[string]string{
		"name":    "test-task",
		"type":    "shell",
		"payload": "echo ok",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var task struct {
		ID int64 `json:"ID"`
	}
	json.Unmarshal(rec.Body.Bytes(), &task)
	return task.ID
}

func TestTaskPauseResumeHandler(t *testing.T) {
	_, mux := setupTaskHandler()
	id := createTestTask(mux)

	// Pause
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/pause", id), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("pause status=%d body=%s", rec.Code, rec.Body.String())
	}
	var paused map[string]any
	json.Unmarshal(rec.Body.Bytes(), &paused)
	if paused["Status"] != "disabled" {
		t.Fatalf("expected disabled, got %v body=%s", paused["Status"], rec.Body.String())
	}

	// Resume
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/resume", id), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("resume status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resumed map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resumed)
	if resumed["Status"] != "enabled" {
		t.Fatalf("expected enabled, got %v", resumed["Status"])
	}
}

func TestTaskTriggerHandler(t *testing.T) {
	_, mux := setupTaskHandler()
	id := createTestTask(mux)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/trigger", id), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("trigger status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTaskTriggerDisabledFails(t *testing.T) {
	_, mux := setupTaskHandler()
	id := createTestTask(mux)

	// Pause first
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/pause", id), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// Trigger should fail
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/trigger", id), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}
