# Backend

The backend service exposes GitFlame integration endpoints, validates repository configuration, stores workflow state, and communicates with the ML service.

Current Sprint 1 Go backend includes:

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
- mock Sprint 1 storage for issue sessions, plans, and recommendation cards
- ML service client with local fallback when the mock ML service is unavailable
- mock Git workflow response with branch, PR URL, and reviewer

## Run locally

```bash
go run ./cmd/server
```

Open API docs:

```text
http://localhost:8000/swagger/
```

## Build

```bash
go build ./cmd/server
```
