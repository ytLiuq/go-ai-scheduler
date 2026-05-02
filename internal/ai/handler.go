package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
	aiadvisor "github.com/example/go-ai-scheduler/internal/ai/advisor"
	"github.com/example/go-ai-scheduler/internal/ai/cron"
	"github.com/example/go-ai-scheduler/internal/ai/loganalysis"
	"github.com/example/go-ai-scheduler/internal/ai/memory"
	predictduration "github.com/example/go-ai-scheduler/internal/ai/predictduration"
	"github.com/example/go-ai-scheduler/internal/ai/taskparser"
	"github.com/example/go-ai-scheduler/internal/ai/tools"
	"github.com/example/go-ai-scheduler/internal/ai/trend"
	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	"github.com/example/go-ai-scheduler/internal/repo"
	"github.com/example/go-ai-scheduler/internal/scheduler/ratelimit"
)

type taskParseRequest struct {
	Input string `json:"input"`
}

type cronNextRequest struct {
	Expression string    `json:"expression"`
	BaseTime   time.Time `json:"base_time"`
}

type logAnalysisRequest struct {
	Log        string `json:"log"`
	ErrorCode  string `json:"error_code"`
	TaskType   string `json:"task_type"`
	RetryCount int    `json:"retry_count"`
	InstanceID int64  `json:"instance_id,omitempty"`
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

type predictDurationRequest struct {
	TaskID int64 `json:"task_id"`
}

type trendAnalysisRequest struct {
	TimeWindowHours int `json:"time_window_hours,omitempty"`
}

// NewRouter wires AI helper endpoints.
// rateLimitRPM controls the LLM endpoint rate limit (0 = no limit).
// rdb is an optional Redis client for caching (nil = no cache).
func NewRouter(llm *adapter.LLMAdapter, repos *repo.Bundle, registry *tools.Registry, store *memory.Store, rdb *redis.Client, rateLimitRPM int) http.Handler {
	var rl *ratelimit.TokenBucket
	if rateLimitRPM > 0 {
		rl = ratelimit.NewTokenBucket(rateLimitRPM/60, rateLimitRPM)
	}
	llmGuard := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if rl != nil && !rl.Allow() {
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded, try again later"})
				return
			}
			next(w, r)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health)
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, r *http.Request) { status(w, r, llm) })
	mux.Handle("/metrics", metrics.DefaultRegistry.Handler())
	mux.HandleFunc("/api/v1/cron/next", cronNext)
	mux.HandleFunc("/api/v1/log-analysis/analyze", llmGuard(func(w http.ResponseWriter, r *http.Request) { analyzeLog(w, r, llm, repos) }))
	mux.HandleFunc("POST /api/v1/advisor/generate", llmGuard(func(w http.ResponseWriter, r *http.Request) { generateAdvice(w, r, llm, repos) }))
	mux.HandleFunc("POST /api/v1/advisor/auto", llmGuard(func(w http.ResponseWriter, r *http.Request) { autoAdvice(w, r, llm, repos, rdb) }))
	mux.HandleFunc("POST /api/v1/task/create", llmGuard(func(w http.ResponseWriter, r *http.Request) { parseTaskNatural(w, r, llm, repos) }))
	mux.HandleFunc("POST /api/v1/task/predict-duration", llmGuard(func(w http.ResponseWriter, r *http.Request) { predictDuration(w, r, llm, repos) }))
	mux.HandleFunc("POST /api/v1/trend/analyze", llmGuard(func(w http.ResponseWriter, r *http.Request) { analyzeTrend(w, r, llm, repos, rdb) }))
	mux.HandleFunc("POST /api/v1/chat", llmGuard(func(w http.ResponseWriter, r *http.Request) { handleChat(w, r, llm, registry, store) }))
	mux.HandleFunc("GET /api/v1/chat/ws", func(w http.ResponseWriter, r *http.Request) { handleChatWS(w, r, llm, registry, store) })
	mux.HandleFunc("GET /api/v1/conversations", func(w http.ResponseWriter, r *http.Request) { listConversations(w, r, store) })
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages", func(w http.ResponseWriter, r *http.Request) { getConversationMessages(w, r, store) })
	return metrics.Instrument("ai-service", mux)
}

// cacheListTasks returns cached task list if Redis is available, falls back to DB.
func cacheListTasks(ctx context.Context, rdb *redis.Client, taskRepo repo.TaskRepository) ([]*model.Task, error) {
	if rdb != nil {
		if data, err := rdb.Get(ctx, "ai:cache:tasks").Bytes(); err == nil {
			var tasks []*model.Task
			if json.Unmarshal(data, &tasks) == nil {
				return tasks, nil
			}
		}
	}
	tasks, err := taskRepo.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	if rdb != nil {
		if data, err := json.Marshal(tasks); err == nil {
			rdb.Set(ctx, "ai:cache:tasks", data, 30*time.Second)
		}
	}
	return tasks, nil
}

// cacheListWorkers returns cached worker list if Redis is available, falls back to DB.
func cacheListWorkers(ctx context.Context, rdb *redis.Client, workerRepo repo.WorkerRepository) ([]*model.WorkerNode, error) {
	if rdb != nil {
		if data, err := rdb.Get(ctx, "ai:cache:workers").Bytes(); err == nil {
			var workers []*model.WorkerNode
			if json.Unmarshal(data, &workers) == nil {
				return workers, nil
			}
		}
	}
	workers, err := workerRepo.ListWorkers(ctx)
	if err != nil {
		return nil, err
	}
	if rdb != nil {
		if data, err := json.Marshal(workers); err == nil {
			rdb.Set(ctx, "ai:cache:workers", data, 30*time.Second)
		}
	}
	return workers, nil
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
	promptTokens, completionTokens := int64(0), int64(0)
	if llm != nil {
		promptTokens, completionTokens = llm.TokenUsage()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":            "ok",
		"service":           "ai-service",
		"llm_enabled":       llmEnabled,
		"model":             model,
		"endpoint":          endpoint,
		"api_key_present":   llm != nil && llm.HasAPIKey(),
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"server_time":       time.Now().UTC().Format(time.RFC3339),
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
	response, err := cron.NextRun(req.Expression, req.BaseTime)
	if err != nil {
		metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "cron_next", "result": "error"})
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "cron_next", "result": "ok"})
	writeJSON(w, http.StatusOK, response)
}

func analyzeLog(w http.ResponseWriter, r *http.Request, llm *adapter.LLMAdapter, repos *repo.Bundle) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r.Method)
		return
	}
	var req logAnalysisRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	if req.InstanceID > 0 && repos != nil {
		instance, err := repos.TaskInstance.GetInstance(r.Context(), req.InstanceID)
		if err == nil && instance != nil {
			task, err := repos.Task.GetTask(r.Context(), instance.TaskID)
			if err == nil && task != nil {
				req.Log = fmt.Sprintf(
					"[Task: %s | Type: %s | Timeout: %ds | MaxRetry: %d | RetryPolicy: %s]\n[Instance ID: %d | RetryCount: %d | ErrorCode: %s | Status: %s]\n%s",
					task.Name, task.Type, task.TimeoutSeconds, task.MaxRetry, task.RetryPolicy,
					instance.ID, instance.RetryCount, instance.ErrorCode, instance.Status,
					req.Log,
				)
			}
			if req.ErrorCode == "" {
				req.ErrorCode = instance.ErrorCode
			}
			if req.TaskType == "" && task != nil {
				req.TaskType = task.Type
			}
			req.RetryCount = instance.RetryCount
		}
	}

	result, err := loganalysis.AnalyzeWithLLM(r.Context(), llm, req.Log, req.ErrorCode, req.TaskType, req.RetryCount)
	if err != nil {
		metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "log_analysis", "result": "error"})
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "log_analysis", "result": "ok"})
	if repos != nil && repos.AIAnalysis != nil {
		outputJSON, _ := json.Marshal(result)
		inputJSON, _ := json.Marshal(req)
		persistAIRecord(r, repos.AIAnalysis, &model.AIAnalysisRecord{
			AnalysisType: "log_analysis",
			InputJSON:    string(inputJSON),
			OutputJSON:   string(outputJSON),
			Confidence:   result.Confidence,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func generateAdvice(w http.ResponseWriter, r *http.Request, llm *adapter.LLMAdapter, repos *repo.Bundle) {
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
	if repos != nil && repos.AIAnalysis != nil {
		inputJSON, _ := json.Marshal(req)
		outputJSON, _ := json.Marshal(advices)
		var maxConf float64
		for _, a := range advices {
			if a.Confidence > maxConf {
				maxConf = a.Confidence
			}
		}
		persistAIRecord(r, repos.AIAnalysis, &model.AIAnalysisRecord{
			AnalysisType: "schedule_advice",
			InputJSON:    string(inputJSON),
			OutputJSON:   string(outputJSON),
			Confidence:   maxConf,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"advices": advices})
}

func autoAdvice(w http.ResponseWriter, r *http.Request, llm *adapter.LLMAdapter, repos *repo.Bundle, rdb *redis.Client) {
	if repos == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "repositories not available"})
		return
	}

	workers, err := cacheListWorkers(r.Context(), rdb, repos.Worker)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("list workers: %v", err)})
		return
	}
	tasks, err := cacheListTasks(r.Context(), rdb, repos.Task)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("list tasks: %v", err)})
		return
	}
	instances, err := repos.TaskInstance.ListInstancesByTimeRange(r.Context(), time.Now().Add(-1*time.Hour), time.Now(), 0, 0)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("list instances: %v", err)})
		return
	}

	var totalWorkers, onlineWorkers int
	var totalLoad float64
	for _, w := range workers {
		totalWorkers++
		if w.Status == "online" {
			onlineWorkers++
			if w.MaxConcurrency > 0 {
				totalLoad += float64(w.CurrentLoad) / float64(w.MaxConcurrency)
			}
		}
	}
	avgWorkerLoad := 0.0
	if onlineWorkers > 0 {
		avgWorkerLoad = totalLoad / float64(onlineWorkers)
	}

	var pendingInstances, failedLastHour int
	var totalLatency float64
	var latencyCount int
	cutoff := time.Now().Add(-1 * time.Hour)
	for _, inst := range instances {
		if inst.Status == "pending" || inst.Status == "dispatched" {
			pendingInstances++
		}
		if inst.CreatedAt.After(cutoff) && inst.Status == "failed" {
			failedLastHour++
		}
		if inst.Status == "success" && !inst.DispatchTime.IsZero() {
			lat := inst.UpdatedAt.Sub(inst.DispatchTime).Milliseconds()
			if lat > 0 {
				totalLatency += float64(lat)
				latencyCount++
			}
		}
	}
	avgDispatchLatencyMs := 0.0
	if latencyCount > 0 {
		avgDispatchLatencyMs = totalLatency / float64(latencyCount)
	}

	maxPendingConfig := 0
	for _, t := range tasks {
		if t.TimeoutSeconds > maxPendingConfig {
			maxPendingConfig = t.TimeoutSeconds
		}
	}

	ctx := aiadvisor.Context{
		AvgWorkerLoad:        avgWorkerLoad,
		TotalWorkers:         totalWorkers,
		OnlineWorkers:        onlineWorkers,
		PendingInstances:     pendingInstances,
		FailedLastHour:       failedLastHour,
		AvgDispatchLatencyMs: avgDispatchLatencyMs,
		MaxPendingConfig:     maxPendingConfig,
	}

	advices, err := aiadvisor.Generate(r.Context(), llm, ctx)
	if err != nil {
		metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "auto_advisor", "result": "error"})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "auto_advisor", "result": "ok"})
	if repos.AIAnalysis != nil {
		outputJSON, _ := json.Marshal(advices)
		inputJSON, _ := json.Marshal(ctx)
		var maxConf float64
		for _, a := range advices {
			if a.Confidence > maxConf {
				maxConf = a.Confidence
			}
		}
		persistAIRecord(r, repos.AIAnalysis, &model.AIAnalysisRecord{
			AnalysisType: "auto_advice",
			InputJSON:    string(inputJSON),
			OutputJSON:   string(outputJSON),
			Confidence:   maxConf,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"advices": advices,
		"context": ctx,
	})
}

func predictDuration(w http.ResponseWriter, r *http.Request, llm *adapter.LLMAdapter, repos *repo.Bundle) {
	var req predictDurationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if req.TaskID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task_id is required"})
		return
	}
	if repos == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "repositories not available"})
		return
	}

	task, err := repos.Task.GetTask(r.Context(), req.TaskID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("task not found: %v", err)})
		return
	}

	instances, err := repos.TaskInstance.ListInstancesByTaskID(r.Context(), req.TaskID)
	if err != nil {
		metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "predict_duration", "result": "error"})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	stats := predictduration.ComputeStats(instances)
	if stats.Count == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"predicted_duration_seconds": nil,
			"confidence":                 0,
			"trend":                      "unknown",
			"explanation":                "no completed instances available for prediction",
			"historical_stats":           stats,
		})
		return
	}

	result, err := predictduration.PredictWithLLM(r.Context(), llm, task, stats)
	if err != nil {
		metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "predict_duration", "result": "error"})
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "predict_duration", "result": "ok"})
	if repos.AIAnalysis != nil {
		outputJSON, _ := json.Marshal(result)
		inputJSON, _ := json.Marshal(req)
		persistAIRecord(r, repos.AIAnalysis, &model.AIAnalysisRecord{
			AnalysisType: "predict_duration",
			InputJSON:    string(inputJSON),
			OutputJSON:   string(outputJSON),
			Confidence:   result.Confidence,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func analyzeTrend(w http.ResponseWriter, r *http.Request, llm *adapter.LLMAdapter, repos *repo.Bundle, rdb *redis.Client) {
	var req trendAnalysisRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if req.TimeWindowHours <= 0 {
		req.TimeWindowHours = 24
	}
	if repos == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "repositories not available"})
		return
	}

	workers, err := cacheListWorkers(r.Context(), rdb, repos.Worker)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("list workers: %v", err)})
		return
	}
	tasks, err := cacheListTasks(r.Context(), rdb, repos.Task)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("list tasks: %v", err)})
		return
	}
	windowFrom := time.Now().Add(-time.Duration(req.TimeWindowHours) * time.Hour)
	instances, err := repos.TaskInstance.ListInstancesByTimeRange(r.Context(), windowFrom, time.Now(), 0, 0)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("list instances: %v", err)})
		return
	}

	snapshot := trend.ComputeSnapshot(workers, tasks, instances, req.TimeWindowHours)

	// Enrich with historical load data.
	if repos.WorkerLoad != nil {
		if snapshots, err := repos.WorkerLoad.ListSnapshots(r.Context(), "", windowFrom, time.Now(), 0); err == nil && len(snapshots) > 0 {
			snapshot.TotalLoadSamples = len(snapshots)
			var totalLoad float64
			for _, s := range snapshots {
				if s.MaxConcurrency > 0 {
					totalLoad += float64(s.CurrentLoad) / float64(s.MaxConcurrency)
				}
			}
			if len(snapshots) > 0 {
				snapshot.AvgLoadOverWindow = totalLoad / float64(len(snapshots))
			}
			// Compare first and last samples for direction.
			first := snapshots[len(snapshots)-1]
			last := snapshots[0]
			firstLoad := 0.0
			lastLoad := 0.0
			if first.MaxConcurrency > 0 {
				firstLoad = float64(first.CurrentLoad) / float64(first.MaxConcurrency)
			}
			if last.MaxConcurrency > 0 {
				lastLoad = float64(last.CurrentLoad) / float64(last.MaxConcurrency)
			}
			diff := lastLoad - firstLoad
			if diff > 0.05 {
				snapshot.LoadDirection = "increasing"
			} else if diff < -0.05 {
				snapshot.LoadDirection = "decreasing"
			} else {
				snapshot.LoadDirection = "stable"
			}
		}
	}

	result, err := trend.AnalyzeWithLLM(r.Context(), llm, snapshot)
	if err != nil {
		metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "trend_analysis", "result": "error"})
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "trend_analysis", "result": "ok"})
	if repos.AIAnalysis != nil {
		outputJSON, _ := json.Marshal(result)
		inputJSON, _ := json.Marshal(req)
		persistAIRecord(r, repos.AIAnalysis, &model.AIAnalysisRecord{
			AnalysisType: "trend_analysis",
			InputJSON:    string(inputJSON),
			OutputJSON:   string(outputJSON),
			Confidence:   0,
		})
	}
	writeJSON(w, http.StatusOK, result)
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

func parseTaskNatural(w http.ResponseWriter, r *http.Request, llm *adapter.LLMAdapter, repos *repo.Bundle) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r.Method)
		return
	}
	var req taskParseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if req.Input == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input is required"})
		return
	}
	resp, err := taskparser.ParseNaturalLanguage(r.Context(), llm, req.Input)
	if err != nil {
		metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "task_parse", "result": "error"})
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	metrics.DefaultRegistry.IncCounter("ai_requests_total", map[string]string{"endpoint": "task_parse", "result": "ok"})
	if repos != nil && repos.AIAnalysis != nil {
		outputJSON, _ := json.Marshal(resp)
		inputJSON, _ := json.Marshal(req)
		persistAIRecord(r, repos.AIAnalysis, &model.AIAnalysisRecord{
			AnalysisType: "task_parse",
			InputJSON:    string(inputJSON),
			OutputJSON:   string(outputJSON),
			Confidence:   resp.Confidence,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func persistAIRecord(r *http.Request, aiRepo repo.AIAnalysisRepository, record *model.AIAnalysisRecord) {
	if err := aiRepo.CreateRecord(r.Context(), record); err != nil {
		log.Printf("persist ai analysis record failed: type=%s err=%v", record.AnalysisType, err)
	}
}
