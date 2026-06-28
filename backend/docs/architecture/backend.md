# Go Backend Architecture - Sprint 3

## Package boundaries

Dependencies point inward toward `internal/domain`:

```text
cmd/server
    |
    v
internal/httpapi --> internal/service --> internal/agent
       |                  |
       +-----------------> internal/repository
                              |
                              v
                        internal/domain
```

- `httpapi` owns routes, JSON encoding, status codes, Swagger, and transport DTOs.
- `service` owns issue validation, workflow transitions, and asynchronous orchestration.
- `agent` owns only the HTTP contract with the SERGE-based Agent Engine.
- `repository` hides storage behind an interface. `MemoryStore` supports isolated tests and `PostgresStore` persists the Sprint 2 workflow.
- `queue` owns the Redis Streams transport contract.
- `domain` contains shared workflow entities and contracts without HTTP or database dependencies.

## Issue-to-plan and code-generation flow

1. GitFlame calls `POST /integrations/gitflame/issues/analyze`.
2. The backend validates issue data, YAML configuration, and repository context.
3. The backend creates a session and an asynchronous task, then returns `202 Accepted`.
4. In queue mode, backend publishes the task to Redis Streams and `agent-worker` calls Agent Engine `POST /v1/plans/generate`.
5. The frontend polls `GET /ai/tasks/{taskId}` until `completed` or `failed`.
6. A correction creates another task and sends `previous_plan` plus `correction_feedback`.
7. Approval stores the user response, prepares the GitFlame branch/commit/PR payload, and enqueues a separate `code_generation` task.
8. The Agent Worker calls Agent Engine `POST /v1/files/generate` with the approved `plan.md`, YAML config, issue metadata, repository metadata, and repository files.
9. The backend validates generated file operations, stores the generated-files contract on the workflow, and exposes it through `GET /ai/issues/{id}/code-generation`.

The Agent Engine request follows the implemented Sprint 2 contract: `request_id`, issue data, repository revision, raw validated `configuration_yaml`, `repository_files`, and nullable correction fields. The response metadata includes relevant files, model ID, token usage, tool-call count, reasoning character count, and generation time.

The Sprint 3 code-generation request is intentionally separate from plan generation. The Agent Engine returns only structured file operations with `action`, `path`, `content` or `diff`, and `explanation`. The backend rejects unsafe paths, duplicate paths, unsupported actions, empty create/modify content, delete operations that include content or diff, and modify/delete operations for files that were not supplied in repository context.

The stored generated-files contract includes:

- `branch_name`
- `commit_message`
- `pr_title`
- `reviewer`
- generated file operations
- Agent Engine request/task identifiers and summary

The Redis worker uses a consumer group, concurrency `1`, bounded retries for temporary Agent Engine errors, and a dead-letter stream for permanent failures. PostgreSQL is authoritative; Redis is only the transport. Local mode remains available for tests and development without Redis.
