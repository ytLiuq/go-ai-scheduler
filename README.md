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
- task CRUD over HTTP
- in-memory repositories for workers, tasks, and task instances
- a scheduler trigger loop that scans `next_trigger_time`, creates task instances, and assigns them to the least-loaded available worker
- worker execution for `shell` and `http` task types
- failure callback and centralized retry handling

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

If you use `memory` repositories, `scheduler` and `api` do not share state across processes.
For shared state across services, run both with `REPO_BACKEND=mysql`.

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
