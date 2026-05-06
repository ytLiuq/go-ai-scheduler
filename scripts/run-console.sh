#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

load_env
require_mysql

AI_PID=""
API_PID=""

trap 'cleanup "$AI_PID" "$API_PID" -- "/cmd/ai-service" "/cmd/api" "ai-service" "api"' INT TERM EXIT

echo "Starting ai-service on ${APP_HTTP_ADDR:-:8083}..."
go run ./cmd/ai-service &
AI_PID=$!

echo "Starting api on :8082..."
APP_HTTP_ADDR=:8082 go run ./cmd/api &
API_PID=$!

wait -n "$AI_PID" "$API_PID"
