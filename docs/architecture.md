# Architecture Notes

## v1 Services

- `scheduler`: deterministic scheduling engine
- `worker`: task executor
- `api`: management and query API
- `ai-service`: AI-assisted log analysis and scheduling advice

## Core Runtime Path

1. task definition stored in MySQL
2. scheduler leader scans due tasks
3. scheduler creates `task_instance`
4. scheduler selects worker and dispatches via gRPC
5. worker executes task and reports status
6. scheduler updates final instance state and retry plan

## Current Runtime Notes

- In `memory` mode, the scheduler elects itself leader locally for single-node development.
- In `mysql` mode, the scheduler uses MySQL `GET_LOCK` for leader gating before starting trigger and retry loops.
- All services expose `/metrics` for process-local counters.
- `ai-service` exposes deterministic helper APIs and remains outside the core scheduling decision path.

## Design Constraints

- AI is not allowed to decide whether a task runs.
- etcd is only used for coordination, not business persistence.
- worker retry must be centralized through scheduler to avoid duplicates.
