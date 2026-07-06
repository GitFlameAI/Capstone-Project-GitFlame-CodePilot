# Sprint 4 Demo Runbook

Sprint 4 demo deployment runs the complete GitFlame CodePilot stand: frontend, backend, PostgreSQL, Redis, Agent Engine, Agent Worker, ML service, and the external recommendation service.

## Prerequisites

- Docker with Compose v2.
- Access to the university network or VPN when the Laguna model endpoint is not publicly reachable.
- Runtime secret for `OPENAI_API_KEY` if the model endpoint requires authentication.
- Optional GitFlame API credentials for the apply-to-GitFlame flow.

## Environment

Create a local `.env` file from the template:

```bash
cp .env.example .env
```

Important Sprint 4 variables:

```text
AGENT_MODEL=laguna
OPENAI_BASE_URL=https://gpu-1.devops-playground.innopolis.university/v1
OPENAI_API_KEY=
MODEL_CONTEXT_LIMIT=32768
GITFLAME_BASE_URL=
GITFLAME_API_KEY=
GITFLAME_TIMEOUT_SECONDS=30
RECOMMENDATION_SERVICE_URL=http://localhost:8003
RECOMMENDATION_SERVICE_TIMEOUT_SECONDS=120
RECOMMENDATION_MODEL=qwen2.5-coder:1.5b
```

Do not commit real API keys. The backend container receives the internal recommendation URL `http://recommendation-service:7860` from the Compose override.

## One-Command Startup

From the repository root:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  up -d --build
```

The recommendation service starts Ollama inside its container and pulls `RECOMMENDATION_MODEL` on first startup. The first run can therefore take longer than the other services.

## Service Checks

Check all containers:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  ps
```

Expected demo services:

```text
frontend
backend
database
redis
agent-engine
agent-worker
ml-service
recommendation-service
```

Check HTTP health and readiness:

```bash
curl http://localhost/
curl http://localhost:8000/health
curl http://localhost:8000/ready
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

Expected response:

```text
PONG
```

Check the OpenAI-compatible model endpoint when needed:

```bash
curl "$OPENAI_BASE_URL/models"
```

## Demo Flow

1. Start the stack with Docker Compose.
2. Confirm `docker compose ps` shows healthy backend, database, Redis, Agent Engine, Agent Worker, and recommendation service.
3. Open the frontend at `http://localhost/`.
4. Save or load `.ai.yml` configuration.
5. Run issue analysis and poll the task until `plan.md` is ready.
6. Edit or approve the plan.
7. Poll code generation and show the generated files contract.
8. If GitFlame credentials are configured, call the apply endpoint and show branch / commit / PR metadata.
9. Run recommendation analysis and show stored recommendation cards.

## Troubleshooting

If backend readiness is `not_ready`, check:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  logs backend agent-engine recommendation-service database redis
```

Common causes:

- `OPENAI_API_KEY` is missing for the model endpoint.
- The university model endpoint is unreachable without VPN.
- The recommendation service is still pulling the Ollama model.
- Existing PostgreSQL volume was created before new migrations; recreate the volume only if local data can be discarded.

## Report Evidence

Capture these outputs for the weekly report:

```text
docker compose ps
curl http://localhost:8000/ready
curl http://localhost:8002/ready
curl http://localhost:8003/ready
GitHub Actions CI result
VM or demo stand URL
```
