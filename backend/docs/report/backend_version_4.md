# Backend Version 4 - Sprint 4

## Implemented Improvements And Bug Fixes

| Area | Backend change | Verification link |
| --- | --- | --- |
| Frontend flow contract | Existing analyze, polling, plan, approve, correct, reject, and code-generation endpoints remain available for the real frontend flow. | `internal/httpapi/server.go`, `internal/httpapi/openapi.json` |
| Edited plan approval | `POST /ai/issues/{id}/approve` accepts optional `plan_markdown`, validates the edited plan, persists a new revision, and sends that exact markdown to code generation. | `internal/service/workflow.go`, `internal/httpapi/integration_test.go` |
| GitFlame webhook | `POST /integrations/gitflame/webhooks/issues` accepts issue events and creates the AI session/task. If the webhook does not include config/files, the backend uses the GitFlame API client. | `internal/httpapi/integrations.go` |
| GitFlame API client | Backend can fetch `.ai.yml`, repository tree, and file contents, then applies `.ai.yml` include/exclude/max file rules before creating the analysis payload. | `internal/httpapi/gitflame_client.go` |
| Apply result to GitFlame | `POST /ai/issues/{id}/gitflame/apply` creates/applies the generated-files branch, commit, and pull request through GitFlame API, then saves `commit_sha`, `pull_request_id`, `pull_request_url`, and apply status back into the generated files contract. | `internal/httpapi/server.go`, `internal/httpapi/gitflame_client.go` |
| Real recommendations flow | Recommendation analysis calls the external ML recommendation service through `/v1/recommendations/analyze`; generated cards are normalized, saved, and returned. No local fallback card is produced. | `internal/httpapi/recommendation_client.go`, `internal/httpapi/server.go` |
| Recommendation categories | Backend preserves ML recommendation `category` values and stores them in PostgreSQL/in-memory reports. | `internal/domain/domain.go`, `internal/repository/postgres.go` |

## API Documentation

- Swagger UI: `GET /swagger/`
- OpenAPI JSON: `GET /openapi.json`
- GitFlame webhook endpoint: `POST /integrations/gitflame/webhooks/issues`
- Edited approval endpoint: `POST /ai/issues/{id}/approve`
- Apply generated files to GitFlame: `POST /ai/issues/{id}/gitflame/apply`
- Recommendation service-backed endpoint: `POST /integrations/gitflame/repositories/{id}/recommendations/analyze`

## Runtime Configuration

New Sprint 4 backend environment variables:

```text
GITFLAME_BASE_URL=https://gitflametest.ru
GITFLAME_API_KEY=
GITFLAME_TIMEOUT_SECONDS=30
GITFLAME_CREDENTIAL_KEY=12345678901234567890123456789012
GITFLAME_CREDENTIAL_KEY_VERSION=1
RECOMMENDATION_SERVICE_URL=http://localhost:8003
RECOMMENDATION_SERVICE_TIMEOUT_SECONDS=120
```

## Verification

```bash
env GOCACHE=/private/tmp/gitflame-go-build go test ./...
```
