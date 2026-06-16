# Sprint 1 Backend Deliverables

## Implemented Backend Features

The Sprint 1 backend skeleton is implemented as a Go service using the standard `net/http` package. It exposes a runnable `GET /health` endpoint, an OpenAPI JSON contract, GitFlame issue workflow contracts, a mock ML-service client integration, in-memory Sprint 1 storage, `.yml` config validation, and a mock Git workflow service interface for branch, PR URL, and reviewer assignment.

Implemented API surface:

- `GET /health`
- `POST /integrations/gitflame/issues/analyze`
- `GET /ai/issues/{id}/plan`
- `POST /ai/issues/{id}/approve`
- `POST /ai/issues/{id}/correct`
- `POST /ai/issues/{id}/reject`
- `POST /integrations/gitflame/repositories/{id}/recommendations/analyze`
- `GET /repositories/{id}/recommendations/status`
- `GET /repositories/{id}/recommendations/summary`
- `GET /repositories/{id}/recommendations`
- `PATCH /recommendations/{id}/close`
- `DELETE /recommendations/{id}`

## Verification

Local health check response:

```json
{"status":"ok","service":"backend"}
```

Swagger/OpenAPI is available after running the backend:

```text
http://localhost:8000/swagger/
```

Docker Compose verification completed successfully:

```bash
docker compose build backend
docker compose build ml-service
docker compose up -d
```

The approve endpoint returned a Sprint 1 mock Git workflow payload with branch name, pull request URL, reviewer, and provider.

## Run Command

```bash
go run ./cmd/server
```

## Infrastructure Notes

Docker Compose starts the backend, mock ML service, and PostgreSQL database:

```bash
docker compose up --build
```

The backend receives the database connection string through `DATABASE_URL`:

```text
postgresql://gitflame:gitflame@database:5432/gitflame_codepilot
```

The PostgreSQL schema is available at:

```text
backend/db/schema.sql
```

Sprint 1 infrastructure instructions are documented in:

```text
infra/README.md
```

## Missing Links

PR/Issue links are not available yet because the backend implementation has not been published to GitHub from this local workspace.
