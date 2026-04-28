package tracing

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateTraceID(t *testing.T) {
	id := GenerateTraceID()
	if len(id) != 32 {
		t.Fatalf("expected 32 char trace ID, got %d", len(id))
	}
	// Ensure uniqueness is very likely.
	id2 := GenerateTraceID()
	if id == id2 {
		t.Fatal("trace IDs should be unique")
	}
}

func TestWithTraceIDAndFromContext(t *testing.T) {
	ctx := WithTraceID(t.Context(), "abc123")
	if got := TraceIDFromContext(ctx); got != "abc123" {
		t.Fatalf("expected abc123, got %s", got)
	}
}

func TestTraceIDFromContextEmpty(t *testing.T) {
	if got := TraceIDFromContext(t.Context()); got != "" {
		t.Fatalf("expected empty, got %s", got)
	}
}

func TestMiddlewareInjectsTraceID(t *testing.T) {
	var capturedID string
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = TraceIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if capturedID == "" {
		t.Fatal("middleware should inject trace ID")
	}
	if rec.Header().Get("X-Trace-ID") == "" {
		t.Fatal("middleware should set X-Trace-ID header")
	}
}

func TestMiddlewarePropagatesExistingTraceID(t *testing.T) {
	var capturedID string
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = TraceIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Trace-ID", "existing-trace-id")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if capturedID != "existing-trace-id" {
		t.Fatalf("expected existing-trace-id, got %s", capturedID)
	}
}

func TestStartSpan(t *testing.T) {
	ctx := WithTraceID(t.Context(), "trace-1")
	spanCtx, end := StartSpan(ctx, "test-op")
	if spanCtx == nil {
		t.Fatal("span context should not be nil")
	}
	end() // should not panic
}

func TestNewTraceID(t *testing.T) {
	id := NewTraceID()
	if len(id) < 8 {
		t.Fatalf("expected trace ID with prefix, got %s", id)
	}
}
