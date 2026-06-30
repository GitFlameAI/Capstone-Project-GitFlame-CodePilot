# GitFlame CodePilot — Frontend (Sprint 3 / Version 3)

Vue 3 demo UI for the **GitFlame CodePilot** AI integration service.

CodePilot is an AI service that GitFlame connects to. This UI demonstrates the
integration from the outside, as the GitFlame team and the TAs would experience it.
The frontend stays **thin** (no business logic), talks **only to the Go backend**, and
never calls the SERGE-based Agent Engine directly.

## The Sprint 3 flow

```
/            Mock GitFlame repository page — the only integration point is the purple
             "Work with AI" button (the eyebrow and the top chip link to gitflame.ru).
                              │  Work with AI
                              ▼
/codepilot   CodePilot landing: what it does (autogeneration vs recommendations), a
             connect form (repository URL, default branch, access token, webhook URL),
             an AI disclaimer + consent (with a readable service usage policy), Continue.
                              │  Continue
                              ▼
/workspace   Four tabs, left → right:
             Repository · Config · Autogeneration · Recommendations
             Autogeneration and Recommendations stay LOCKED until a .ai.yml is saved.
```

### Repository tab
Three blocks stacked top to bottom: **Connection** (editable — you can switch to a
different repository / branch / token; changing the repository re-locks the AI tabs
because the `.ai.yml` is per-repository), **Files** (a name/path-only tree), and
**Recommendations** (a short analysis summary, or a prompt to configure / analyse).

### Config tab
The form follows the agreed configuration contract in
`docs/config/ai_config_spec.md` (branch `sprint-3/danil-codegen-contracts`), which is
intentionally small. Only four things are configurable:

| Field | Maps to |
| --- | --- |
| Default branch | `repository.default_branch` |
| Exclude paths (chip multi-select) | `analysis.exclude` |
| Recommendation categories (toggles) | `recommendations.categories` |
| Keep reports for (days) | `storage.recommendation_ttl_days` |

If **no category** is selected, the system produces **no recommendations**. A live
`.ai.yml` preview updates as the form changes; **Save** unlocks the last two tabs.

### Autogeneration tab
Pick an **existing repository issue** (fields auto-fill) or **create a new one** (empty
form). The user does **not** enter repository context — the Agent Engine prepares it via
RAG (`context_AI/ml/autogen_prompt.md`). Submitting polls the plan task; the plan is
**editable** (Edit / Preview). **Approve & generate code** queues a code-generation task
and lists the **generated file operations** — each is `{ action, path, description }`,
matching `generated_files_contract.md` (no diffs/content; branch/commit/PR/reviewer come
from the backend wrapper). The result panel has **Back to issues** (returns to the issue
picker) and **Go to pull request**. **Request correction** and **Reject** are also
available; Approve and Reject show independent loading states.

### Recommendations tab
A compact **grid of small cards** (category + confidence + severity + short problem).
Clicking a card opens a **detail overlay** (dim background) where you can page through
recommendations with ←/→, **delete** one, or **create an issue** from it (which hands off
to the Autogeneration tab with the title/description pre-filled). Filters sit in one row: a **confidence sort** toggle (ascending / descending), a
**Categories** multi-select and a **Severity** multi-select (each with All / None).
There is no "resolved" state — a recommendation is either turned into an issue or
dismissed. Severity is kept and explained in a legend and in the overlay.

## Tech stack

- Vue 3 (`<script setup>` SFCs) + Vue Router 4, Vite 6
- Plain JavaScript (no TypeScript) — runs as-is after `npm install`
- No UI/icon libraries — inline SVG icons, GitFlame palette (purple `#905BFB`,
  Geologica font), tokens in `src/styles/theme.css`

## Requirements

- Node.js 18+ (tested on Node 22), npm 9+

## Quick start (standalone, no backend)

```bash
cd frontend
npm install
npm run dev
```

Open the printed URL (default http://localhost:5173). The app runs **standalone in mock
mode** by default — no backend, database, Redis or GPU required. The in-browser mock
seeds a demo report and simulates the full async task lifecycle, so every loading state
is visible. This is what the Version 3 screenshots / video are captured from.

### Demo walkthrough

1. On `/`, press **Work with AI**.
2. On the landing screen, read the explainer and the **service usage policy** link, fill
   the connect form (repo URL is pre-filled; enter any access token and tick both consent
   boxes — leaving them blank shows the red-underline validation), press **Continue**.
3. **Config:** note the "i-in-circle" hints, adjust exclude paths / categories, then
   **Save .ai.yml** (unlocks the last two tabs).
4. **Autogeneration:** pick an existing issue or create a new one, **Generate plan**, edit
   the plan, then **Approve & generate code** to see the generated file operations.
5. **Recommendations:** browse the card grid, sort by confidence or filter by category & severity, open a card to page
   through, delete, or **Create issue** (jumps to Autogeneration pre-filled).

### Triggering error / retry / timeout states in mock mode

The mock reads the **issue title**: a title containing `fail` → `502 agent_engine_error`
(Retry appears); `timeout` → `504 inference_timeout`; an empty title/description/author →
a validation error.

## Mock mode vs. live backend

Mode is selected by `VITE_API_BASE` (empty = mock; set, e.g. `/api`, = live HTTP). To run
against the Go backend (port 8000): `cp .env.example .env` (it contains
`VITE_API_BASE=/api`) and `npm run dev`. `vite.config.js` proxies `/api` →
`http://localhost:8000`.

> Contract note: the Config form emits the Sprint 3 configuration contract
> (`docs/config/ai_config_spec.md`). The backend YAML parser still enforces the older,
> larger schema; reconciling the two is tracked in `docs/review/internal_review.md`
> (finding F9). Mock mode is unaffected.

## Endpoints consumed

Issue → plan → code-generation:
`POST /integrations/gitflame/issues/analyze` → `GET /ai/tasks/{taskId}` (poll) →
`POST /ai/tasks/{taskId}/retry` · `POST /ai/issues/{id}/approve` →
`GET /ai/issues/{id}/code-generation` (poll) · `POST /ai/issues/{id}/correct` ·
`POST /ai/issues/{id}/reject`.

Recommendations:
`POST /integrations/gitflame/repositories/{id}/recommendations/analyze` ·
`GET /repositories/{id}/recommendations[/status|/summary]` ·
`DELETE /recommendations/{id}` (Sprint 3 UI uses analyze / summary / list / delete;
`PATCH /recommendations/{id}/close` remains in the client for backend parity but is no
longer used by the UI).

## Project structure

```
frontend/src/
  router.js                 # / , /codepilot , /workspace
  store/session.js          # reactive() singleton: connection + config + pendingIssue
  data/demo.js              # demo repo, issues, file tree, exclude options, .ai.yml build/parse
  utils/markdown.js         # dependency-free Markdown renderer
  api/{index,client,mock}.js
  components/
    ui/                     # GfButton, GfIcon, GfSpinner, GfModal, GfTooltip
    RepoChrome, RepoToolbar, FileBrowser     # mock GitFlame page
    FileTree, ContextPicker, MarkdownView    # workspace building blocks
    workspace/{RepositoryTab,ConfigTab,AutogenTab,RecommendationsTab}.vue
  views/{GitFlameDemoView,LandingView,WorkspaceView}.vue
```

## Build

```bash
npm run build     # outputs to dist/
npm run preview   # serves the production build locally
```
