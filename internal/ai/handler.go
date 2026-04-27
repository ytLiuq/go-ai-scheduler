package ai

import (
	"encoding/json"
	"net/http"
	"time"

	aicron "github.com/example/go-ai-scheduler/internal/ai/cron"
	"github.com/example/go-ai-scheduler/internal/ai/loganalysis"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
)

type cronNextRequest struct {
	Expression string    `json:"expression"`
	BaseTime   time.Time `json:"base_time"`
}

type logAnalysisRequest struct {
	Log string `json:"log"`
}

// NewRouter wires AI helper endpoints.
func NewRouter() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health)
	mux.Handle("/metrics", metrics.DefaultRegistry.Handler())
	mux.HandleFunc("/api/v1/cron/next", cronNext)
	mux.HandleFunc("/api/v1/log-analysis/analyze", analyzeLog)
	return metrics.Instrument("ai-service", mux)
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func cronNext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r.Method)
		return
	}

	var req cronNextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	response, err := aicron.NextRun(req.Expression, req.BaseTime)
	if err != nil {
		metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "cron_next", "result": "error"})
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "cron_next", "result": "ok"})
	writeJSON(w, http.StatusOK, response)
}

func analyzeLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r.Method)
		return
	}

	var req logAnalysisRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "log_analysis", "result": "ok"})
	writeJSON(w, http.StatusOK, loganalysis.Analyze(req.Log))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func methodNotAllowed(w http.ResponseWriter, method string) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
		"error":  "method not allowed",
		"method": method,
	})
}
