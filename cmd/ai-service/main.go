package main

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/example/go-ai-scheduler/internal/ai"
	"github.com/example/go-ai-scheduler/internal/ai/adapter"
	"github.com/example/go-ai-scheduler/internal/ai/memory"
	"github.com/example/go-ai-scheduler/internal/ai/tools"
	"github.com/example/go-ai-scheduler/internal/app"
	"github.com/example/go-ai-scheduler/internal/config"
	"github.com/example/go-ai-scheduler/internal/pkg/logger"
)

func main() {
	cfg := config.Default("ai-service")
	l := logger.New(cfg.ServiceName)
	resources, cleanup := app.BuildResources(cfg, l)
	defer cleanup()

	llmConfig := adapter.Config{
		Endpoint: os.Getenv("LLM_ENDPOINT"),
		APIKey:   os.Getenv("LLM_API_KEY"),
		Model:    defaultStr(os.Getenv("LLM_MODEL"), "gpt-4o"),
		Timeout:  30 * time.Second,
	}
	llm := adapter.New(llmConfig)
	if llm != nil && llm.Enabled() {
		l.Printf("LLM adapter configured: endpoint=%s model=%s", llmConfig.Endpoint, llmConfig.Model)
		// Configure fallback LLM if env vars are set.
		fbCfg := adapter.Config{
			Endpoint: os.Getenv("LLM_FALLBACK_ENDPOINT"),
			APIKey:   os.Getenv("LLM_FALLBACK_API_KEY"),
			Model:    defaultStr(os.Getenv("LLM_FALLBACK_MODEL"), "gpt-4o"),
			Timeout:  30 * time.Second,
		}
		if fb := adapter.New(fbCfg); fb != nil {
			llm.SetFallback(fb)
			l.Printf("LLM fallback configured: endpoint=%s model=%s", fbCfg.Endpoint, fbCfg.Model)
		}
	} else {
		l.Printf("LLM adapter not configured, running heuristics-only mode")
	}

	// Wire agent tools and conversation store.
	registry := tools.NewRegistry(tools.AllTools(resources.Repositories)...)
	store := memory.NewStore(resources.DB)

	l.Printf("agent tools registered: %d", len(registry.Definitions()))

	rateLimitRPM, _ := strconv.Atoi(os.Getenv("AI_RATE_LIMIT_RPM"))
	var rdb *redis.Client
	if resources.Redis != nil {
		rdb = resources.Redis.Raw()
	}
	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: ai.NewRouter(llm, resources.Repositories, registry, store, rdb, rateLimitRPM),
	}

	// Start periodic cleanup of old AI analysis records (retain 90 days).
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		cutoff := time.Now().Add(-90 * 24 * time.Hour)
		if n, err := resources.Repositories.AIAnalysis.DeleteOldRecords(context.Background(), cutoff); err != nil {
			l.Printf("ai analysis cleanup error: %v", err)
		} else if n > 0 {
			l.Printf("ai analysis cleanup: deleted %d records older than 90 days", n)
		}
		for range ticker.C {
			cutoff := time.Now().Add(-90 * 24 * time.Hour)
			if n, err := resources.Repositories.AIAnalysis.DeleteOldRecords(context.Background(), cutoff); err != nil {
				l.Printf("ai analysis cleanup error: %v", err)
			} else if n > 0 {
				l.Printf("ai analysis cleanup: deleted %d records older than 90 days", n)
			}
		}
	}()

	l.Printf("starting ai-service http server on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		l.Fatalf("ai-service exited with error: %v", err)
	}
}

func defaultStr(val, def string) string {
	if val == "" {
		return def
	}
	return val
}
