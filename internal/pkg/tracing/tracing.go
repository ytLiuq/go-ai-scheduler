package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"time"
)

// TraceIDLength is the length of a W3C-compatible trace ID in bytes.
const TraceIDLength = 16

type ctxKey string

const traceIDKey ctxKey = "trace-id"
const spanIDKey ctxKey = "span-id"

// GenerateTraceID returns a hex-encoded 32-char trace ID.
func GenerateTraceID() string {
	b := make([]byte, TraceIDLength)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// WithTraceID attaches a trace ID to the context.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// TraceIDFromContext extracts the trace ID from context.
func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey).(string); ok {
		return v
	}
	return ""
}

// Middleware injects a trace ID into every request if none is present.
// It reads "X-Trace-ID" from the request header or generates a new one.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get("X-Trace-ID")
		if traceID == "" {
			traceID = GenerateTraceID()
		}
		ctx := WithTraceID(r.Context(), traceID)
		w.Header().Set("X-Trace-ID", traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Setup is a no-op for the lightweight implementation. Kept for interface
// compatibility with the OTel version.
func Setup(ctx context.Context, serviceName, endpoint string) (func(context.Context) error, error) {
	if endpoint != "" {
		log.Printf("tracing: lightweight mode, trace IDs propagated via X-Trace-ID header (otlp=%s ignored)", endpoint)
	}
	return func(ctx context.Context) error { return nil }, nil
}

// StartSpan returns a context with a new span ID for lightweight tracing.
func StartSpan(ctx context.Context, operation string) (context.Context, func()) {
	spanID := generateSpanID()
	ctx = context.WithValue(ctx, spanIDKey, spanID)
	startTime := time.Now()
	return ctx, func() {
		duration := time.Since(startTime)
		log.Printf("trace span: op=%s span=%s trace=%s duration=%s",
			operation, spanID, TraceIDFromContext(ctx), duration)
	}
}

func generateSpanID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// DispatchWithTrace attaches a trace ID to dispatch requests.
func DispatchWithTrace(ctx context.Context) string {
	traceID := TraceIDFromContext(ctx)
	if traceID == "" {
		traceID = GenerateTraceID()
	}
	return traceID
}

// NewTraceID creates a trace ID and returns it alongside a string for
// embedding in task instances and dispatches.
func NewTraceID() string {
	id := GenerateTraceID()
	return fmt.Sprintf("trace-%s", id[:8])
}
