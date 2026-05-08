#!/usr/bin/env bash
# Shared helpers for go-ai-scheduler development scripts.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Load .env.ai-service if present.
load_env() {
  if [[ -f ".env.ai-service" ]]; then
    # shellcheck disable=SC1091
    source ".env.ai-service"
  fi
}

# Ensure required env vars are set.
require_mysql() {
  if [[ "${REPO_BACKEND:-}" != "mysql" ]]; then
    echo "FATAL: REPO_BACKEND must be 'mysql'" >&2
    exit 1
  fi
  if [[ -z "${MYSQL_DSN:-}" ]]; then
    echo "FATAL: MYSQL_DSN is required" >&2
    exit 1
  fi
}

# Kill all tracked background PIDs and matching processes by name.
# Usage: cleanup "$PID1" "$PID2" ... -- pkill_patterns...
cleanup() {
  local code=$?
  local pids=()
  # Collect PIDs (everything before --)
  while [[ $# -gt 0 ]]; do
    if [[ "$1" == "--" ]]; then
      shift
      break
    fi
    pids+=("$1")
    shift
  done
  # Remaining args are pkill patterns
  for pid in "${pids[@]}"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
    fi
  done
  for pattern in "$@"; do
    pkill -f "$pattern" 2>/dev/null || true
  done
  wait 2>/dev/null || true
  exit "$code"
}

# Wait for TCP port to become available.
wait_for_tcp() {
  local name=$1 host=$2 port=$3 attempts=${4:-60}
  for ((i = 1; i <= attempts; i++)); do
    if bash -lc ">/dev/tcp/${host}/${port}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "ERROR: $name did not become ready: ${host}:${port}" >&2
  return 1
}

# Wait for HTTP endpoint to return a successful response.
wait_for_http() {
  local name=$1 url=$2 attempts=${3:-40}
  for ((i = 1; i <= attempts; i++)); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  echo "ERROR: $name did not become ready: $url" >&2
  return 1
}

# Start MySQL/Redis via Docker Compose if not already listening.
ensure_dependency() {
  local name=$1 host=$2 port=$3 compose_service=$4

  if bash -lc ">/dev/tcp/${host}/${port}" >/dev/null 2>&1; then
    echo "Reusing existing ${name} on ${host}:${port}..."
    return 0
  fi

  echo "Starting ${name} with Docker Compose..."
  docker compose -f deployments/docker-compose/docker-compose.yml up -d "${compose_service}"
  wait_for_tcp "${name}" "${host}" "${port}"
}

setup_go_env() {
  export GOCACHE="${GOCACHE:-/tmp/go-build-cache}"
  export GOMODCACHE="${GOMODCACHE:-/tmp/go-mod-cache}"
  export GOPROXY_CANDIDATES="${GOPROXY_CANDIDATES:-https://goproxy.cn,direct;${GOPROXY:-https://proxy.golang.org,direct};direct}"
}

go_with_proxy_fallback() {
  local cmd=("$@")
  local original_ifs="$IFS"
  IFS=';'
  read -r -a proxies <<< "${GOPROXY_CANDIDATES}"
  IFS="$original_ifs"

  local proxy
  local last_rc=0
  for proxy in "${proxies[@]}"; do
    proxy="${proxy#"${proxy%%[![:space:]]*}"}"
    proxy="${proxy%"${proxy##*[![:space:]]}"}"
    [[ -z "$proxy" ]] && continue

    echo "Using GOPROXY=${proxy}"
    if GOPROXY="$proxy" "${cmd[@]}"; then
      return 0
    fi
    last_rc=$?
