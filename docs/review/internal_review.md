# Internal Review — Sprint 2 (Version 2)

Reviewer: Roman (frontend) · Date: Sprint 2 / Week 3
Scope: integration review across the Sprint 2 branches with a focus on the
frontend ↔ backend contract for the async issue → plan flow and recommendations.

This review records verified scenarios, findings (with severity), and follow-up
issues to open on the board.

## 1. Verified scenarios (frontend)

Verified in mock mode (offline) and against the documented backend contract. Live
full-stack verification is pending the VM deployment (Redis + Agent Engine + GPU).

| # | Scenario | Result |
| --- | --- | --- |
| S1 | Submit issue → task `queued → processing → completed` → plan shown | PASS |
| S2 | Approve → `generated_files_contract` (branch/commit/PR/reviewer) shown | PASS |
| S3 | Request correction (feedback) → new task polled → revised plan | PASS |
| S4 | Reject → rejected result | PASS |
| S5 | Recoverable Agent Engine failure → Retry → success | PASS |
| S6 | Client-side timeout → "keep waiting" resumes polling | PASS |
| S7 | Validation errors (missing title/yaml/context) → 422 surfaced on form | PASS |
| S8 | Recommendations: load, mark resolved, dismiss | PASS |
| S9 | Recommendations: empty state when no report exists (404) | PASS |

## 2. Findings

### F1 — Recommendation card contract mismatch (severity: medium)
`backend/internal/domain/domain.go::RecommendationCard` exposes
`{id, severity, file, line, problem, suggestion, confidence, state}` but **no
`category`**, while the project spec and the ML `recommendation_schema.json` include
`category`. The frontend tolerates the missing field, but the contracts diverge.
**Recommendation:** add `category` to the backend card and the OpenAPI schema so the
detailed report can group by category.
**Follow-up issue:** "Align backend RecommendationCard with ML recommendation_schema (add category)".

### F2 — Recommendations handler uses a hardcoded local fallback (severity: high)
`analyzeRecommendations` in `server.go` returns a single static card and a fixed
summary; it is **not yet wired to the recommendations ML service** the way the plan
flow is wired to the Agent Engine. The demo therefore shows placeholder data in live
mode.
**Recommendation:** wire backend → recommendations service (Karim) and persist real
cards, mirroring the agent-task pattern.
**Follow-up issue:** "Wire backend recommendations endpoint to the recommendations service".

### F3 — Strict request decoding is brittle (severity: low)
The backend decodes with `DisallowUnknownFields()`, so any extra field sent by the
frontend produces `400 invalid_json`. This keeps the contract tight but couples the
clients to the exact field set.
**Recommendation:** generate the frontend client from the OpenAPI spec, or relax to
tolerant decoding for forward-compatibility.
**Follow-up issue:** "Generate typed API client from OpenAPI to prevent contract drift".

### F4 — `/ai/issues/{id}` id handling is inconsistent between stores (severity: low)
The two store implementations resolve `{id}` differently. The Postgres store looks up
`WHERE s.id::text=$1 OR s.external_issue_id=$1`, so it accepts **either** the
`session_id` or the `issue_id`. The in-memory store (`MemoryStore.Session`) only matches
the `session_id` (its map is keyed by `session.ID`), so passing an `issue_id` there
returns 404. The deployed stack uses Postgres, but unit/local runs on the memory store
behave differently — a latent inconsistency. The frontend sidesteps this entirely by
always using the `session_id` returned from `analyze`, which works in both stores.
**Recommendation:** make both stores agree — either accept `session_id` only on the public
path, or have the memory store resolve `issue_id` the same way Postgres does.
**Follow-up issue:** "Unify issue/session id resolution across memory and Postgres stores".

### F5 — No CORS, proxy-only access (severity: low / documentation)
The backend sets no CORS headers. This is fine behind the Vite dev proxy and nginx
(`/api`), but a direct cross-origin `VITE_API_BASE=http://host:8000` fails in the
browser. The frontend README documents the proxy approach.
**Follow-up issue:** "Document proxy-only access or add CORS to the backend".

### F6 — Empty `generated_files_contract.files` (severity: info)
Expected for Sprint 2 (code generation lands in a later sprint). The UI shows only the
contract metadata and does not imply files exist. No action this sprint.

## 3. Follow-up issues to open

- [ ] Align backend `RecommendationCard` with the ML schema (add `category`). (F1)
- [ ] Wire backend recommendations endpoint to the recommendations service. (F2)
- [ ] Generate a typed frontend API client from OpenAPI. (F3)
- [ ] Unify issue/session id resolution across memory and Postgres stores. (F4)
- [ ] Document proxy-only access or add CORS. (F5)

## 4. Notes for the integration merge

- Frontend talks only to the Go backend; no direct Agent Engine calls. Confirmed.
- Frontend builds cleanly in both mock and `VITE_API_BASE=/api` modes.
- Docker build bakes `VITE_API_BASE=/api`; nginx proxies `/api/ → backend:8000`.
- Recommended merge order keeps this branch last, after backend/db/redis/agent are in.
