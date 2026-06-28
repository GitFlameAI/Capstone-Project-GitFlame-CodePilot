# Manual Evaluation Issues

Use this file to evaluate model outputs. Each issue includes the exact issue text, attached repository context files, expected relevant files, and required points.

| Issue | Title | Issue text | Attached files | Expected relevant files | Required points |
| --- | --- | --- | --- | --- | --- |
| issue_01 | Persist issue-to-plan sessions in PostgreSQL | Issue sessions and generated plans are lost whenever the backend restarts. Replace the in-memory persistence for the issue-to-plan workflow with PostgreSQL while preserving current API behavior. Store plan revisions and user approve, correct, and reject actions. Existing tests must continue to pass. | `backend/internal/app/storage.go`<br>`backend/internal/app/server.go`<br>`backend/internal/app/models.go`<br>`backend/db/schema.sql` | `backend/internal/app/storage.go`<br>`backend/internal/app/server.go`<br>`backend/db/schema.sql` | Introduce a storage interface so handlers are not coupled to MemoryStore<br>Persist issue sessions, generated plan revisions, and user responses<br>Use transactions for state changes that write multiple records<br>Keep MemoryStore available for isolated tests or replace its test usage<br>Add restart-persistence and invalid-transition tests |
| issue_02 | Make ML client resilient to transient failures | The backend currently fails immediately when the ML service is slow or temporarily unavailable. Add configurable request timeout and bounded retries for connection errors, HTTP 429, and HTTP 5xx responses. Do not retry validation errors or cancelled requests. Return useful errors without exposing sensitive response bodies. | `backend/internal/app/services.go`<br>`backend/internal/app/config.go`<br>`backend/internal/app/server.go` | `backend/internal/app/services.go`<br>`backend/internal/app/config.go` | Add timeout and retry settings to backend configuration<br>Use bounded exponential backoff<br>Respect context cancellation and request deadlines<br>Retry only transient network, 429, and 5xx failures<br>Add deterministic tests using an HTTP test server |
| issue_03 | Add snippet limits to the repository AI configuration | Extend the repository .yml configuration with RAG limits for maximum retrieved files and maximum snippets per file. Validate positive bounds and expose the parsed values to the issue analysis flow. The user must not choose a model or plan format, and allowed_actions must not be part of the public configuration. | `backend/internal/app/config_service.go`<br>`backend/internal/app/models.go`<br>`backend/models/ai_config.go`<br>`docs/config/ai_config.example.yml` | `backend/internal/app/config_service.go`<br>`backend/internal/app/models.go`<br>`docs/config/ai_config.example.yml` | Add max_files and snippets_per_file fields under rag<br>Validate both limits with explicit positive upper bounds<br>Remove allowed_actions from the public YAML contract<br>Keep command behavior as server-side defaults rather than user configuration<br>Add parser and validation tests for valid, missing, zero, negative, and oversized values |
| issue_04 | Support asynchronous plan generation in the frontend | Plan generation may take several minutes once real models and the queue are enabled. Update the Work with AI flow so the UI can display queued and processing states, poll for the final plan, stop polling after completion or failure, and allow retry after recoverable errors. Avoid duplicate submissions when the user clicks Generate plan repeatedly. | `frontend/src/components/IssuePlanPanel.vue`<br>`frontend/src/api/client.js`<br>`frontend/src/api/index.js`<br>`frontend/src/api/mock.js` | `frontend/src/components/IssuePlanPanel.vue`<br>`frontend/src/api/client.js`<br>`frontend/src/api/mock.js` | Represent queued, processing, completed, and failed states<br>Poll a backend status endpoint with cleanup on unmount<br>Prevent duplicate submissions<br>Provide retry and actionable failure feedback<br>Update the mock API and add component tests for state transitions |
| issue_05 | Expire recommendation results using repository retention settings | Recommendation results must be retained for the number of days configured by the repository owner and then removed automatically. Add an explicit expiration timestamp, ensure expired results are not returned by read APIs, and provide a safe cleanup operation that can run repeatedly. Existing close and delete behavior must remain unchanged. | `backend/db/schema.sql`<br>`backend/internal/app/storage.go`<br>`backend/internal/app/server.go`<br>`backend/models/recommendation_run.go`<br>`backend/models/recommendation.go`<br>`docs/config/ai_config.example.yml` | `backend/db/schema.sql`<br>`backend/internal/app/storage.go`<br>`backend/internal/app/server.go`<br>`backend/models/recommendation_run.go` | Persist an expiration timestamp derived from recommendation_ttl_days<br>Exclude expired runs from status, summary, and recommendation list responses<br>Add an idempotent indexed cleanup query or worker operation<br>Preserve close and delete status history<br>Test expiry boundaries and repeated cleanup |

## issue_01: Persist issue-to-plan sessions in PostgreSQL

**Issue text:**

Issue sessions and generated plans are lost whenever the backend restarts. Replace the in-memory persistence for the issue-to-plan workflow with PostgreSQL while preserving current API behavior. Store plan revisions and user approve, correct, and reject actions. Existing tests must continue to pass.

**Attached files:**
- `backend/internal/app/storage.go`
- `backend/internal/app/server.go`
- `backend/internal/app/models.go`
- `backend/db/schema.sql`

**Expected relevant files:**
- `backend/internal/app/storage.go`
- `backend/internal/app/server.go`
- `backend/db/schema.sql`

**Required points:**
- Introduce a storage interface so handlers are not coupled to MemoryStore
- Persist issue sessions, generated plan revisions, and user responses
- Use transactions for state changes that write multiple records
- Keep MemoryStore available for isolated tests or replace its test usage
- Add restart-persistence and invalid-transition tests

## issue_02: Make ML client resilient to transient failures

**Issue text:**

The backend currently fails immediately when the ML service is slow or temporarily unavailable. Add configurable request timeout and bounded retries for connection errors, HTTP 429, and HTTP 5xx responses. Do not retry validation errors or cancelled requests. Return useful errors without exposing sensitive response bodies.

**Attached files:**
- `backend/internal/app/services.go`
- `backend/internal/app/config.go`
- `backend/internal/app/server.go`

**Expected relevant files:**
- `backend/internal/app/services.go`
- `backend/internal/app/config.go`

**Required points:**
- Add timeout and retry settings to backend configuration
- Use bounded exponential backoff
- Respect context cancellation and request deadlines
- Retry only transient network, 429, and 5xx failures
- Add deterministic tests using an HTTP test server

## issue_03: Add snippet limits to the repository AI configuration

**Issue text:**

Extend the repository .yml configuration with RAG limits for maximum retrieved files and maximum snippets per file. Validate positive bounds and expose the parsed values to the issue analysis flow. The user must not choose a model or plan format, and allowed_actions must not be part of the public configuration.

**Attached files:**
- `backend/internal/app/config_service.go`
- `backend/internal/app/models.go`
- `backend/models/ai_config.go`
- `docs/config/ai_config.example.yml`

**Expected relevant files:**
- `backend/internal/app/config_service.go`
- `backend/internal/app/models.go`
- `docs/config/ai_config.example.yml`

**Required points:**
- Add max_files and snippets_per_file fields under rag
- Validate both limits with explicit positive upper bounds
- Remove allowed_actions from the public YAML contract
- Keep command behavior as server-side defaults rather than user configuration
- Add parser and validation tests for valid, missing, zero, negative, and oversized values

## issue_04: Support asynchronous plan generation in the frontend

**Issue text:**

Plan generation may take several minutes once real models and the queue are enabled. Update the Work with AI flow so the UI can display queued and processing states, poll for the final plan, stop polling after completion or failure, and allow retry after recoverable errors. Avoid duplicate submissions when the user clicks Generate plan repeatedly.

**Attached files:**
- `frontend/src/components/IssuePlanPanel.vue`
- `frontend/src/api/client.js`
- `frontend/src/api/index.js`
- `frontend/src/api/mock.js`

**Expected relevant files:**
- `frontend/src/components/IssuePlanPanel.vue`
- `frontend/src/api/client.js`
- `frontend/src/api/mock.js`

**Required points:**
- Represent queued, processing, completed, and failed states
- Poll a backend status endpoint with cleanup on unmount
- Prevent duplicate submissions
- Provide retry and actionable failure feedback
- Update the mock API and add component tests for state transitions

## issue_05: Expire recommendation results using repository retention settings

**Issue text:**

Recommendation results must be retained for the number of days configured by the repository owner and then removed automatically. Add an explicit expiration timestamp, ensure expired results are not returned by read APIs, and provide a safe cleanup operation that can run repeatedly. Existing close and delete behavior must remain unchanged.

**Attached files:**
- `backend/db/schema.sql`
- `backend/internal/app/storage.go`
- `backend/internal/app/server.go`
- `backend/models/recommendation_run.go`
- `backend/models/recommendation.go`
- `docs/config/ai_config.example.yml`

**Expected relevant files:**
- `backend/db/schema.sql`
- `backend/internal/app/storage.go`
- `backend/internal/app/server.go`
- `backend/models/recommendation_run.go`

**Required points:**
- Persist an expiration timestamp derived from recommendation_ttl_days
- Exclude expired runs from status, summary, and recommendation list responses
- Add an idempotent indexed cleanup query or worker operation
- Preserve close and delete status history
- Test expiry boundaries and repeated cleanup
