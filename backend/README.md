# Backend

The backend service exposes GitFlame integration endpoints, validates repository configuration, stores workflow state, and communicates with the ML service.

Current Sprint 2 Go backend includes:

- `GET /health`
- OpenAPI contract at `GET /openapi.json`
- Swagger UI at `GET /swagger/` and `GET /docs`
- issue workflow endpoints:
  - `POST /integrations/gitflame/issues/analyze`
  - `GET /ai/issues/{id}/plan`
  - `POST /ai/issues/{id}/approve`
  - `POST /ai/issues/{id}/correct`
  - `POST /ai/issues/{id}/reject`
- recommendation endpoints:
  - `POST /integrations/gitflame/repositories/{id}/recommendations/analyze`
  - `GET /repositories/{id}/recommendations/status`
  - `GET /repositories/{id}/recommendations/summary`
  - `GET /repositories/{id}/recommendations`
  - `PATCH /recommendations/{id}/close`
  - `DELETE /recommendations/{id}`
- PostgreSQL storage for issue sessions, generated plans, plan revisions, agent task states, and recommendation cards
- ML service client with local fallback when the mock ML service is unavailable
- `.yml` config service for branch prefix, include/exclude patterns, approval commands, and reviewer policy
- mock Git workflow service interface with branch, PR URL, reviewer, and provider response

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
docker compose up --build
```

The frontend is exposed on port `80`, and the backend remains available on port `8000`:

```text
http://localhost/
http://localhost:8000/health
```

The backend receives:

```text
ML_SERVICE_URL=http://ml-service:8001
DATABASE_URL=postgresql://gitflame:gitflame@database:5432/gitflame_codepilot
```

PostgreSQL schema can be applied manually:

```bash
psql postgresql://gitflame:gitflame@localhost:5432/gitflame_codepilot -f backend/db/migrations/initial_schema.sql
```

## Build

```bash
go build ./cmd/server
```
