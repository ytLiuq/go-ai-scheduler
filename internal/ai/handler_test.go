package ai

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCronNext(t *testing.T) {
	router := NewRouter(nil, nil)

	body, _ := json.Marshal(map[string]any{
		"expression": "*/15 * * * *",
		"base_time":  time.Date(2026, time.April, 27, 14, 3, 0, 0, time.UTC),
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/cron/next", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "\"next_run\"") {
		t.Fatalf("expected next_run in response: %s", recorder.Body.String())
	}
}

func TestAnalyzeLog(t *testing.T) {
	router := NewRouter(nil, nil)

	body, _ := json.Marshal(map[string]any{
		"log": "request failed: context deadline exceeded",
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/log-analysis/analyze", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "\"timeout\"") {
		t.Fatalf("expected timeout category in response: %s", recorder.Body.String())
	}
}
