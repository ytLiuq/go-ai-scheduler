# Architecture

## Services

| Service | Port | Role |
|---------|------|------|
| `api` | :8082 | External management API (JWT auth, RBAC), Web console host, AI proxy |
| `ai-service` | :8083 | AI auxiliary service (LLM agent, analysis, prediction, trend) |
| `scheduler` | :8081 / :9090 | Control plane — trigger loop, retry loop, dispatch, leader election |
| `worker` | :8080 | Task executor (shell, http, container), heartbeat, status reporting |

## Transport

- **External** (user ↔ API): HTTP + JWT. AI chat over WebSocket with SSE fallback.
- **Internal** (API ↔ ai-service): HTTP reverse proxy.
- **Internal** (scheduler ↔ worker): HTTP or gRPC, switchable via `INTERNAL_PROTOCOL`.

## Core Runtime Path

1. Task defined via API or AI natural language → stored in MySQL
2. Scheduler leader election (MySQL `GET_LOCK` or etcd)
3. Timing wheel + trigger loop scans `next_trigger_time` → creates `task_instance`
4. Router picks least-loaded available worker → dispatch (HTTP or gRPC)
5. Worker executes (shell/http/container) → reports status
6. On failure: retry loop handles re-dispatch, AI auto-analyzes
7. On success: downstream dependency tasks advance

## AI Decision Flow

```
User message → API proxy → ai-service → LLM (with tools)
                                           ↓
                              Agent loop: tool_call → execute → tool_result → LLM
                                           ↓
                              SSE/WebSocket → frontend
```

AI does NOT participate in core scheduling decisions. It is advisory only.

## Storage

| System | Purpose |
|--------|---------|
| MySQL | All persistent state (tasks, instances, workers, AI records, conversations) |
| Redis | Cache (due tasks, worker state, worker load, AI query cache). Optional — degrades gracefully to MySQL-only. |
| etcd | Leader election (alternative to MySQL `GET_LOCK`) |

## Design Constraints

- AI is not allowed to decide whether a task runs.
- Worker retry must be centralized through scheduler to avoid duplicates.
- All services are single Go binaries — no application server, no sidecar.
- Redis is optional; the system runs correctly without it.
