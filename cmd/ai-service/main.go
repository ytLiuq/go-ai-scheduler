package main

import (
	"net/http"

	"github.com/example/go-ai-scheduler/internal/ai"
	"github.com/example/go-ai-scheduler/internal/config"
	"github.com/example/go-ai-scheduler/internal/pkg/logger"
)

func main() {
	cfg := config.Default("ai-service")
	l := logger.New(cfg.ServiceName)
	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: ai.NewRouter(),
	}

	l.Printf("starting ai-service http server on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		l.Fatalf("ai-service exited with error: %v", err)
	}
}
