package main

import (
	"net/http"
	"os"
	"strconv"
	"time"

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
	} else {
		l.Printf("LLM adapter not configured, running heuristics-only mode")
	}

	// Wire agent tools and conversation store.
	registry := tools.NewRegistry(tools.AllTools(resources.Repositories)...)
	store := memory.NewStore(resources.DB)

	l.Printf("agent tools registered: %d", len(registry.Definitions()))

	rateLimitRPM, _ := strconv.Atoi(os.Getenv("AI_RATE_LIMIT_RPM"))
	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: ai.NewRouter(llm, resources.Repositories, registry, store, rateLimitRPM),
	}

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
