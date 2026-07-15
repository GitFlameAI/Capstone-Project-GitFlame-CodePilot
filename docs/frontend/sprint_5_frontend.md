# Sprint 5 — Frontend (Version 5)

Owner: Roman Titov · Branch: `sprint-5/roman-frontend`

Sprint 5 moves the frontend from a mock-first demo to a **real, end-to-end integration
with the Go backend**, and hardens every failure path. The frontend stays thin — it
talks only to the Go backend and never to the Agent Engine — and keeps a mock fallback
so the demo always runs even without the stack.

## 1. Secure GitFlame connection flow

The backend changed how the GitFlame access token is handled (Arthur/Amir): the token is
no longer a single global `GITFLAME_API_KEY`; it is a per-user/per-repository credential
stored **AES-GCM encrypted** on the backend, behind an **HttpOnly `codepilot_session`
cookie**. The frontend was updated to match:

- **The frontend never stores the GitFlame token.** It is entered once on the connect
  screen, sent once to `POST /integrations/gitflame/connections`, and then cleared from
  the form. There is no `localStorage`/`sessionStorage`/JS-cookie copy, and it is not
  kept in the reactive store either (Sprint 4 kept it in memory; Sprint 5 removes even
  that).
- **Every request uses `credentials:'include'`** (`src/api/client.js`) so the session
  cookie is carried on all authenticated calls (connection, apply, repository-scoped).
- **The store keeps only metadata** (`src/store/session.js`): `connection id`,
  `repository { id, name, default_branch, web_url }`, `repo_url`, `token_last4`,
  `token_status`. All subsequent calls use the **backend-issued `repository.id`**, not a
  value computed on the frontend.
- **Connection request body.** The frontend sends `{ access_token, repo_url }` and also a
  parsed `repository` object. The backend resolves the repository from `repo_url`
  (stripping a trailing `/code`, `/tree/...`, etc.); the parsed object is a compatibility
  fallback. `/code` is optional in the URL.

Lifecycle wired in the UI:

| Action | Endpoint | Where |
| --- | --- | --- |
| Connect | `POST /integrations/gitflame/connections` | Landing "Continue" |
| Reconnect / replace token | `PUT /integrations/gitflame/connections/{id}` | Repository → Change, reconnect gate |
| Disconnect | `DELETE /integrations/gitflame/connections/{id}` + `DELETE /auth/session` | Repository → Disconnect |

The `POST` also creates the server session if none exists, so connecting is a single
step from the user's point of view.

## 2. Full autogeneration flow with Apply to GitFlame

The Autogeneration tab now runs the complete contract:

```
issue → analyze → poll task → editable plan → approve(edited plan_markdown)
      → poll code-generation → generated file operations
      → Apply to GitFlame → branch + commit + pull request
```

- **Approve forwards the edited plan.** `POST /ai/issues/{id}/approve` now carries the
  `plan_markdown` the user reviewed, so code generation uses exactly that plan.
- **Apply step.** A new **Apply to GitFlame** action calls
  `POST /ai/issues/{id}/gitflame/apply`. On success the result panel shows the returned
  **commit SHA** and the **real pull-request URL**, each generated file is marked
  *applied*, and **Go to pull request** opens the real PR. Apply is idempotent
  (re-applying returns the same PR), and a failed apply is recoverable — nothing is
  written to the default branch, and the user can retry.

## 3. Explicit error states

A single mapping (`src/api/errors.js`) turns backend error codes / HTTP statuses into a
friendly `{ title, message, kind, retryable, tokenProblem }` descriptor, used by the
connect screen, the reconnect gate, the apply step and the recommendations tab:

| Situation | What the user sees |
| --- | --- |
| Invalid / expired / revoked token | "Access token problem" → reconnect prompt |
| GitFlame unreachable | "GitFlame is unavailable" → retry |
| Agent Engine busy / timeout | "The AI service is busy" → retry |
| Queue / database unavailable (503) | "Service temporarily unavailable" → retry |
| Apply failed | "Couldn't apply the changes to GitFlame" → retry (branch untouched) |
| Backend unreachable | "Can't reach CodePilot" → retry |
| Validation | the backend's own field-level detail |

`api/index.js` also wraps every live call: a token/connection failure (401/403 or a
`gitflame_*` connection code) flips `session.tokenStatus` to `invalid`, which raises the
reconnect gate anywhere in the workspace.

## 4. Refresh keeps you signed in

Because the session is an HttpOnly cookie, a page refresh no longer asks for the token —
the workspace is restored from a `sessionStorage` **metadata** snapshot (no token) and
the cookie keeps the calls authenticated. The **reconnect gate** appears only when the
backend actually reports the session/token is invalid. In mock mode a
"Simulate expired session (demo)" button on the Repository tab makes the gate
demonstrable without a backend.

## 5. Mock parity

The in-memory mock (`src/api/mock.js`) mirrors the new endpoints so the whole Sprint 5
flow is demoable with **zero backend**: connection create/reconnect/revoke (with
`token_last4`, and a rejected-token path for `invalid`/`expired` tokens), the apply step
(commit SHA + PR URL, idempotent, plus an `apply-fail` demo path), and `GET /ready`.
Switching to the real backend is still just `VITE_API_BASE=/api` — no component changes.

## Verified scenarios (Sprint 5)

| # | Scenario | Result |
| --- | --- | --- |
| S1 | Connect: token sent once, metadata stored, no token in storage/memory | PASS |
| S2 | Invalid/expired token → friendly error + field highlight | PASS |
| S3 | Save config → unlock AI tabs | PASS |
| S4 | Issue → plan → edit → approve (edited plan forwarded) | PASS |
| S5 | Approve → code generation → generated file operations | PASS |
| S6 | Apply to GitFlame → commit SHA + real PR URL; files marked applied | PASS |
| S7 | Apply idempotent; apply failure recoverable (branch untouched) | PASS |
| S8 | Request correction / Reject | PASS |
| S9 | Recommendations analyze (sends repository_context) → grid; Create issue | PASS |
| S10 | Refresh keeps the workspace via the session cookie (no token re-entry) | PASS |
| S11 | Reconnect gate on 401/403 → PUT reconnect restores the session | PASS |
| S12 | Disconnect → revoke + logout → back to connect screen | PASS |
| S13 | Backend unreachable / GitFlame down / Agent busy → distinct messages | PASS |

S1–S13 verified in mock mode; the mock backend flow is covered by a runtime smoke test
(connection → approve(edited) → code-gen → apply → recommendations → reconnect → revoke).

## Cross-team notes / dependencies

- **Live end-to-end** still depends on GitFlame exposing its repository API at
  `GITFLAME_BASE_URL` and a valid token with read (and, for apply, write) scope — the
  backend's stated dependency, not a frontend gap.
- **Recommendations analyze context.** The direct `.../recommendations/analyze` endpoint
  requires the caller to send `repository_context` (≥1 file path); it does not self-fetch
  the tree on this path (only the webhook path does). The frontend now sends the
  non-excluded paths from the known file tree so the call is valid. If the backend later
  fetches context itself for the direct path (like the webhook path), the frontend field
  becomes redundant but harmless. Flagged to Arthur.
- **No `GET /integrations/gitflame/connections` yet.** After the tab is closed (not just
  refreshed) the frontend cannot re-list connections from the backend, so a fresh tab
  starts from the connect screen. Non-blocking; noted for the backend backlog.
