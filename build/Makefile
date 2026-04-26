APP_NAME := go-ai-scheduler

.PHONY: build
build:
	go build ./...

.PHONY: run-scheduler
run-scheduler:
	go run ./cmd/scheduler

.PHONY: run-worker
run-worker:
	go run ./cmd/worker

.PHONY: run-api
run-api:
	go run ./cmd/api

.PHONY: migrate
migrate:
	REPO_BACKEND=mysql go run ./cmd/migrate

.PHONY: run-ai
run-ai:
	go run ./cmd/ai-service
