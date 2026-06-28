# Test Issues for Manual Review

The model receives the issue plus the full contents of every listed attached file. Expected files and required points are hidden evaluation criteria and are not sent as instructions to the model.

## Issue 1: Persist issue-to-plan sessions in PostgreSQL

Issue sessions and generated plans are lost whenever the backend restarts. Replace the in-memory persistence for the issue-to-plan workflow with PostgreSQL while preserving current API behavior. Store plan revisions and user approve, correct, and reject actions. Existing tests must continue to pass.

Attached files:

- `backend/internal/app/storage.go`
- `backend/internal/app/server.go`
- `backend/internal/app/models.go`
- `backend/db/schema.sql`

## Issue 2: Make ML client resilient to transient failures

The backend currently fails immediately when the ML service is slow or temporarily unavailable. Add configurable request timeout and bounded retries for connection errors, HTTP 429, and HTTP 5xx responses. Do not retry validation errors or cancelled requests. Return useful errors without exposing sensitive response bodies.

Attached files:

- `backend/internal/app/services.go`
- `backend/internal/app/config.go`
- `backend/internal/app/server.go`

## Issue 3: Add snippet limits to the repository AI configuration

Extend the repository `.yml` configuration with RAG limits for maximum retrieved files and maximum snippets per file. Validate positive bounds and expose the parsed values to the issue analysis flow. The user must not choose a model or plan format, and `allowed_actions` must not be part of the public configuration.

Attached files:

- `backend/internal/app/config_service.go`
- `backend/internal/app/models.go`
- `backend/models/ai_config.go`
- `docs/config/ai_config.example.yml`

## Issue 4: Support asynchronous plan generation in the frontend

Plan generation may take several minutes once real models and the queue are enabled. Update the Work with AI flow so the UI can display queued and processing states, poll for the final plan, stop polling after completion or failure, and allow retry after recoverable errors. Avoid duplicate submissions when the user clicks Generate plan repeatedly.

Attached files:

- `frontend/src/components/IssuePlanPanel.vue`
- `frontend/src/api/client.js`
- `frontend/src/api/index.js`
- `frontend/src/api/mock.js`

## Issue 5: Expire recommendation results using repository retention settings

Recommendation results must be retained for the number of days configured by the repository owner and then removed automatically. Add an explicit expiration timestamp, ensure expired results are not returned by read APIs, and provide a safe cleanup operation that can run repeatedly. Existing close and delete behavior must remain unchanged.

Attached files:

- `backend/db/schema.sql`
- `backend/internal/app/storage.go`
- `backend/internal/app/server.go`
- `backend/models/recommendation_run.go`
- `backend/models/recommendation.go`
- `docs/config/ai_config.example.yml`

