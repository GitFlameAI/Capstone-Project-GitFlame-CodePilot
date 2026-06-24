# User Stories & Acceptance Criteria — Frontend (Sprint 2 / Version 2)

These cover the frontend-facing slice of Version 2. They can be lifted directly
into the LaTeX report (User Stories / Acceptance Criteria sections). Each story
maps to a concrete component **and to a Sprint 2 board task** so the demo is
traceable end to end.

> What changed from Sprint 1: the issue → plan call is now **asynchronous**
> (the backend creates an agent task and the frontend polls `GET /ai/tasks/{taskId}`),
> `/correct` is async as well, and the approve response is a
> `generated_files_contract` — there is **no `pull_request_url`** in Version 2.
> US-3 and US-4 below are revised accordingly; US-5…US-8 are unchanged.

---

### US-1 — Enable AI for a repository
**As a** repository owner, **I want** an obvious entry point to enable AI on my
repo, **so that** I can start using code generation and recommendations.

**Acceptance criteria**
- A purple **Work with AI** button is visible on the repository Code page, next
  to History / Access.
- Clicking it opens a wizard with two actions: configure AI and work on an issue.
- _Component:_ `RepoToolbar.vue` → `WorkWithAiWizard.vue`
- _Board task:_ "Реализовать форму нового issue …" (entry point to the form).

---

### US-2 — Configure `.ai.yml`
**As a** user, **I want** to choose AI settings through a form, **so that** I get
a valid `.ai.yml` without writing YAML by hand.

**Acceptance criteria**
- The form exposes: default branch, target branch prefix, analysis include/exclude
  paths, code-generation toggle and approval requirement, recommendation
  categories, severity threshold, and RAG max files.
- A live preview of the generated `.ai.yml` updates as the form changes.
- _Component:_ `YamlConfigPanel.vue`
- _Board task:_ carried over from Sprint 1; the YAML is now sent as
  `yaml_config` with every analyze call.

---

### US-3 — Submit an issue and watch generation progress  *(revised for V2)*
**As a** user, **I want** the system to turn an issue into a Markdown plan
asynchronously, **so that** I can submit an issue and watch its status instead of
waiting on a frozen request.

**Acceptance criteria**
- The form accepts issue **title, description (body), author** and a
  **repository context** (one file path per line), plus the `.ai.yml`.
- Submitting calls `POST /integrations/gitflame/issues/analyze`, which returns
  `202 { session_id, task_id, status: "queued" }`.
- The UI then polls `GET /ai/tasks/{taskId}` and shows the live status:
  **queued → processing → completed / failed**.
- On `completed`, `task.plan_markdown` is rendered as the plan.
- _Component:_ `IssuePlanPanel.vue`; _helpers:_ `api/index.js` `pollTask()`
- _Board tasks:_ "Реализовать форму нового issue: title, description, repository
  context"; "После отправки issue показывать status: queued, processing,
  completed, failed"; "После completed отображать generated plan.md".

---

### US-4 — Approve / correct / reject a plan  *(revised for V2)*
**As a** user, **I want** to approve, request corrections to, or reject a plan,
**so that** I stay in control of what the AI does.

**Acceptance criteria**
- **Approve** (`POST /ai/issues/{id}/approve`) returns and displays the
  `generated_files_contract`: branch name, file operations, commit message,
  PR title and reviewer. *(No `pull_request_url` in V2 — code/PR creation is a
  later sprint.)*
- **Correct** (`POST /ai/issues/{id}/correct { feedback }`) accepts feedback text
  and returns `202 { task_id }`; the UI polls the **new** task and shows the
  revised plan.
- **Reject** (`POST /ai/issues/{id}/reject`) closes the plan.
- `{id}` is the `session_id` returned by analyze.
- _Component:_ `IssuePlanPanel.vue`
- _Board task:_ "Подключить кнопки approve, correct и reject; для correct
  добавить поле feedback".

---

### US-4b — Recover from Agent Engine failures  *(new in V2)*
**As a** user, **I want** clear failure, timeout and retry handling, **so that** a
transient Agent Engine error doesn't lose my work.

**Acceptance criteria**
- A failed task surfaces `task.error { http_status, code, detail }` in a readable
  error state.
- If the error is recoverable (`502/503/504` or
  `agent_engine_error / agent_engine_unreachable`), a **Retry** button calls
  `POST /ai/tasks/{taskId}/retry` and resumes polling.
- A client-side **timeout** state appears if polling exceeds the limit, with a
  "keep waiting" option.
- Form **validation errors** (missing title / context / YAML) are shown inline
  before any request is sent.
- _Component:_ `IssuePlanPanel.vue`; _helper:_ `pollTask()` (`client_timeout`,
  `cancelled` codes)
- _Board task:_ "Добавить loading, empty, success, validation error, Agent Engine
  error, timeout и retry states".

---

### US-5 — See a recommendations summary
**As a** user, **I want** a recommendations summary on the Code tab, **so that** I
can quickly gauge the health of my repository.

**Acceptance criteria**
- The widget shows a loading state, then a summary and the top recommendation cards.
- Open vs. resolved counts are visible.
- If no analysis exists yet, an empty state offers to run one.
- Errors show a retry action.
- _Component:_ `RecommendationsWidget.vue`
- _Board task:_ "Подключить recommendation cards к backend API".

---

### US-6 — Open the detailed analysis
**As a** user, **I want** a full analysis page, **so that** I can review every
recommendation with its file, line, problem and suggestion.

**Acceptance criteria**
- A "View detailed analysis" link opens the report page (`/recommendations`).
- Cards can be filtered by severity; resolved cards can be shown or hidden.
- Each card shows severity, file:line, problem, suggestion and confidence.
- _Component / view:_ `DetailedAnalysisView.vue`
- _Board task:_ "Подключить recommendation cards к backend API".

---

### US-7 — Resolve or dismiss a recommendation
**As a** user, **I want** to mark a recommendation resolved or dismiss it, **so
that** the list reflects what I have handled.

**Acceptance criteria**
- "Mark resolved" (`PATCH /recommendations/{id}/close`) sets the card to a
  resolved (greyed) state and updates counts.
- "Dismiss" (`DELETE /recommendations/{id}`) removes the card from the list.
- Both actions show a per-card busy state and survive a page refresh against the
  live backend.
- _Components:_ `RecommendationCard.vue`, `RecommendationsWidget.vue`,
  `DetailedAnalysisView.vue`
- _Board task:_ "Подключить close/delete actions для recommendations".

---

### US-8 — Connect the demo to the backend
**As a** developer, **I want** the demo to run standalone or against the real Go
backend, **so that** I can demo without infrastructure and still validate the
contracts.

**Acceptance criteria**
- With no configuration, the app runs in mock mode (in-browser backend) that now
  simulates the full async task lifecycle.
- Setting `VITE_API_BASE` switches every call to the real backend with no code
  change; `/api` is proxied to port 8000 in development.
- _Files:_ `api/index.js`, `api/client.js`, `api/mock.js`, `vite.config.js`
- _Board task:_ "Переключить frontend с mock mode на Go backend API"; "Не
  связывать frontend напрямую с SERGE-based Agent Engine".

---

> Note: a condensed, report-ready version of these stories (US-F1…US-F6) with the
> same board-task mapping lives in `docs/report/version_2_features.md`. This file
> is the detailed frontend copy; keep the two in sync if either changes.
