# Infrastructure

This folder stores Sprint 1 infrastructure notes. The runnable Docker setup is defined in the root `docker-compose.yml`.

## Services

The current Compose setup starts:

- `backend`: Go backend API on port `8000`.
- `ml-service`: mock ML service on port `8001`.
- `database`: PostgreSQL 16 on port `5432`.
- `redis`: Redis 7 broker on port `6379`.

The backend receives the internal Compose database connection string through:

```text
DATABASE_URL=postgresql://gitflame:gitflame@database:5432/gitflame_codepilot
```

For local host tools such as `psql` or pgAdmin, use:

```text
DATABASE_URL=postgresql://gitflame:gitflame@localhost:5432/gitflame_codepilot
```

The backend also receives the internal Redis URL for Sprint 2 agent tasks:

```text
REDIS_URL=redis://redis:6379/0
```

For local host tools, use:

```text
REDIS_URL=redis://localhost:6379/0
```

## Run With Docker Compose

From the repository root:

```bash
docker compose up --build
```

After startup, verify the backend:

```text
http://localhost:8000/health
```

Open Swagger/OpenAPI docs:

```text
http://localhost:8000/swagger/
```

## Apply Database Schema

Automatic migrations are not configured in Sprint 1. Apply the PostgreSQL schema manually after the database container is running:

```bash
psql postgresql://gitflame:gitflame@localhost:5432/gitflame_codepilot -f backend/db/schema.sql
```

Optional storage verification:

```bash
psql postgresql://gitflame:gitflame@localhost:5432/gitflame_codepilot -f backend/db/verification.sql
```

## Sprint 1 Notes

- The Git workflow is implemented as a mock service interface.
- The mock Git workflow returns a branch name, pull request URL, reviewer, and provider.
- The `.yml` config service validates Sprint 1 branch rules, include/exclude patterns, approval commands, and reviewer policy.

## Sprint 2 Notes

- Redis is available as the initial broker for queued SERGE Agent Engine tasks.
- The Compose service name is `redis`, so containers should use `redis://redis:6379/0`.
- Local development tools can use `redis://localhost:6379/0`.
- Redis data is stored in the `redis_data` Docker volume.

## Sprint 2 Version 2 Deployment

Clone the current organization repository:

```bash
git clone https://github.com/GitFlameAI/Capstone-Project-GitFlame-CodePilot.git
cd Capstone-Project-GitFlame-CodePilot
```

Version 2 keeps the base Compose file and adds queue-based Agent Engine services through an override file. Use both files on every lifecycle command:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  up -d --build
```

Version 2 adds:

```text
redis         Redis Streams broker
agent-worker  Sequential Go worker with concurrency 1
agent-engine  SERGE-based issue-to-plan service
```

Copy the root environment template before startup:

```bash
cp .env.example .env
```

At minimum, verify these values in `.env`:

```text
OPENAI_BASE_URL=http://host.docker.internal:9000/v1
AGENT_MODEL=Qwen/Qwen3-Coder-30B-A3B-Instruct
OPENAI_API_KEY=
TASK_DISPATCH_MODE=redis
```

`OPENAI_BASE_URL` must point to a running OpenAI-compatible model server. The Sprint 2 override maps `host.docker.internal` to the Docker host gateway for Linux and VM deployments.

## Version 2 Verification

Check container states:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  ps
```

Health endpoints:

```text
Frontend:          http://localhost/
Backend health:    http://localhost:8000/health
Backend readiness: http://localhost:8000/ready
ML service:        http://localhost:8001/health
Agent Engine:      http://localhost:8002/health
Agent readiness:   http://localhost:8002/ready
```

Check Redis:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  exec redis redis-cli ping
```

Expected response:

```text
PONG
```

View Version 2 logs:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  logs -f backend agent-worker agent-engine redis database
```

Stop Version 2:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  down
```

## Updating Version 2 On The VM

From the project directory on the VM:

```bash
git checkout main
git pull origin main
cp -n .env.example .env
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  up -d --build
```

Verify after restart:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  ps
curl http://localhost/
curl http://localhost:8000/health
curl http://localhost:8000/ready
curl http://localhost:8001/health
curl http://localhost:8002/health
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  exec redis redis-cli ping
```

Before starting Version 2, edit `.env` and set a reachable `OPENAI_BASE_URL`. Add `OPENAI_API_KEY` only when the selected model provider requires it.

## Sprint 3 Version 3 Deployment

Sprint 3 deployment, smoke-test, and demo instructions are documented in:

```text
infra/sprint3-deployment.md
```

Version 3 keeps the same Compose entrypoint and extends the Agent Engine flow with approved-plan-to-code-generation:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  up -d --build
```

The external model or vLLM server requirements are documented in:

```text
infra/vllm-server-requirements.md
```

