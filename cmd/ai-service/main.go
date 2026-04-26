package main

import (
	"log"

	"github.com/example/go-ai-scheduler/internal/config"
	"github.com/example/go-ai-scheduler/internal/pkg/logger"
)

func main() {
	cfg := config.Default("ai-service")
	l := logger.New(cfg.ServiceName)
	l.Printf("starting service on %s", cfg.HTTPAddr)
	log.Printf("%s bootstrapped", cfg.ServiceName)
}

