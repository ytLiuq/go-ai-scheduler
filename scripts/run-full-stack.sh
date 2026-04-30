#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ ! -f ".env.ai-service" ]]; then
  echo "run-full-stack requires .env.ai-service; create it from .env.ai-service.example" >&2
  exit 1
fi

# shellcheck disable=SC1091
source ".env.ai-service"

export REPO_BACKEND="${REPO_BACKEND:-mysql}"
export MYSQL_DSN="${MYSQL_DSN:-root:root@tcp(127.0.0.1:3306)/go_ai_scheduler?parseTime=true}"
export AUTO_MIGRATE="${AUTO_MIGRATE:-true}"
export MIGRATION_DIR="${MIGRATION_DIR:-migrations}"
export REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"

if [[ "${REPO_BACKEND}" != "mysql" ]]; then
  echo "run-full-stack requires REPO_BACKEND=mysql" >&2
  exit 1
fi

AI_PID=""
API_PID=""
SCHEDULER_PID=""
WORKER_PID=""

cleanup() {
  local code=$?
  for pid in "$AI_PID" "$API_PID" "$SCHEDULER_PID" "$WORKER_PID"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
    fi
  done
  pkill -f '/cmd/ai-service' 2>/dev/null || true
  pkill -f '/cmd/api' 2>/dev/null || true
  pkill -f '/cmd/scheduler' 2>/dev/null || true
  pkill -f '/cmd/worker' 2>/dev/null || true
  pkill -x ai-service 2>/dev/null || true
  pkill -x api 2>/dev/null || true
  pkill -x scheduler 2>/dev/null || true
  pkill -x worker 2>/dev/null || true
  wait 2>/dev/null || true
  exit "$code"
}

trap cleanup INT TERM EXIT

wait_for_tcp() {
  local name=$1
  local host=$2
  local port=$3
  local attempts=${4:-60}
  local i
  for ((i = 1; i <= attempts; i++)); do
    if bash -lc ">/dev/tcp/${host}/${port}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "$name did not become ready: ${host}:${port}" >&2
  return 1
}

ensure_dependency() {
  local name=$1
  local host=$2
  local port=$3
  local compose_service=$4

  if bash -lc ">/dev/tcp/${host}/${port}" >/dev/null 2>&1; then
    echo "Reusing existing ${name} on ${host}:${port}..."
    return 0
  fi

  echo "Starting ${name} with Docker Compose..."
  docker compose -f deployments/docker-compose/docker-compose.yml up -d "${compose_service}"
  wait_for_tcp "${name}" "${host}" "${port}"
}

wait_for_http() {
  local name=$1
  local url=$2
  local attempts=${3:-40}
  local i
  for ((i = 1; i <= attempts; i++)); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  echo "$name did not become ready: $url" >&2
  return 1
}

echo "Ensuring MySQL and Redis dependencies..."
ensure_dependency "mysql" "127.0.0.1" "3306" "mysql"
ensure_dependency "redis" "127.0.0.1" "6379" "redis"

echo "Starting ai-service on :8083..."
env APP_HTTP_ADDR=:8083 go run ./cmd/ai-service &
AI_PID=$!

echo "Starting api on :8082..."
env APP_HTTP_ADDR=:8082 go run ./cmd/api &
API_PID=$!

echo "Starting scheduler on :8080 / :9090..."
env APP_HTTP_ADDR=:8080 APP_GRPC_ADDR=:9090 SCHEDULER_URL=http://127.0.0.1:8080 SCHEDULER_GRPC_ADDR=127.0.0.1:9090 ETCD_ENDPOINTS=, go run ./cmd/scheduler &
SCHEDULER_PID=$!

wait_for_http "scheduler" "http://127.0.0.1:8080/healthz"

echo "Starting worker on :8081 / :9091..."
env APP_HTTP_ADDR=:8081 APP_GRPC_ADDR=:9091 SCHEDULER_URL=http://127.0.0.1:8080 SCHEDULER_GRPC_ADDR=127.0.0.1:9090 go run ./cmd/worker &
WORKER_PID=$!

wait -n "$AI_PID" "$API_PID" "$SCHEDULER_PID" "$WORKER_PID"
