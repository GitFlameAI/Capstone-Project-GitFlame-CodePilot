# Sprint 5 Reproducibility Runbook

Sprint 5 deployment verification focuses on proving that the final GitFlame CodePilot
stack can be started from a clean Docker state with documented environment variables
and repeatable commands.

## Services

The demo stack is started from the repository root with the base Compose file and the
deployment override:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  up -d --build
```

Expected services:

```text
frontend
backend
database
redis
ml-service
agent-engine
agent-worker
recommendation-service
```

## Environment

Create a local environment file from the template:

```bash
cp .env.example .env
```

Required groups of variables:

```text
# Ports
BACKEND_PORT=8000
ML_SERVICE_PORT=8001
FRONTEND_PORT=80
DATABASE_PORT=5432
REDIS_PORT=6379
AGENT_ENGINE_PORT=8002
RECOMMENDATION_SERVICE_PORT=8003

# Storage and queue
DATABASE_URL=postgresql://gitflame:gitflame@localhost:5432/gitflame_codepilot
REDIS_URL=redis://localhost:6379/0
TASK_DISPATCH_MODE=redis
AGENT_QUEUE_NAME=gitflame:agent:tasks
AGENT_CONSUMER_GROUP=gitflame-agent-workers
AGENT_QUEUE_MAX_LENGTH=1000
WORKER_MAX_RETRIES=3

# Agent Engine and model endpoint
AGENT_ENGINE_URL=http://localhost:8002
AGENT_ENGINE_TIMEOUT_SECONDS=600
AGENT_MODEL=laguna
OPENAI_BASE_URL=https://gpu-1.devops-playground.innopolis.university/v1
OPENAI_API_KEY=
AGENT_FALLBACK_MODEL=
FALLBACK_OPENAI_BASE_URL=
FALLBACK_OPENAI_API_KEY=
MODEL_CONTEXT_LIMIT=32768

# RAG
RAG_BASE_URL=
RAG_API_KEY=

# GitFlame integration
GITFLAME_BASE_URL=
GITFLAME_API_KEY=
GITFLAME_TIMEOUT_SECONDS=30
GITFLAME_CREDENTIAL_KEY=
GITFLAME_CREDENTIAL_KEY_VERSION=1
SESSION_COOKIE_NAME=codepilot_session
SESSION_COOKIE_SECURE=false
SESSION_TTL_HOURS=168

# Recommendation service
RECOMMENDATION_SERVICE_URL=http://localhost:8003
RECOMMENDATION_SERVICE_TIMEOUT_SECONDS=120
```

Do not commit real API keys or GitFlame tokens. `GITFLAME_CREDENTIAL_KEY` is required
only when storing user/repository GitFlame access tokens through the backend connection
flow. For production-like deployments it should be provided through a secret manager or
VM-local `.env` file.

Inside Docker Compose, service URLs use container names. For example, the backend uses
`http://agent-engine:8001`, `http://recommendation-service:7860`,
`postgresql://gitflame:gitflame@database:5432/gitflame_codepilot`, and
`redis://redis:6379/0`.

## Clean Reproducibility Check

Use this sequence when local data can be discarded:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  down -v
```

Then start the stack again:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  up -d --build
```

The PostgreSQL schema is applied on first volume creation from:

```text
backend/db/migrations/initial_schema.sql
```

Additional migration files document incremental schema changes, while the init schema
must represent the current clean-start database state.

## Validation Commands

Validate the merged Compose configuration:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  config --quiet
```

Check containers:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  ps
```

Check HTTP health and readiness:

```bash
curl http://localhost/
curl http://localhost:8000/health
curl http://localhost:8000/ready
curl http://localhost:8001/health
curl http://localhost:8002/health
curl http://localhost:8002/ready
curl http://localhost:8003/health
curl http://localhost:8003/ready
```

Check Redis:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  exec redis redis-cli ping
```

Expected Redis response:

```text
PONG
```

Check the model endpoint when it is reachable from the current network:

```bash
curl "$OPENAI_BASE_URL/models"
```

## CI Summary

The CI workflow is defined in:

```text
.github/workflows/ci.yml
```

It validates:

- frontend dependency installation and production build;
- Go backend tests and server/worker builds;
- Python recommendation service tests and Ruff linting;
- Docker Compose configuration.

## Report Evidence For Roma

Provide these artifacts for the final report:

```text
docker compose ps output after clean startup
curl http://localhost:8000/ready
curl http://localhost:8002/ready
curl http://localhost:8003/ready
Redis PONG output
CI workflow result
VM or demo stand URL, if deployed
```

Known limitation: external GitFlame, RAG, and OpenAI-compatible model checks require
valid runtime URLs and secrets. The repository documents the variables, but secrets
must be supplied outside git.
