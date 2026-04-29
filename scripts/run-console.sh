#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ -f ".env.ai-service" ]]; then
  # shellcheck disable=SC1091
  source ".env.ai-service"
fi

AI_PID=""
API_PID=""

cleanup() {
  local code=$?
  if [[ -n "$AI_PID" ]] && kill -0 "$AI_PID" 2>/dev/null; then
    kill "$AI_PID" 2>/dev/null || true
  fi
  if [[ -n "$API_PID" ]] && kill -0 "$API_PID" 2>/dev/null; then
    kill "$API_PID" 2>/dev/null || true
  fi
  pkill -f '/cmd/ai-service' 2>/dev/null || true
  pkill -f '/cmd/api' 2>/dev/null || true
  pkill -x ai-service 2>/dev/null || true
  pkill -x api 2>/dev/null || true
  wait 2>/dev/null || true
  exit "$code"
}

trap cleanup INT TERM EXIT

echo "Starting ai-service on ${APP_HTTP_ADDR:-:8083}..."
go run ./cmd/ai-service &
AI_PID=$!

echo "Starting api on :8082..."
APP_HTTP_ADDR=:8082 go run ./cmd/api &
API_PID=$!

wait -n "$AI_PID" "$API_PID"
