package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const TraceIDLength = 16

type ctxKey string

const traceIDKey ctxKey = "trace-id"
const spanIDKey ctxKey = "span-id"

var tracerLog = slog.Default().With("component", "tracing")

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

// Setup is a no-op for the lightweight implementation.
func Setup(ctx context.Context, serviceName, endpoint string) (func(context.Context) error, error) {
	if endpoint != "" {
		tracerLog.Info("tracing: lightweight mode, trace IDs propagated via X-Trace-ID header", "otlp", endpoint)
	}
	return func(ctx context.Context) error { return nil }, nil
}

// StartSpan returns a context with a new span ID for lightweight tracing.
func StartSpan(ctx context.Context, operation string) (context.Context, func()) {
	spanID := generateSpanID()
	ctx = context.WithValue(ctx, spanIDKey, spanID)
	startTime := time.Now()
	return ctx, func() {
		tracerLog.Debug("trace span", "op", operation, "span", spanID, "trace", TraceIDFromContext(ctx), "duration", time.Since(startTime))
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

// NewTraceID creates a trace ID.
func NewTraceID() string {
	id := GenerateTraceID()
	return fmt.Sprintf("trace-%s", id[:8])
}
