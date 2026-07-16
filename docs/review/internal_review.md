# Internal Review

Per-sprint integration reviews. The **Sprint 5** review is current; the **Sprint 4**,
**Sprint 3** and **Sprint 2** reviews are kept below for history.

---

# Internal Review — Sprint 5 (Version 5)

Reviewer: Roman (frontend) · Date: Sprint 5 / Week 6
Scope: connecting the frontend to the real Go backend end-to-end (secure GitFlame
connection flow, apply-to-GitFlame, error states), reviewed against the Sprint 5 backend
branches (`sprint-5/arthur-backend-fix`, `sprint-5/amir-db-storage`) and their OpenAPI.
Frontend flows verified in **mock mode** (no backend required); the mock mirrors the new
endpoints, and the mock flow is covered by a runtime smoke test.

## 1. Verified scenarios (frontend)

| # | Scenario | Result |
| --- | --- | --- |
| S1 | Connect: token sent once to `POST /connections`, only metadata stored, no token in storage/memory | PASS |
| S2 | Invalid / expired token → friendly "Access token problem" + field highlight | PASS |
| S3 | Save `.ai.yml` → Autogeneration & Recommendations unlock | PASS |
| S4 | Issue → plan → **edit** → approve forwards the edited `plan_markdown` | PASS |
| S5 | Approve → code-generation task → generated file operations listed | PASS |
| S6 | **Apply to GitFlame** → commit SHA + real PR URL; files marked *applied* | PASS |
| S7 | Apply idempotent; apply failure recoverable (default branch untouched) | PASS |
| S8 | Request correction / Reject still work | PASS |
| S9 | Recommendations analyze sends `repository_context`; grid + Create issue | PASS |
| S10 | Refresh keeps the workspace via the HttpOnly session cookie (no re-entry) | PASS |
| S11 | Reconnect gate on 401/403 → `PUT /connections/{id}` restores the session | PASS |
| S12 | Disconnect → revoke + logout → back to connect screen | PASS |
| S13 | Backend unreachable / GitFlame down / Agent busy → distinct messages | PASS |

## 2. Contract alignment (checked against the backend branches)

- Routes, cookie name (`codepilot_session`) and the `GitFlameConnection(Request)` schemas
  match Arthur's `openapi.json` and handlers. Connection request needs only
  `access_token` (+ `repo_url` when `repository.id` is omitted); the response
  `token_status` enum (`active|invalid|expired|revoked|reauth_required`) is handled.
- `POST /ai/issues/{id}/approve` accepts `{ plan_markdown }`; `POST .../gitflame/apply`
  returns the contract with `apply_status`, `commit_sha`, `pull_request_url`.
- Every authenticated call sends `credentials:'include'`; same-origin nginx proxy means
  no extra CORS. On HTTP the backend runs `SESSION_COOKIE_SECURE=false` (VM default).

## 3. Follow-up issues to open

- [ ] Backend: add `GET /integrations/gitflame/connections` so a fresh browser tab can
      restore the connection list (currently metadata lives only in `sessionStorage`).
- [ ] Backend: have the direct `.../recommendations/analyze` fetch repository context via
      the connection (like the webhook path), so the frontend need not send
      `repository_context`.
- [ ] Frontend: generate a typed API client from OpenAPI (carried over from Sprint 4).
- [ ] Frontend: add component tests to CI (carried over).

## 4. Notes for the integration merge

- Frontend talks only to the Go backend; no direct Agent Engine calls. Confirmed.
- Frontend builds cleanly in both mock and `VITE_API_BASE=/api` modes (79 modules).
- Docker build bakes `VITE_API_BASE=/api`; nginx proxies `/api/ → backend:8000`.
- Recommended merge order keeps this branch **last**, after the backend/db/agent branches.

---

# Internal Review — Sprint 4 (Version 4)

Reviewer: Roman (frontend) · Date: Sprint 4 / Week 5
Scope: the Sprint 4 usability changes and the alignment with the new backend GitFlame
integration endpoints, reviewed across the Sprint 4 branches. All scenarios verified in
**mock mode** (no backend required).

## 1. Verified scenarios (frontend, mock mode)

| # | Scenario | Result |
| --- | --- | --- |
| S1 | Roadmap: two tracks behind a **sliding toggle** (animated), hover outline on the inactive tab | PASS |
| S2 | Roadmap: steps auto-advance; the **progress bar and the step change stay in sync** (no late jumps after hover or manual clicks) | PASS |
| S3 | Roadmap: **play/pause** control stops/starts auto-advance; hovering still pauses; resumes correctly | PASS |
| S4 | Roadmap: reaching a track's last step **auto-switches** to the other track | PASS |
| S5 | Landing preview: **Preview renders Markdown** (not raw code); **Request correction** takes typed input and produces a revision; **Reject** is red | PASS |
| S6 | Landing: single consent; empty → red underline blocks Continue; **Continue is dimmed until the form is complete** but still clickable; **Default branch is empty**; `external` icon by "GitFlame" | PASS |
| S7 | Routing: **"Start with" is gone**; the workspace always opens on **Config** | PASS |
| S8 | Repository: Exclude a file → excluded row shows **Include**; a fully-excluded folder collapses to `folder/**`; `.ai.yml` cannot be excluded; folders start collapsed | PASS |
| S9 | Config draft: edit categories/excludes, switch tabs and back → edits persist; `.ai.yml` and the tab unlock change only on **Save** | PASS |
| S10 | Config: **Discard changes** reverts the draft to the saved config; the button appears only when dirty | PASS |
| S11 | Config: save hint matches state (unlock wording only before first save); the **first save shows a green "Go to Autogeneration" banner** | PASS |
| S12 | Config **Exclude paths**: typing/adding a very long token no longer stretches the page — chips ellipsise, the hint wraps | PASS |
| S13 | Recommendations: first open auto-runs; dismissing the last card re-runs on the next visit, not immediately | PASS |
| S14 | Refresh on the workspace: stays in the workspace and shows a **mandatory token gate in mock mode** (token never persisted) | PASS |
| S15 | Locked tab hint: **Config** is a link to the tab; the plate shrinks to its content | PASS |
| S16 | Responsive: at high zoom / narrow widths the roadmap toggle **stacks** and no label clips; no right-overflow elsewhere | PASS |

## 2. Findings and follow-ups

### F1 — Browser push for live updates is a backend follow-up (medium)
The webhook is GitFlame → backend. Pushing updates to an open browser needs backend SSE or
polling (e.g. `GET .../events`), out of scope this sprint. The frontend demonstrates the UX with
a mock push (`applyMockPush`); wiring it to real events is tracked for a later sprint.

### F2 — Expired-token detection is live-mode only (low, honest limitation)
The token gate's *missing* state is exercised on every refresh (mock and live). The
*invalid/expired* state is triggered by a backend 401/403, so it only appears against the real
backend; mock has no real auth.

### F3 — Config is applied on Save, not on tree toggle (by design)
Excluding files in the Repository tree edits the **draft** only; the `.ai.yml` changes on Save in
the Config tab. The Repository tab surfaces an "Unsaved · review in Config" hint, and Config now
has a **Discard changes** button so the draft can be reverted.

### F4 — Roadmap timing relies on wall-clock pause/resume (low)
The progress bar (CSS) and the advance (JS timer) are kept in sync by pausing/resuming both on the
same events and tracking remaining time. This is robust for hover/button pauses and manual step
selection; it does not attempt frame-accurate sync, which is unnecessary for a decorative bar.

---


# Internal Review — Sprint 3 (Version 3)

Reviewer: Roman (frontend) · Date: Sprint 3 / Week 4
Scope: the frontend ↔ backend contracts for the issue → plan → **code-generation** flow,
the **configuration** contract, and the recommendations path, reviewed across the Sprint 3
branches (`danil-codegen-contracts`, `arthur-backend`, `amir-db-storage`, `karim`,
`ruslan-deployment`).

## 1. Verified scenarios (frontend, mock mode)

| # | Scenario | Result |
| --- | --- | --- |
| S1 | GitFlame page → Work with AI → landing; eyebrow/top chip link to gitflame.ru | PASS |
| S2 | Connect form validation (empty token / unticked consent → red underline, blocked) | PASS |
| S3 | Service usage policy opens in a modal; Continue is centered | PASS |
| S4 | Repository tab: Connection/Files/Config stacked vertically; edit connection switches repo and re-locks AI tabs | PASS |
| S5 | Config tab: only the 4 contract fields; exclude paths as a chip picker; empty categories ⇒ "no recommendations"; Save unlocks tabs | PASS |
| S6 | Autogeneration: pick existing issue (auto-fills) or create new; no context field | PASS |
| S7 | Plan Edit/Preview; edited plan persists on approve | PASS |
| S8 | Approve → code-generation task polled → file ops `{action, path, description}`; Back to issues / Go to PR | PASS |
| S9 | Approve and Reject show independent spinners | PASS |
| S10 | Correction → new task polled → revised plan; Reject → rejected | PASS |
| S11 | Recoverable failure (`fail`/`timeout` titles) → Retry / keep-waiting | PASS |
| S12 | Recommendations grid → detail overlay with ←/→, delete, create issue (→ Autogeneration pre-filled) | PASS |
| S13 | Category filter (all on → all show; all off → none); no "resolved" state present | PASS |

## 2. Findings

### F9 — Configuration contract drift: spec vs backend parser (severity: high) — NEW
The authoritative Sprint 3 config spec (`docs/config/ai_config_spec.md`, danil branch) is
intentionally small: `repository.default_branch`, `analysis.enabled` + `analysis.exclude`,
`recommendations.enabled` + `recommendations.categories`, and
`storage.recommendation_ttl_days`. The frontend Config form now emits exactly this shape.
However the Go parser `ParseAIConfig` (`backend/internal/service/config.go`, arthur/amir
branches) still enforces the **older, larger** schema and will reject the new config:

- it requires `version: 1` (the new spec has no `version`);
- it requires a non-empty `analysis.include` (dropped from the spec);
- it requires a `code_generation` block with `require_user_approval: true` and
  `reviewer_policy: issue_author` (the whole section was dropped from the spec);
- it reads retention from `recommendations.retention_days`, **not** the spec's
  `storage.recommendation_ttl_days`, so the configured TTL would be ignored.

Impact: in live mode a config saved from the UI would be rejected (or its retention
ignored). Mock mode is unaffected (it does not strictly parse the YAML).
**Recommendation:** align the backend parser to the agreed spec — drop the `version`,
`analysis.include` and `code_generation` requirements, and read
`storage.recommendation_ttl_days`. The spec is the agreement, so the parser should follow
it. **Follow-up issue:** "Align ParseAIConfig with ai_config_spec.md (drop version/include/
code_generation; read storage.recommendation_ttl_days)".

### F7 — `SaveRecommendations` fails with SQLSTATE 42P08 (severity: high) — OPEN
The live recommendations save errors with `inconsistent types deduced for parameter $4`
because the retention placeholder is used as both an int column value and inside a text
interval expression. Mock mode is unaffected. A ready-to-apply fix is in
`docs/review/sql_42P08_fix_for_amir.md` (split into two typed parameters / use
`make_interval`). **Owner:** Amir.

### F1 — Recommendation card still has no `category` (severity: medium) — STILL OPEN
`domain.RecommendationCard` exposes no `category`, while the ML schema and the Sprint 3 UI
(card grid + category filter) rely on it. The mock supplies `category`; the live backend
card must add it. **Recommendation:** add `category` to the backend card + OpenAPI.

### F2 — Recommendations endpoint wiring (severity: high) — TRACK
Confirm the backend persists real ML cards (and that F7 unblocks the save). Until then live
mode shows placeholder/empty data while mock shows the seeded report.

### F8 — Code-generation polling has two valid sources (severity: info) — NOTED
After approve, files arrive either by polling `GET /ai/tasks/{taskId}` (the approve response
carries the code-generation `task_id`) or via `GET /ai/issues/{id}/code-generation`. The UI
polls the task id; `getCodeGeneration()` stays in the client for parity. No action.

### F-cleanup — Dead components removed (severity: info) — NEW
Superseded Sprint 2 components and the now-orphaned `RecommendationCard.vue` /
`SeverityBadge.vue` were removed (the new Recommendations tab renders its own cards).
No remaining imports reference them; `npm run build` is clean (70 modules).

> Sprint 2 findings F3 (OpenAPI client), F4 (id resolution), F5 (CORS / proxy-only) remain
> open and are tracked below; none block Sprint 3.

## 3. Follow-up issues to open

- [ ] **Align `ParseAIConfig` with `ai_config_spec.md`** (drop version/include/code_generation; read `storage.recommendation_ttl_days`). (F9)
- [ ] **Fix 42P08 in `SaveRecommendations`** (typed params / `make_interval`). (F7)
- [ ] Add `category` to the backend `RecommendationCard` + OpenAPI. (F1)
- [ ] Confirm backend recommendations endpoint persists real ML cards. (F2)
- [ ] Generate a typed frontend API client from OpenAPI. (F3, carried over)
- [ ] Unify issue/session id resolution across memory and Postgres stores. (F4, carried over)
- [ ] Document proxy-only access or add CORS. (F5, carried over)

## 4. Notes for the integration merge

- Frontend talks only to the Go backend; no direct Agent Engine calls. Confirmed.
- Frontend builds cleanly in both mock and `VITE_API_BASE=/api` modes.
- The Config form emits the agreed Sprint 3 contract; live use depends on F9 being resolved.
- Recommended merge order keeps this branch after backend/db/redis/agent; F7 and F9 should
  land with the backend/db branches.

---

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
