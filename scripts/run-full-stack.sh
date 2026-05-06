#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

if [[ ! -f ".env.ai-service" ]]; then
  echo "FATAL: run-full-stack requires .env.ai-service; create it from .env.ai-service.example" >&2
  exit 1
fi

load_env

export REPO_BACKEND="${REPO_BACKEND:-mysql}"
export MYSQL_DSN="${MYSQL_DSN:-root:root@tcp(127.0.0.1:3306)/go_ai_scheduler?parseTime=true}"
export AUTO_MIGRATE=true
export MIGRATION_DIR="${MIGRATION_DIR:-migrations}"
export REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"
export GOCACHE="${GOCACHE:-/tmp/go-build-cache}"
export GOMODCACHE="${GOMODCACHE:-/tmp/go-mod-cache}"

require_mysql

AI_PID=""
API_PID=""
SCHEDULER_PID=""
WORKER_PID=""
RUN_BIN_DIR=""

trap 'cleanup "$AI_PID" "$API_PID" "$SCHEDULER_PID" "$WORKER_PID" -- \
  "/cmd/ai-service" "/cmd/api" "/cmd/scheduler" "/cmd/worker" \
  "ai-service" "api" "scheduler" "worker"' INT TERM EXIT

echo "Ensuring MySQL, Redis, and etcd dependencies..."
ensure_dependency "mysql" "127.0.0.1" "3306" "mysql"
ensure_dependency "redis" "127.0.0.1" "6379" "redis"
ensure_dependency "etcd" "127.0.0.1" "2379" "etcd"

echo "Warming Go module and build caches..."
go mod download

RUN_BIN_DIR="$(mktemp -d /tmp/go-ai-scheduler-run.XXXXXX)"
echo "Building local service binaries into ${RUN_BIN_DIR}..."
go build -o "${RUN_BIN_DIR}/ai-service" ./cmd/ai-service
go build -o "${RUN_BIN_DIR}/scheduler" ./cmd/scheduler
go build -o "${RUN_BIN_DIR}/api" ./cmd/api
go build -o "${RUN_BIN_DIR}/worker" ./cmd/worker

echo "Starting ai-service on :8083..."
env APP_HTTP_ADDR=:8083 "${RUN_BIN_DIR}/ai-service" &
AI_PID=$!
wait_for_http "ai-service" "http://127.0.0.1:8083/healthz" 120

echo "Starting scheduler on :8081 / :9090..."
env APP_HTTP_ADDR=:8081 APP_GRPC_ADDR=:9090 SCHEDULER_URL=http://127.0.0.1:8081 SCHEDULER_GRPC_ADDR=127.0.0.1:9090 ETCD_ENDPOINTS=127.0.0.1:2379 AI_SERVICE_URL=http://127.0.0.1:8083 "${RUN_BIN_DIR}/scheduler" &
SCHEDULER_PID=$!
wait_for_http "scheduler" "http://127.0.0.1:8081/healthz" 180

echo "Starting api on :8082..."
env APP_HTTP_ADDR=:8082 AI_SERVICE_URL=http://127.0.0.1:8083 SCHEDULER_URL=http://127.0.0.1:8081 "${RUN_BIN_DIR}/api" &
API_PID=$!
wait_for_http "api" "http://127.0.0.1:8082/healthz" 120

echo "Starting worker on :8080 / :9091..."
env APP_HTTP_ADDR=:8080 APP_GRPC_ADDR=:9091 SCHEDULER_URL=http://127.0.0.1:8081 SCHEDULER_GRPC_ADDR=127.0.0.1:9090 "${RUN_BIN_DIR}/worker" &
WORKER_PID=$!
wait_for_http "worker" "http://127.0.0.1:8080/healthz" 120

wait -n "$AI_PID" "$API_PID" "$SCHEDULER_PID" "$WORKER_PID"
