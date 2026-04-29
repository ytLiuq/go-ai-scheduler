# go-ai-scheduler

`go-ai-scheduler` is a Go-native distributed task scheduling platform with AI-assisted capabilities.

## Scope of v1

The first implementation phase focuses on the deterministic scheduling path:

- task definition and persistence
- scheduler leader election
- worker registration and heartbeat
- task dispatch and execution
- instance status reporting
- retry and timeout control

AI capabilities are isolated as auxiliary services and do not participate in the core scheduling decision path.

## Project Layout

```text
cmd/
  scheduler/    scheduler service entrypoint
  worker/       worker service entrypoint
  api/          management API entrypoint
  ai-service/   AI auxiliary service entrypoint
internal/
  scheduler/    scheduler domain modules
  worker/       worker domain modules
  api/          API modules
  ai/           AI modules
  config/       config types and loader
  model/        domain models
  repo/         repository abstractions
  pkg/          shared infrastructure code
proto/          gRPC protobuf definitions
migrations/     database schema migrations
deployments/    deployment manifests
```

## Milestones

1. scaffold services, schema, and proto contracts
2. implement worker registration and heartbeat
3. implement task CRUD and scheduler trigger loop
4. implement dispatch, execution, retry, and observability
5. integrate AI cron parsing and log analysis

## Current Bootstrap Status

The current local bootstrap implementation supports:

- worker registration and heartbeat over HTTP
- task CRUD over HTTP, including deletion
- in-memory repositories for workers, tasks, and task instances
- cron-based scheduling for `cron_expr` tasks via `next_trigger_time`
- scheduler leader gating: local single-node mode by default, MySQL `GET_LOCK` when using the MySQL repository backend
- a scheduler trigger loop that scans `next_trigger_time`, creates task instances, and assigns them to the least-loaded available worker
- worker execution for `shell` and `http` task types
- failure and timeout callback with centralized retry handling
- `/metrics` endpoints on `scheduler`, `api`, `worker`, and `ai-service`
- `ai-service` helper APIs for cron next-run calculation and log analysis

## Repository Backend

The scheduler supports two repository backends:

- `memory` (default): useful for local feature development
- `mysql`: persistent state for `task`, `task_instance`, and `worker_node`

Use the following environment variables to enable MySQL repositories:

```bash
export REPO_BACKEND=mysql
export MYSQL_DSN='root:root@tcp(127.0.0.1:3306)/go_ai_scheduler?parseTime=true'
```

Enable startup migration execution when using MySQL:

```bash
export AUTO_MIGRATE=true
export MIGRATION_DIR=migrations
```

Or run migrations explicitly:

```bash
REPO_BACKEND=mysql go run ./cmd/migrate
```

## Service Split

- `scheduler`: internal control plane, trigger loop, retry loop, worker registration, worker heartbeat, task runtime report
- `api`: external management and query plane, including task CRUD, worker query, and task instance query
- `ai-service`: auxiliary HTTP service exposing `/api/v1/cron/next` and `/api/v1/log-analysis/analyze`

If you use `memory` repositories, `scheduler` and `api` do not share state across processes.
For shared state across services, run both with `REPO_BACKEND=mysql`.

## AI Service Configuration And Startup

Run the following commands from the repository root:

```bash
cd /root/workspace/go-ai-scheduler
```

A sample environment file is provided at `.env.ai-service.example`.
Do not put real credentials into the example file and commit it.

Create a local private env file first:

```bash
cd /root/workspace/go-ai-scheduler
cp .env.ai-service.example .env.ai-service
```

The repository ignores `.env.ai-service`, so you can store your real key there without committing it.

The `ai-service` reads its LLM configuration from these environment variables:

```bash
export LLM_ENDPOINT='https://api.openai.com/v1'
export LLM_API_KEY='sk-...'
export LLM_MODEL='gpt-4o'
```

Notes:

- `LLM_ENDPOINT` must be the API base URL, not the full `/chat/completions` path
- `LLM_API_KEY` is sent as a Bearer token
- `LLM_MODEL` defaults to `gpt-4o` when unset

If you want AI analysis records to persist to MySQL, also configure:

```bash
export REPO_BACKEND=mysql
export MYSQL_DSN='root:root@tcp(127.0.0.1:3306)/go_ai_scheduler?parseTime=true'
export AUTO_MIGRATE=false
export MIGRATION_DIR=migrations
export REDIS_ADDR='127.0.0.1:6379'
```

Use `go run ./cmd/migrate` to apply migrations explicitly before starting services.
Keeping `AUTO_MIGRATE=false` avoids rerunning non-idempotent migrations on every boot.

You can start both the web console API and `ai-service` with one command:

```bash
cd /root/workspace/go-ai-scheduler
source .env.ai-service
make run-console
```

This runs:

- `ai-service` on `:8083`
- `api` on `:8082`

The script lives at `scripts/run-console.sh` and stops both processes together when you press `Ctrl+C`.

If you only want to start `ai-service`:

```bash
cd /root/workspace/go-ai-scheduler
source .env.ai-service
go run ./cmd/ai-service
```

If you want the external `api` service to proxy AI requests to `ai-service`, configure:

```bash
export AI_SERVICE_URL='http://127.0.0.1:8083'
```

If you only want to start `api`:

```bash
cd /root/workspace/go-ai-scheduler
source .env.ai-service
go run ./cmd/api
```

To start the local MySQL and Redis dependencies for `ai-service`:

```bash
cd /root/workspace/go-ai-scheduler
docker compose -f deployments/docker-compose/docker-compose.yml up -d mysql redis
```

If you also need the scheduler stack later, start etcd too:

```bash
cd /root/workspace/go-ai-scheduler
docker compose -f deployments/docker-compose/docker-compose.yml up -d mysql redis etcd
```

Current AI helper endpoints include:

- `POST /api/v1/cron/parse`
- `POST /api/v1/log-analysis/analyze`
- `POST /api/v1/advisor/generate`
- `POST /api/v1/cron/next`

## Web Console

The management UI is served by the `api` service at:

```text
http://127.0.0.1:8082
```

Demo login accounts:

- `admin / admin123`
- `operator / operator123`
- `viewer / viewer123`

Current console features:

- dashboard summary for tasks, workers, and recent instances
- task list, create, edit, pause, resume, and manual trigger
- worker list and load view
- task instance list
- AI tools for cron parsing, log analysis, and scheduling advice

The AI tools page uses the proxied API endpoints and renders structured results in the console instead of raw JSON.

## Internal Transport

The external management plane is HTTP only.

The internal `scheduler <-> worker` control plane supports both:

- `http`
- `grpc`

Switch worker-side internal transport with:

```bash
export INTERNAL_PROTOCOL=http
```

or:

```bash
export INTERNAL_PROTOCOL=grpc
export SCHEDULER_GRPC_ADDR=127.0.0.1:9090
```

When `INTERNAL_PROTOCOL=grpc`, the worker will:

- register through scheduler gRPC
- send heartbeat through scheduler gRPC
- report execution result through scheduler gRPC
- receive task dispatch through worker gRPC

## Observability

Each service exposes a plain-text Prometheus-style metrics endpoint at `/metrics`.
