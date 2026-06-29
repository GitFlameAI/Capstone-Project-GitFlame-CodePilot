# Sprint 3 Version 3 Deployment Notes

Sprint 3 extends Version 2 from issue-to-plan generation to approved-plan-to-code-generation. The deployment stack still uses the base Compose file plus the backend override file, but the Agent Engine now also exposes `POST /v1/files/generate` and requires a reachable OpenAI-compatible model endpoint for real code generation.

## Required Runtime Inputs

Copy the environment template before startup:

```bash
cp .env.example .env
```

Set the primary model endpoint:

```text
AGENT_MODEL=Qwen/Qwen3-Coder-30B-A3B-Instruct
OPENAI_BASE_URL=https://router.huggingface.co/v1
OPENAI_API_KEY=
```

Set fallback model values only when a second model endpoint is available:

```text
AGENT_FALLBACK_MODEL=
FALLBACK_OPENAI_BASE_URL=
FALLBACK_OPENAI_API_KEY=
```

For a self-hosted vLLM deployment, the model endpoint must expose an OpenAI-compatible API. The expected primary and fallback base URLs are:

```text
http://<server-ip>:9000/v1
http://<server-ip>:9001/v1
```

The model server requirements are documented in:

```text
infra/vllm-server-requirements.md
```

## One-Command Startup

Run Version 3 with the base Compose file and the backend override:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  up -d --build
```

The override adds or configures:

```text
redis          Redis Streams broker
agent-worker   Go worker for plan and code-generation tasks
agent-engine   SERGE-based Agent Engine with /v1/plans/generate and /v1/files/generate
```

## Health And Readiness Checks

Check container states:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  ps
```

Check service health:

```bash
curl http://localhost/
curl http://localhost:8000/health
curl http://localhost:8000/ready
curl http://localhost:8002/health
curl http://localhost:8002/ready
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

Check the external model endpoint directly:

```bash
curl "$OPENAI_BASE_URL/models"
```

The Agent Engine readiness endpoint also checks model availability. It returns `ready` only when one configured model is listed by the OpenAI-compatible model server.

## Smoke-Test Checklist

1. Validate the merged Compose configuration:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  config --quiet
```

2. Start the stack with the one-command startup command.
3. Confirm `docker compose ps` shows the project services running.
4. Save the `docker compose ps` output or screenshot for the report.
5. Confirm backend health and readiness:

```bash
curl http://localhost:8000/health
curl http://localhost:8000/ready
```

6. Confirm Agent Engine health and model readiness:

```bash
curl http://localhost:8002/health
curl http://localhost:8002/ready
```

7. Confirm the external model endpoint:

```bash
curl "$OPENAI_BASE_URL/models"
```

8. Run the product flow:

```text
YAML configuration
issue analysis
plan generation
approve plan
code-generation task
generated-files contract
```

9. Verify the generated-files result through the backend code-generation status endpoint:

```text
GET /ai/issues/{issueId}/code-generation
```

## Demo Flow

Use this order for a real-model Sprint 3 demo:

1. Show `.env` values for `AGENT_MODEL`, `OPENAI_BASE_URL`, and optional fallback variables without exposing secrets.
2. Start the stack with Docker Compose.
3. Show `docker compose ps`.
4. Show backend `/health` and `/ready` responses.
5. Show Agent Engine `/health` and `/ready` responses.
6. Create or reuse a saved YAML configuration.
7. Run issue-to-plan generation.
8. Approve the generated or user-edited `plan.md`.
9. Poll `GET /ai/issues/{issueId}/code-generation`.
10. Show the generated-files contract returned by the backend.

## Report Evidence To Capture

```text
docker compose ps output or screenshot
curl http://localhost:8000/health
curl http://localhost:8000/ready
curl http://localhost:8002/health
curl http://localhost:8002/ready
curl "$OPENAI_BASE_URL/models"
infrastructure PR URL
deployment URL or VM URL after verification
```
