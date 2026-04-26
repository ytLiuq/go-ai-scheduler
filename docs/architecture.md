# Architecture Notes

## v1 Services

- `scheduler`: deterministic scheduling engine
- `worker`: task executor
- `api`: management and query API
- `ai-service`: AI-assisted cron parsing and log analysis

## Core Runtime Path

1. task definition stored in MySQL
2. scheduler leader scans due tasks
3. scheduler creates `task_instance`
4. scheduler selects worker and dispatches via gRPC
5. worker executes task and reports status
6. scheduler updates final instance state and retry plan

## Design Constraints

- AI is not allowed to decide whether a task runs.
- etcd is only used for coordination, not business persistence.
- worker retry must be centralized through scheduler to avoid duplicates.

