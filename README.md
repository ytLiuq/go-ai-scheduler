# go-ai-scheduler

`go-ai-scheduler` is a Go-native distributed task scheduling platform with AI-assisted capabilities.

## Architecture

```
                  ┌──────────┐
                  │  Web UI  │  :8082
                  └────┬─────┘
                       │ HTTP (JWT)
                  ┌────▼─────┐      ┌───────────┐
                  │   API    │──────│ ai-service │  :8083
                  └────┬─────┘      └─────┬─────┘
                       │                  │ LLM API
                  ┌────▼─────┐      (OpenAI-compatible)
                  │ Scheduler│  :8081 / :9090 gRPC
                  └────┬─────┘
                  ┌────▼─────┐
                  │  Worker  │  :8080
                  └──────────┘
                       │
                  ┌────▼─────┐
                  │   MySQL  │  :3306
                  │   Redis  │  :6379
                  │   etcd   │  :2379
                  └──────────┘
```

## Features

### Core Scheduling
- Cron-based and event-triggered task execution
- Leader election (MySQL GET_LOCK / etcd)
- Worker registration, heartbeat, and health checking
- Load-aware routing (least-loaded, round-robin)
- Sharded task execution
- Retry with configurable policies (fixed interval, exponential backoff, error-code-based)
- Downstream task chaining via dependencies + DAG visualization
- Webhook event triggers with payload template injection (`{{.event.payload.key}}`)

### AI Capabilities (ai-service, port :8083)

| Endpoint | Description |
|----------|-------------|
| `POST /api/v1/chat` | Conversational AI agent with SSE streaming |
| `GET /api/v1/chat/ws` | Conversational AI agent over WebSocket |
| `POST /api/v1/log-analysis/analyze` | SRE-style log failure analysis |
| `POST /api/v1/advisor/generate` | Scheduling recommendations (throttle/migrate/scale) |
| `POST /api/v1/advisor/auto` | Auto-context advisor (no manual input needed) |
| `POST /api/v1/task/create` | Natural language → task definition |
| `POST /api/v1/task/predict-duration` | Execution time prediction from historical data |
| `POST /api/v1/trend/analyze` | System-wide trend analysis with recommendations |
| `POST /api/v1/cron/next` | Cron next-run computation |
| `GET /api/v1/conversations` | List chat conversations |
| `GET /api/v1/conversations/{id}/messages` | Load chat history |
| `POST /api/v1/events/receive` | Webhook event trigger with payload injection |

### AI Agent Tools
The chat agent has access to 12 tools: `query_tasks`, `query_instances`, `query_workers`, `get_task_detail`, `get_system_health`, `analyze_failure`, `create_task`, `trigger_task`, `pause_task`, `retry_failed_instance`, `delete_task`, `get_worker_load_history`.

### Observability
- Prometheus `/metrics` on all services
- JSON structured logging (`{"ts":"...","level":"INFO","service":"...","msg":"..."}`)
- Token usage tracking (`ai_tokens_total` counter)
- Grafana dashboard with dispatch rate, instance status, AI requests, token usage
- Auto-analysis of failed instances with AI

### Security
- JWT authentication with RBAC (admin/operator/viewer)
- Multi-tenant isolation via JWT claims
- LLM endpoint rate limiting (`AI_RATE_LIMIT_RPM` env var)
- Multi-model fallback (`LLM_FALLBACK_ENDPOINT`)

## Quick Start

### Prerequisites
- Go 1.23+
- MySQL 8.0, Redis 7, etcd (via Docker Compose)
- LLM API key (OpenAI-compatible)

### 1. Start infrastructure
```bash
docker compose -f deployments/docker-compose/docker-compose.yml up -d
```

### 2. Configure AI service
```bash
cp .env.ai-service.example .env.ai-service
# Edit .env.ai-service with your LLM credentials
```

### 3. Run full stack
```bash
make run-full-stack
```

Or run individual services:
```bash
# Console only (API + AI)
make run-console

# Individual services
source .env.ai-service
go run ./cmd/ai-service   # :8083
go run ./cmd/api          # :8082
go run ./cmd/scheduler    # :8081 + :9090 gRPC
go run ./cmd/worker       # :8080
```

### 4. Open web console
```
http://127.0.0.1:8082
```
Demo logins: `admin/admin123`, `operator/operator123`, `viewer/viewer123`

## Configuration

### Core environment variables
| Variable | Default | Description |
|----------|---------|-------------|
| `REPO_BACKEND` | — | Set to `mysql` |
| `MYSQL_DSN` | — | MySQL connection string |
| `REDIS_ADDR` | — | Redis address (optional, enables caching) |
| `ETCD_ENDPOINTS` | — | etcd cluster (for leader election) |

### AI service
| Variable | Default | Description |
|----------|---------|-------------|
| `LLM_ENDPOINT` | — | OpenAI-compatible API base URL |
| `LLM_API_KEY` | — | API key (Bearer token) |
| `LLM_MODEL` | `gpt-4o` | Model name |
| `LLM_FALLBACK_ENDPOINT` | — | Fallback endpoint |
| `LLM_FALLBACK_API_KEY` | — | Fallback API key |
| `LLM_FALLBACK_MODEL` | `gpt-4o` | Fallback model |
| `AI_RATE_LIMIT_RPM` | `0` (no limit) | LLM request rate limit |
| `AI_SERVICE_URL` | `http://127.0.0.1:8083` | API → ai-service proxy target |
| `AI_RATE_LIMIT_RPM` | `0` (no limit) | LLM request rate limit (requests/minute) |
| `JWT_SECRET` | auto-generated | JWT signing key |
| `LOG_LEVEL` | `info` | debug/info/warn/error |
| `SCHEDULER_URL` | `http://127.0.0.1:8081` | Worker/API → scheduler target |

## Deployment

### Docker
```bash
docker build -t go-ai-scheduler .
```
Run individual services:
```bash
docker run go-ai-scheduler /app/scheduler
docker run go-ai-scheduler /app/worker
```

### Kubernetes
```bash
kubectl apply -f deployments/k8s/
```
Manifests include HPA: api (2-8), ai-service (2-6), scheduler (1-3). LLM credentials sourced from `ai-service-llm` Secret.

### Docker Compose (full stack)
```bash
docker compose -f deployments/docker-compose/docker-compose.yml up -d
```
Includes MySQL, Redis, etcd, api, ai-service, scheduler, and worker.

## Project Layout
```
cmd/            Service entrypoints (api, scheduler, worker, ai-service, migrate)
internal/
  ai/           AI modules (adapter, agent, advisor, loganalysis, memory,
                predictduration, prompts, stream, taskparser, tools, trend)
  api/          Management API (handlers, middleware, services)
  scheduler/    Scheduling engine (trigger, retry, dispatch, health, leader)
  worker/       Task executor (executor, reporter, heartbeat, sandbox)
  config/       Configuration loader
  model/        Domain models
  repo/         Repository interfaces + MySQL/test-store implementations
  pkg/          Shared utilities (metrics, logger, cronexpr, xmysql, xredis)
proto/          gRPC definitions + generated code
migrations/     MySQL schema migrations
deployments/    Docker Compose, Kubernetes manifests, Grafana dashboard
```

## CI/CD
GitHub Actions workflow (`.github/workflows/ci.yml`): MySQL + Redis services, migrations, `go test ./...`, `go vet ./...`, `go build ./cmd/...`.
