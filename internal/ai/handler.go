package ai

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
	aiadvisor "github.com/example/go-ai-scheduler/internal/ai/advisor"
	aicron "github.com/example/go-ai-scheduler/internal/ai/cron"
	"github.com/example/go-ai-scheduler/internal/ai/loganalysis"
	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	"github.com/example/go-ai-scheduler/internal/repo"
)

type cronNextRequest struct {
	Expression string    `json:"expression"`
	BaseTime   time.Time `json:"base_time"`
}

type cronParseRequest struct {
	Input string `json:"input"`
}

type logAnalysisRequest struct {
	Log        string `json:"log"`
	ErrorCode  string `json:"error_code"`
	TaskType   string `json:"task_type"`
	RetryCount int    `json:"retry_count"`
}

type advisorRequest struct {
	AvgWorkerLoad        float64 `json:"avg_worker_load"`
	TotalWorkers         int     `json:"total_workers"`
	OnlineWorkers        int     `json:"online_workers"`
	PendingInstances     int     `json:"pending_instances"`
	FailedLastHour       int     `json:"failed_last_hour"`
	AvgDispatchLatencyMs float64 `json:"avg_dispatch_latency_ms"`
	MaxPendingConfig     int     `json:"max_pending_config"`
}

// NewRouter wires AI helper endpoints.
func NewRouter(llm *adapter.LLMAdapter, aiRepo repo.AIAnalysisRepository) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health)
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, r *http.Request) { status(w, r, llm) })
	mux.Handle("/metrics", metrics.DefaultRegistry.Handler())
	mux.HandleFunc("/api/v1/cron/next", cronNext)
	mux.HandleFunc("POST /api/v1/cron/parse", func(w http.ResponseWriter, r *http.Request) { parseCronNatural(w, r, llm, aiRepo) })
	mux.HandleFunc("/api/v1/log-analysis/analyze", func(w http.ResponseWriter, r *http.Request) { analyzeLog(w, r, llm, aiRepo) })
	mux.HandleFunc("POST /api/v1/advisor/generate", func(w http.ResponseWriter, r *http.Request) { generateAdvice(w, r, llm, aiRepo) })
	return metrics.Instrument("ai-service", mux)
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func status(w http.ResponseWriter, _ *http.Request, llm *adapter.LLMAdapter) {
	model := ""
	endpoint := ""
	llmEnabled := llm != nil && llm.Enabled()
	if llmEnabled {
		model = llm.Model()
		endpoint = llm.Endpoint()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"service":         "ai-service",
		"llm_enabled":     llmEnabled,
		"model":           model,
		"endpoint":        endpoint,
		"api_key_present": llm != nil && llm.HasAPIKey(),
		"server_time":     time.Now().UTC().Format(time.RFC3339),
	})
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

func parseCronNatural(w http.ResponseWriter, r *http.Request, llm *adapter.LLMAdapter, aiRepo repo.AIAnalysisRepository) {
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
	if aiRepo != nil {
		outputJSON, _ := json.Marshal(resp)
		inputJSON, _ := json.Marshal(req)
		persistAIRecord(r, aiRepo, &model.AIAnalysisRecord{
			AnalysisType: "cron_parse",
			InputJSON:    string(inputJSON),
			OutputJSON:   string(outputJSON),
			Confidence:   resp.Confidence,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func analyzeLog(w http.ResponseWriter, r *http.Request, llm *adapter.LLMAdapter, aiRepo repo.AIAnalysisRepository) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r.Method)
		return
	}
	var req logAnalysisRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	result, err := loganalysis.AnalyzeWithLLM(r.Context(), llm, req.Log, req.ErrorCode, req.TaskType, req.RetryCount)
	if err != nil {
		metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "log_analysis", "result": "error"})
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "log_analysis", "result": "ok"})
	if aiRepo != nil {
		outputJSON, _ := json.Marshal(result)
		inputJSON, _ := json.Marshal(req)
		persistAIRecord(r, aiRepo, &model.AIAnalysisRecord{
			AnalysisType: "log_analysis",
			InputJSON:    string(inputJSON),
			OutputJSON:   string(outputJSON),
			Confidence:   result.Confidence,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func generateAdvice(w http.ResponseWriter, r *http.Request, llm *adapter.LLMAdapter, aiRepo repo.AIAnalysisRepository) {
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
	if aiRepo != nil {
		inputJSON, _ := json.Marshal(req)
		outputJSON, _ := json.Marshal(advices)
		var maxConf float64
		for _, a := range advices {
			if a.Confidence > maxConf {
				maxConf = a.Confidence
			}
		}
		persistAIRecord(r, aiRepo, &model.AIAnalysisRecord{
			AnalysisType: "schedule_advice",
			InputJSON:    string(inputJSON),
			OutputJSON:   string(outputJSON),
			Confidence:   maxConf,
		})
	}
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

func persistAIRecord(r *http.Request, aiRepo repo.AIAnalysisRepository, record *model.AIAnalysisRecord) {
	if err := aiRepo.CreateRecord(r.Context(), record); err != nil {
		log.Printf("persist ai analysis record failed: type=%s err=%v", record.AnalysisType, err)
	}
}
