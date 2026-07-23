# GitFlame CodePilot — Frontend (Sprint 6 / Version 6, final)

Vue 3 demo UI for the **GitFlame CodePilot** AI integration service.

CodePilot is an AI service that GitFlame connects to. This UI demonstrates the
integration from the outside, as the GitFlame team and the TAs would experience it.
The frontend stays **thin** (no business logic), talks **only to the Go backend**, and
never calls the SERGE-based Agent Engine directly.

- **Live demo (VM):** http://10.93.27.34/
- **Backend Swagger:** http://10.93.27.34:8000/swagger/
- **Repository:** https://github.com/kite121/Capstone-Project-GitFlame-CodePilot

## What the product does

Two AI capabilities, both under human approval:

1. **Issue → Plan → Code → Pull request.** Pick a repository issue (or write one),
   CodePilot drafts a Markdown implementation plan you can edit, approve, correct or
   reject. On approval it generates a set of file operations, and **Apply to GitFlame**
   opens a branch + commit + pull request for review.
2. **Repository recommendations.** CodePilot analyses the repository against your
   `.ai.yml` and returns cards (severity, category, file, line, problem, suggestion,
   confidence) you can filter, dismiss, or turn into an issue.

## Tech stack

- **Vue 3** (`<script setup>`) + **Vue Router 4**, built with **Vite 6**.
- No state library — a single `reactive()` store (`src/store/session.js`), in line with
  the "boring and reliable" rule.
- No runtime dependencies beyond `vue` + `vue-router`.
- Served in production by **nginx** (multi-stage `Dockerfile`), which also proxies
  `/api/` to the Go backend so the app and API are same-origin.

## Sprint 6 (Version 6) — what changed

Sprint 6 is the final polish pass on the workspace UI, driven by usability feedback.
No new dependencies, routes, or contracts — only clearer, real-data behaviour:

- **Real "Exclude paths" suggestions.** The Config → *Exclude paths* picker no longer
  offers a hard-coded list of generic globs. Its suggestions are now derived from the
  **actual connected repository tree** (`folder/**` for every real directory, `*.min.js`
  / `*.lock` / `*.map` only when such files exist, and top-level files as exact paths).
  This removes the last placeholder/mock data from the workspace; the demo GitFlame host
  page at `/` is intentionally still a simulation. (`utils/excludePaths.js`,
  `components/workspace/ConfigTab.vue`, `data/demo.js`.)
- **Correct file-tree indentation.** In the Repository tab, files inside nested folders
  are now indented to match their parent folder, so it is always clear which folder a
  file belongs to. (`components/FileTree.vue`.)
- **Clearer "How it works" auto-play control.** On `/codepilot`, the play/pause control
  now reflects the *effective* state: hovering the block visibly pauses it (the icon
  switches to ▶), pressing **Resume** keeps it advancing even while the cursor stays on
  the block, and moving the cursor out and back pauses it again. (`components/landing/Roadmap.vue`.)
- **De-duplicated Re-run.** The Recommendations "repository changed" banner no longer
  carries its own *Re-run now* button; it points to the single **Re-run** already in the
  Summary just below. (`components/workspace/RecommendationsTab.vue`.)

Full write-up: `docs/frontend/sprint_6_frontend.md`.

## Sprint 5 (Version 5) — what changed

Sprint 5 connects the frontend to the **real backend** end-to-end and hardens the
error handling. Highlights:

- **Secure GitFlame connection flow.** The frontend no longer holds the GitFlame
  access token. You enter it once on the connect screen; the backend validates it,
  stores it **AES-GCM encrypted**, creates a server session, and returns an **HttpOnly
  `codepilot_session` cookie** plus connection metadata only (`connection id`,
  `repository`, `token_last4`, `token_status`). Every request is sent with
  `credentials:'include'`. The token is never written to `localStorage`,
  `sessionStorage`, a JS cookie, or even kept in memory after the request.
  - Connect: `POST /integrations/gitflame/connections`
  - Reconnect / replace token: `PUT /integrations/gitflame/connections/{id}`
  - Disconnect: `DELETE /integrations/gitflame/connections/{id}` (+ `DELETE /auth/session`)
- **Apply to GitFlame.** After code generation the workspace shows an explicit
  **Apply to GitFlame** step (`POST /ai/issues/{id}/gitflame/apply`) that opens the
  branch/commit/PR and shows the returned **commit SHA** and the **real pull-request
  URL**. Applying is idempotent and errors are recoverable (nothing is written to your
  default branch).
- **Edited plan is honoured.** Approve now forwards the exact (optionally edited)
  `plan_markdown` the user reviewed, so code generation uses that plan.
- **Refresh keeps you signed in.** Because the session lives in the HttpOnly cookie, a
  page refresh restores the workspace without re-entering the token. A **reconnect
  prompt** appears only when the backend reports the session/token is expired or
  revoked (401/403). (In mock mode a "Simulate expired session (demo)" button makes
  this demonstrable without a backend.)
- **Clear error states.** A single mapping (`src/api/errors.js`) turns every backend
  failure into a friendly message + next action: invalid/expired token, GitFlame
  unavailable, Agent Engine busy, queue/database unavailable, apply failed, or the
  backend being unreachable.

Full write-up: `docs/frontend/sprint_5_frontend.md`. Internal review:
`docs/review/internal_review.md`.

## The flow

```
/            Mock GitFlame repository page — the only integration point is the purple
             "Work with AI" button.
                              │  Work with AI
                              ▼
/codepilot   CodePilot landing: an interactive "How it works" roadmap and hands-on
             preview, then a connect form (repository URL, default branch, access
             token). Continue sends the token ONCE to the backend and stores only the
             returned metadata.
                              │  Continue
                              ▼
/workspace   Four tabs, left → right:
             Repository · Config · Autogeneration · Recommendations
             Autogeneration and Recommendations stay LOCKED until a .ai.yml is saved.
```

### Repository tab
Connection details (editable — reconnect with a new token, or disconnect entirely; the
token status is shown), an interactive file tree with per-file/-folder **Exclude/Include**
toggles, the copyable **webhook URL** to register in GitFlame, and a recommendations
summary.

### Config tab
Edits the repository `.ai.yml` (default branch, `analysis.exclude`,
`recommendations.categories`, `storage.recommendation_ttl_days`). The *Exclude paths*
picker suggests patterns built from the **real repository tree** (and still accepts any
custom pattern you type). Edits are held in a **draft** that survives tab switches;
**Save** commits them and unlocks the AI tabs.

### Autogeneration tab
Pick an issue or create one → editable Markdown plan (**Edit / Preview**, **Request
correction**, **Reject**) → **Approve & generate code** → generated file operations →
**Apply to GitFlame** → **Go to pull request** (real PR URL).

### Recommendations tab
Auto-runs the analysis; a grid of cards filterable by category/severity; open a card to
**Delete** it or **Create issue** (handed to Autogeneration).

## Run it

Mock mode (no backend — the default):

```bash
cd frontend
npm install        # only needed once
npm run dev        # open the printed URL (http://localhost:5173)
npm run build      # production build check
```

Live mode (against the Go backend):

```bash
# from the repo root, bring up the stack (backend, db, redis, agent engine, ml, recs):
docker compose -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml up -d --build

# then either open the containerised frontend (built with VITE_API_BASE=/api), or run
# the dev server against the backend:
cd frontend
echo "VITE_API_BASE=/api" > .env
npm run dev        # the Vite proxy forwards /api -> http://localhost:8000
```

The backend needs `GITFLAME_BASE_URL`, `GITFLAME_CREDENTIAL_KEY`, and (on plain HTTP)
`SESSION_COOKIE_SECURE=false`. See the root `.env.example` and
`infra/sprint5-reproducibility-runbook.md`.

## Project layout

```
src/
  api/
    index.js      selects mock vs real backend; wraps auth failures; pollTask()
    client.js     real HTTP client (credentials:'include', connection + apply endpoints)
    mock.js       in-memory backend with the same shapes (connection, tasks, apply)
    errors.js     backend error code/status -> friendly { title, message, kind }
  store/session.js  reactive session: connection metadata (never the token) + config draft
  data/demo.js      demo repo/issues/tree + .ai.yml <-> form helpers
  views/            GitFlameDemoView (/), LandingView (/codepilot), WorkspaceView (/workspace)
  components/
    landing/        Roadmap, TryDemo
    workspace/      RepositoryTab, ConfigTab, AutogenTab, RecommendationsTab
    ui/             GfButton, GfModal, GfSpinner, GfTooltip, GfIcon
    FileTree, ContextPicker, MarkdownView, MultiSelect, RepoChrome
  utils/            clipboard, excludePaths, markdown
```

## Track / contribution

Industrial track. The frontend makes the CodePilot ↔ GitFlame integration usable and
demonstrable: a real connection handshake, the full issue→plan→approve→generate→apply
pull-request flow, and the recommendations experience — all against the same Go backend
contract the GitFlame team would integrate with, with a mock fallback so the demo always
runs. Frontend owner: **Roman Titov** (`sprint-5/roman-frontend`).
