# Backend

The backend service exposes GitFlame integration endpoints, validates repository configuration, stores workflow state, and communicates with Agent Engine.

Current Sprint 4 Go backend includes:

- `GET /health`
- `GET /ready` with storage, Redis, and Agent Engine dependency checks
- OpenAPI contract at `GET /openapi.json`
- Swagger UI at `GET /swagger/` and `GET /docs`
- asynchronous Agent Engine integration through `POST /v1/plans/generate` and `POST /v1/files/generate`
- issue workflow endpoints:
  - `POST /integrations/gitflame/issues/analyze`
  - `POST /integrations/gitflame/webhooks/issues`
  - `GET /ai/tasks/{taskId}`
  - `POST /ai/tasks/{taskId}/retry`
  - `GET /ai/issues/{id}/plan`
  - `POST /ai/issues/{id}/approve`
  - `GET /ai/issues/{id}/code-generation`
  - `POST /ai/issues/{id}/correct`
  - `POST /ai/issues/{id}/reject`
- recommendation endpoints:
  - `POST /integrations/gitflame/repositories/{id}/recommendations/analyze`
  - `GET /repositories/{id}/recommendations/status`
  - `GET /repositories/{id}/recommendations/summary`
  - `GET /repositories/{id}/recommendations`
  - `PATCH /recommendations/{id}/close`
  - `DELETE /recommendations/{id}`
- asynchronous task states: `queued`, `processing`, `completed`, and `failed`
- correction requests that send both the previous plan and user feedback to Agent Engine
- Agent Engine error mapping for `400`, `404`, `422`, `502`, `503`, and `504`
- code-generation task queued after approval with generated file operations and a GitFlame branch/commit/PR payload
- approval can include an edited `plan_markdown`; backend validates and sends that exact final plan to code generation
- GitFlame webhook ingestion can fetch `.ai.yml`, repository tree, and file contents through `GITFLAME_BASE_URL`
- repository recommendations are generated through the external recommendation service and persisted without a fallback card
- PostgreSQL storage for issue sessions, revisions, agent tasks, and recommendations
- Redis Streams broker with consumer groups, retry, queue limit, acknowledgement, and dead-letter handling
- standalone `cmd/agent-worker` with concurrency `1`
- `.yml` config service for branch prefix, include/exclude patterns, approval commands, and reviewer policy

## Run locally

```bash
set DATABASE_URL=postgresql://gitflame:gitflame@localhost:5432/gitflame_codepilot
go run ./cmd/server
```

Open API docs:

```text
http://localhost:8000/swagger/
```

## Run with Docker Compose

From the repository root:

```bash
docker compose -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml up --build
```

The frontend is exposed on port `80`, and the backend remains available on port `8000`:

```text
http://localhost/
http://localhost:8000/health
```

The backend receives:

```text
AGENT_ENGINE_URL=http://agent-engine:8001
AGENT_ENGINE_TIMEOUT_SECONDS=600
GITFLAME_BASE_URL=
GITFLAME_API_KEY=
RECOMMENDATION_SERVICE_URL=http://recommendations:8000
REDIS_URL=redis://redis:6379/0
AGENT_QUEUE_NAME=gitflame:agent:tasks
DATABASE_URL=postgresql://gitflame:gitflame@database:5432/gitflame_codepilot
AGENT_MODEL=Qwen/Qwen3-Coder-30B-A3B-Instruct
OPENAI_BASE_URL=http://host.docker.internal:9000/v1
```

Compose exposes Agent Engine itself on host port `8002`, while containers use
`http://agent-engine:8001`. `OPENAI_BASE_URL` must point to a running OpenAI-compatible model
server and `OPENAI_API_KEY` must be set when that provider requires authentication.

`REDIS_URL` matches the Compose Redis service (`redis://redis:6379/0`); host-side tools use `redis://localhost:6379/0`. Redis transports tasks, while PostgreSQL remains the source of truth. Set `TASK_DISPATCH_MODE=redis` only when the `agent-worker` service is running. The default `local` mode is convenient for isolated backend development.

PostgreSQL schema can be applied manually:

```bash
psql postgresql://gitflame:gitflame@localhost:5432/gitflame_codepilot \
  -f backend/db/migrations/initial_schema.sql
```

## Build

```bash
go build ./cmd/server
go build ./cmd/agent-worker
```

Run the Redis worker:

```bash
DATABASE_URL=postgresql://gitflame:gitflame@localhost:5432/gitflame_codepilot \
REDIS_URL=redis://localhost:6379/0 \
AGENT_ENGINE_URL=http://localhost:8002 \
go run ./cmd/agent-worker
```

To extend the supplied root Compose file with the worker and queue-mode settings:

```bash
docker compose -f docker-compose.yml -f backend/deploy/docker-compose.sprint2.override.yml up --build
```

## Project structure

The backend follows an idiomatic Go dependency direction instead of placing the whole application in one package:

```text
cmd/server              application entry point
internal/config         environment configuration
internal/domain         API and workflow domain models
internal/agent          SERGE-based Agent Engine HTTP client
internal/httpapi        routing plus GitFlame and recommendation service integration clients
internal/repository     storage contracts and development implementation
internal/queue          Redis Streams broker and job transport
internal/service        validation and issue workflow orchestration
db                      SQL schema and verification scripts
```

See [`docs/architecture/backend.md`](docs/architecture/backend.md) for the boundaries and request flow.
Sprint 3 backend report material is in [`docs/report/backend_version_3.md`](docs/report/backend_version_3.md).
Sprint 4 backend report material is in [`docs/report/backend_version_4.md`](docs/report/backend_version_4.md).
The Redis payload and delivery rules are documented in [`docs/architecture/redis_job_contract.md`](docs/architecture/redis_job_contract.md).
