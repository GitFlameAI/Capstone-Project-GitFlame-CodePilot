# GitFlame CodePilot — Frontend (Sprint 2 / Version 2)

Vue 3 demo UI that imitates a GitFlame repository page and demonstrates the two
AI integration points planned for the product:

- a purple **Work with AI** button (next to "History") that opens a wizard
  for configuring `.ai.yml` and for running the **issue → plan → approve/correct/reject** loop;
- an **AI Recommendations** widget on the Code tab, with a dedicated **detailed
  analysis** page (filters, resolve, dismiss).

The UI is intentionally independent of GitFlame's internal code: GitFlame is
treated as an external product that sends us `.yml`, issue data and repo metadata,
and renders our structured responses. The frontend talks **only to the Go backend**
and never calls the SERGE-based Agent Engine directly.

## What changed in Sprint 2 (Version 2)

In Sprint 1 the issue → plan call was **synchronous**: one POST returned the plan.
In Sprint 2 the backend creates an **asynchronous agent task**, so the flow is now:

```
POST /integrations/gitflame/issues/analyze   -> 202 { session_id, task_id, status: "queued" }
GET  /ai/tasks/{taskId}  (poll)              -> status: queued | processing | completed | failed
   completed -> task.plan_markdown            (rendered as plan.md)
   failed    -> task.error { http_status, code, detail }  (+ Retry if recoverable)
POST /ai/tasks/{taskId}/retry                -> 202 (only for recoverable Agent Engine errors)
POST /ai/issues/{id}/approve                 -> generated_files_contract
POST /ai/issues/{id}/correct  { feedback }   -> 202 { task_id }  (async again — poll the new task)
POST /ai/issues/{id}/reject                  -> 200
```

The frontend now implements the full state machine for this flow:
**loading / queued / processing / completed / failed / timeout / retry**, plus
form **validation errors** and **Agent Engine errors** surfaced from `task.error`.

> Note: `{id}` in the `/ai/issues/{id}/...` routes accepts either the
> `session_id` or the original `issue_id`. This frontend consistently uses the
> `session_id` returned by `analyze`.

## Tech stack

- Vue 3 (`<script setup>` SFCs) + Vue Router 4
- Vite 6 build tooling
- Plain JavaScript (no TypeScript) so the code runs as-is after `npm install`
- No UI/icon libraries — icons are inline SVG, styling follows the GitFlame
  palette (signature purple `#905BFB`, Geologica font)

## Requirements

- Node.js 18+ (tested on Node 22)
- npm 9+

## Quick start (standalone, no backend)

```bash
cd frontend
npm install
npm run dev
```

Open the printed URL (default http://localhost:5173).

The app runs **standalone in mock mode** by default — no backend required. The
mock backend lives entirely in the browser, seeds a demo report, and now
simulates the **full async task lifecycle** (queued → processing → completed)
with small delays so every loading state is visible. This is what the Version 2
screenshots/GIFs are captured from.

### Triggering the error / retry / timeout states in mock mode

The mock reads the **issue title** to decide the outcome, so you can demo every
branch without a backend:

| Put this in the issue title | Result |
| --- | --- |
| anything normal (e.g. "Add pagination") | task **completes**, plan is shown |
| contains **`fail`** | task **fails** with `502 agent_engine_error` → **Retry** appears (retry then succeeds) |
| contains **`timeout`** | task **fails** with `504 inference_timeout` → **Retry** appears |
| leave title or repository context empty | **validation error** on the form |

## Mock mode vs. live backend

Mode is selected automatically by the `VITE_API_BASE` environment variable.

| Mode | When | What it does |
| --- | --- | --- |
| Mock (default) | `VITE_API_BASE` is **unset / empty** | In-browser backend, no server needed |
| Live | `VITE_API_BASE` is set (e.g. `/api`) | Real HTTP calls to the Go backend |

To run against the team's Go backend (Artur's service, port 8000):

```bash
# 1. start the Go backend so it listens on :8000
#    (full stack: docker-compose up — backend + postgres + redis + agent-worker)

# 2. point the frontend at it
cp .env.example .env
# .env already contains: VITE_API_BASE=/api

npm run dev
```

`vite.config.js` proxies `/api` → `http://localhost:8000`, so the browser is not
affected by CORS during development. The frontend uses exactly the endpoint
shapes the Go backend exposes, so no code change is needed to switch modes.

## Endpoints consumed

Issue → plan workflow (Version 2, async):

- `POST   /integrations/gitflame/issues/analyze`  → `202 { session_id, task_id, status }`
- `GET    /ai/tasks/{taskId}`                      → task status + `plan_markdown` / `error`
- `POST   /ai/tasks/{taskId}/retry`               → re-queue a recoverable failed task
- `POST   /ai/issues/{id}/approve`                → `generated_files_contract`
- `POST   /ai/issues/{id}/correct`  `{ feedback }`→ `202 { task_id }` (poll again)
- `POST   /ai/issues/{id}/reject`

Recommendations:

- `POST   /integrations/gitflame/repositories/{id}/recommendations/analyze`
- `GET    /repositories/{id}/recommendations/status`
- `GET    /repositories/{id}/recommendations/summary`
- `GET    /repositories/{id}/recommendations`
- `PATCH  /recommendations/{id}/close`
- `DELETE /recommendations/{id}`

## Project structure

```
frontend/
├── index.html              # loads fonts, mounts #app
├── vite.config.js          # @ alias, dev proxy /api -> :8000
├── .env.example            # VITE_API_BASE toggle
├── src/
│   ├── main.js             # app entry
│   ├── App.vue             # router shell
│   ├── router.js           # / and /recommendations
│   ├── api/
│   │   ├── index.js        # selects mock vs http + pollTask() helper
│   │   ├── client.js       # real fetch client + ApiError (carries error code)
│   │   └── mock.js         # in-browser backend, async task lifecycle + seed data
│   ├── data/demo.js        # demo repo, files, default .ai.yml, repository context
│   ├── styles/theme.css    # GitFlame design tokens
│   ├── components/
│   │   ├── ui/             # GfButton, GfModal, GfIcon, GfSpinner
│   │   ├── RepoChrome.vue, RepoToolbar.vue, FileBrowser.vue
│   │   ├── RecommendationsWidget.vue, RecommendationCard.vue, SeverityBadge.vue
│   │   ├── WorkWithAiWizard.vue, YamlConfigPanel.vue, IssuePlanPanel.vue
│   └── views/
│       ├── RepositoryView.vue        # / — Code page + widget + wizard
│       └── DetailedAnalysisView.vue  # /recommendations — full report
└── README.md
```

## Build

```bash
npm run build     # outputs to dist/
npm run preview   # serves the production build locally
```
