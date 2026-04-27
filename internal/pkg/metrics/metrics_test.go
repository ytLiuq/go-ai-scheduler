package metrics

import (
	"strings"
	"testing"
)

func TestRenderCounter(t *testing.T) {
	registry := NewRegistry()
	registry.IncCounter("http_requests_total", map[string]string{
		"service": "api",
		"status":  "200",
	})

	rendered := registry.Render()
	if !strings.Contains(rendered, `http_requests_total{service="api",status="200"} 1`) {
		t.Fatalf("unexpected metrics output: %s", rendered)
	}
}
