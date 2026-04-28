package ai

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
	aiadvisor "github.com/example/go-ai-scheduler/internal/ai/advisor"
	aicron "github.com/example/go-ai-scheduler/internal/ai/cron"
	"github.com/example/go-ai-scheduler/internal/ai/loganalysis"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
)

type cronNextRequest struct {
	Expression string    `json:"expression"`
	BaseTime   time.Time `json:"base_time"`
}

type cronParseRequest struct {
	Input string `json:"input"`
}

type logAnalysisRequest struct {
	Log       string `json:"log"`
	ErrorCode string `json:"error_code"`
	TaskType  string `json:"task_type"`
	RetryCount int   `json:"retry_count"`
}

type advisorRequest struct {
	AvgWorkerLoad       float64 `json:"avg_worker_load"`
	TotalWorkers        int     `json:"total_workers"`
	OnlineWorkers       int     `json:"online_workers"`
	PendingInstances    int     `json:"pending_instances"`
	FailedLastHour      int     `json:"failed_last_hour"`
	AvgDispatchLatencyMs float64 `json:"avg_dispatch_latency_ms"`
	MaxPendingConfig    int     `json:"max_pending_config"`
}

// NewRouter wires AI helper endpoints.
func NewRouter(llm *adapter.LLMAdapter) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health)
	mux.Handle("/metrics", metrics.DefaultRegistry.Handler())
	mux.HandleFunc("/api/v1/cron/next", cronNext)
	mux.HandleFunc("POST /api/v1/cron/parse", func(w http.ResponseWriter, r *http.Request) { parseCronNatural(w, r, llm) })
	mux.HandleFunc("/api/v1/log-analysis/analyze", func(w http.ResponseWriter, r *http.Request) { analyzeLog(w, r, llm) })
	mux.HandleFunc("POST /api/v1/advisor/generate", func(w http.ResponseWriter, r *http.Request) { generateAdvice(w, r, llm) })
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

func parseCronNatural(w http.ResponseWriter, r *http.Request, llm *adapter.LLMAdapter) {
	var req cronParseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if req.Input == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input is required"})
		return
	}
	resp, err := aicron.ParseNaturalLanguage(r.Context(), llm, req.Input)
	if err != nil {
		metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "cron_parse", "result": "error"})
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "cron_parse", "result": "ok"})
	writeJSON(w, http.StatusOK, resp)
}

func analyzeLog(w http.ResponseWriter, r *http.Request, llm *adapter.LLMAdapter) {
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
	if llm != nil && llm.Enabled() {
		writeJSON(w, http.StatusOK, loganalysis.AnalyzeWithLLM(r.Context(), llm, req.Log, req.ErrorCode, req.TaskType, req.RetryCount))
	} else {
		writeJSON(w, http.StatusOK, loganalysis.Analyze(req.Log))
	}
}

func generateAdvice(w http.ResponseWriter, r *http.Request, llm *adapter.LLMAdapter) {
	var req advisorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	advices, err := aiadvisor.Generate(r.Context(), llm, aiadvisor.Context{
		AvgWorkerLoad:        req.AvgWorkerLoad,
		TotalWorkers:         req.TotalWorkers,
		OnlineWorkers:        req.OnlineWorkers,
		PendingInstances:     req.PendingInstances,
		FailedLastHour:       req.FailedLastHour,
		AvgDispatchLatencyMs: req.AvgDispatchLatencyMs,
		MaxPendingConfig:     req.MaxPendingConfig,
	})
	if err != nil {
		metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "advisor", "result": "error"})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "advisor", "result": "ok"})
	writeJSON(w, http.StatusOK, map[string]interface{}{"advices": advices})
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
