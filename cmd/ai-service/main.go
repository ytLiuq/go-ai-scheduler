package main

import (
	"context"
	"log/slog"
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
)

func main() {
	cfg := config.Default("ai-service")
	l := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}).WithAttrs([]slog.Attr{slog.String("service", cfg.ServiceName)}))
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
		l.Debug("LLM adapter configured", "endpoint", llmConfig.Endpoint, "model", llmConfig.Model)
		fbCfg := adapter.Config{
			Endpoint: os.Getenv("LLM_FALLBACK_ENDPOINT"),
			APIKey:   os.Getenv("LLM_FALLBACK_API_KEY"),
			Model:    defaultStr(os.Getenv("LLM_FALLBACK_MODEL"), "gpt-4o"),
			Timeout:  30 * time.Second,
		}
		if fb := adapter.New(fbCfg); fb != nil {
			llm.SetFallback(fb)
			l.Debug("LLM fallback configured", "endpoint", fbCfg.Endpoint, "model", fbCfg.Model)
		}
	} else {
		l.Info("LLM adapter not configured, running heuristics-only mode")
	}

	registry := tools.NewRegistry(tools.AllTools(resources.Repositories)...)
	store := memory.NewStore(resources.DB)

	l.Debug("agent tools registered", "count", len(registry.Definitions()))

	rateLimitRPM, _ := strconv.Atoi(os.Getenv("AI_RATE_LIMIT_RPM"))
	var rdb *redis.Client
	if resources.Redis != nil {
		rdb = resources.Redis.Raw()
	}
	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: ai.NewRouter(llm, resources.Repositories, registry, store, rdb, rateLimitRPM),
	}

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		cutoff := time.Now().Add(-90 * 24 * time.Hour)
		if n, err := resources.Repositories.AIAnalysis.DeleteOldRecords(context.Background(), cutoff); err != nil {
			l.Warn("ai analysis cleanup error", "error", err)
		} else if n > 0 {
			l.Debug("ai analysis cleanup: deleted old records", "count", n, "retention", "90d")
		}
		for range ticker.C {
			cutoff := time.Now().Add(-90 * 24 * time.Hour)
			if n, err := resources.Repositories.AIAnalysis.DeleteOldRecords(context.Background(), cutoff); err != nil {
				l.Warn("ai analysis cleanup error", "error", err)
			} else if n > 0 {
				l.Debug("ai analysis cleanup: deleted old records", "count", n, "retention", "90d")
			}
			convCutoff := time.Now().Add(-30 * 24 * time.Hour)
			if n, err := store.DeleteOldConversations(context.Background(), convCutoff); err != nil {
				l.Warn("conversation cleanup error", "error", err)
			} else if n > 0 {
				l.Debug("conversation cleanup: deleted messages from old conversations", "count", n)
			}
		}
	}()

	l.Info("starting ai-service http server", "addr", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		l.Error("ai-service exited with error", "error", err)
		os.Exit(1)
	}
}

func defaultStr(val, def string) string {
	if val == "" {
		return def
	}
	return val
}
