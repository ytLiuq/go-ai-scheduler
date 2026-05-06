package metrics

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type metricKey struct {
	name   string
	labels string
}

// Registry stores process-local counters and renders them in Prometheus text format.
type Registry struct {
	mu       sync.RWMutex
	counters map[metricKey]*int64
}

// DefaultRegistry is used by the bootstrap services.
var DefaultRegistry = NewRegistry()

// NewRegistry creates an empty metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		counters: make(map[metricKey]*int64),
	}
}

// IncCounter increments one counter by 1.
func (r *Registry) IncCounter(name string, labels map[string]string) {
	r.AddCounter(name, labels, 1)
}

// AddCounter increments one counter by delta.
func (r *Registry) AddCounter(name string, labels map[string]string, delta int64) {
	key := metricKey{name: sanitizeName(name), labels: encodeLabels(labels)}

	r.mu.RLock()
	counter, ok := r.counters[key]
	r.mu.RUnlock()
	if ok {
		atomic.AddInt64(counter, delta)
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if counter, ok = r.counters[key]; ok {
		atomic.AddInt64(counter, delta)
		return
	}
	counter = new(int64)
	r.counters[key] = counter
	atomic.AddInt64(counter, delta)
}

// Handler exposes registry values in Prometheus text format.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(r.Render()))
	})
}

// Render returns the registry in Prometheus text exposition format.
func (r *Registry) Render() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := make([]metricKey, 0, len(r.counters))
	for key := range r.counters {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].name == keys[j].name {
			return keys[i].labels < keys[j].labels
		}
		return keys[i].name < keys[j].name
	})

	var builder strings.Builder
	for _, key := range keys {
		value := atomic.LoadInt64(r.counters[key])
		builder.WriteString(key.name)
		if key.labels != "" {
			builder.WriteByte('{')
			builder.WriteString(key.labels)
			builder.WriteByte('}')
		}
		builder.WriteByte(' ')
		builder.WriteString(fmt.Sprintf("%d\n", value))
	}
	return builder.String()
}

// Instrument wraps one HTTP handler and records request counters.
func Instrument(service string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)

		DefaultRegistry.IncCounter("http_requests_total", map[string]string{
			"service": service,
			"method":  r.Method,
			"path":    r.URL.Path,
			"status":  fmt.Sprintf("%d", recorder.status),
		})
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("metrics: underlying ResponseWriter does not support hijacking")
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "unnamed_metric"
	}
	name = strings.ReplaceAll(name, "-", "_")
	return name
}

func encodeLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", key, labels[key]))
	}
	return strings.Join(parts, ",")
}
