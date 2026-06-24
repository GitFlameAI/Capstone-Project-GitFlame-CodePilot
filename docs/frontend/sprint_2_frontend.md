# Frontend — Sprint 2 (Version 2)

Owner: Roman (frontend)
Branch: `sprint-2/roman-frontend`

This document describes the Sprint 2 state of the Vue.js demo UI and how it talks to
the Go backend. The frontend is deliberately **thin**: it contains no business logic,
talks **only** to the Go backend (never to the SERGE-based Agent Engine directly), and
exists to visualise and demo the integration.

## 1. What changed since Sprint 1

In Sprint 1 the "Work with AI" issue flow was **synchronous**: a single call returned a
finished plan. In Sprint 2 the backend moved plan generation onto a **Redis queue + Agent
Worker + SERGE-based Agent Engine**, so the flow is now **asynchronous**. The frontend was
rewritten to match that contract.

| Area | Sprint 1 (MVP) | Sprint 2 (Version 2) |
| --- | --- | --- |
| Data source | In-memory mock only | **Real Go backend** via `VITE_API_BASE` (mock kept as offline fallback) |
| Issue → plan | One synchronous call returns the plan | `analyze` returns a **task**; UI **polls** `GET /ai/tasks/{taskId}` until `completed`/`failed` |
| Task states | none | **queued → processing → completed / failed** rendered live |
| Correction | synchronous | **asynchronous** (returns a new task that is polled) |
| Approve result | mock `git_workflow` + `pull_request_url` | `generated_files_contract` (branch, commit, PR title, reviewer) |
| Error handling | single error string | **validation / Agent Engine error / timeout / retry** states |
| Recommendations | wired to backend | wired to backend (context fixed to file paths; states hardened) |

## 2. Component map

```
src/
  api/
    index.js        # selects mock vs http; exposes api + pollTask() helper
    client.js       # real HTTP client (Sprint 2 endpoints, error code surfaced)
    mock.js         # offline backend; simulates the async task lifecycle
  components/
    WorkWithAiWizard.vue   # modal with two tabs: Configure AI / Work on an issue
    YamlConfigPanel.vue    # .yml generation (Sprint 1)
    IssuePlanPanel.vue     # *rewritten* async issue → plan → approve/correct/reject
    RecommendationsWidget.vue  # summary + preview cards (Code tab)
    RecommendationCard.vue     # single recommendation card (close / dismiss)
  views/
    RepositoryView.vue        # GitFlame-like Code tab (integration points)
    DetailedAnalysisView.vue  # full recommendations report page
```

## 3. Issue → plan state machine (IssuePlanPanel.vue)

```
form ──analyze()──▶ task ──poll──▶ plan ──approve──▶ done (approved + contract)
  ▲                  │   │            │ ──reject───▶ done (rejected)
  │                  │   │            └─correct──▶ task (poll) ──▶ plan (new revision)
  │                  │   └─failed──▶ failed ──retry──▶ task
  │                  └─client timeout──▶ timeout ──keep waiting──▶ task
  └──────────────────────────── start over ──────────────────────┘
```

Polling is centralised in `pollTask()` (`src/api/index.js`). It calls
`GET /ai/tasks/{taskId}` on an interval until the task is terminal, supports an
`AbortSignal` (polling stops when the modal closes), and raises a `client_timeout`
after 120 s so the UI can offer "keep waiting".

`approve` / `correct` / `reject` are addressed by **`session_id`** (the backend accepts
either the session id or the issue id on `/ai/issues/{id}/...`).

## 4. Backend contract used by the frontend

| Method | Path | Used for |
| --- | --- | --- |
| POST | `/integrations/gitflame/issues/analyze` | start plan generation → returns `task_id` |
| GET | `/ai/tasks/{taskId}` | poll task status / read finished plan |
| POST | `/ai/tasks/{taskId}/retry` | retry a recoverable failed task |
| POST | `/ai/issues/{id}/approve` | approve plan → `generated_files_contract` |
| POST | `/ai/issues/{id}/correct` | request a correction (async, body `{feedback}`) |
| POST | `/ai/issues/{id}/reject` | reject plan |
| POST | `/integrations/gitflame/repositories/{id}/recommendations/analyze` | run analysis |
| GET | `/repositories/{id}/recommendations[/status\|/summary]` | load report |
| PATCH | `/recommendations/{id}/close` | mark resolved |
| DELETE | `/recommendations/{id}` | dismiss |

The frontend sends **only fields the backend knows** (the backend uses
`DisallowUnknownFields`). Issue requests send `repository_context` as a list of file
paths (used for RAG retrieval by the Agent Engine).

## 5. Run modes

- **Mock (default, no backend):** `npm run dev` — fully demoable for screenshots/GIFs.
- **Live backend (dev):** copy `.env.example` → `.env`, set `VITE_API_BASE=/api`; Vite
  proxies `/api` → `http://localhost:8000`.
- **Docker:** the image is built with `VITE_API_BASE=/api` and nginx proxies `/api/` →
  `http://backend:8000/`, so the deployed UI always runs against the live backend.

A small badge in the recommendations widget indicates when mock mode is active.
