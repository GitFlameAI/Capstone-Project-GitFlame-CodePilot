# GitFlame CodePilot — Frontend (Sprint 4 / Version 4)

Vue 3 demo UI for the **GitFlame CodePilot** AI integration service.

CodePilot is an AI service that GitFlame connects to. This UI demonstrates the
integration from the outside, as the GitFlame team and the TAs would experience it.
The frontend stays **thin** (no business logic), talks **only to the Go backend**, and
never calls the SERGE-based Agent Engine directly.

## Sprint 4 (Version 4) — what changed

Sprint 4 is a usability pass driven by feedback, plus alignment with the new backend
GitFlame integration endpoints:

- **Landing redesign.** The two text explainers were replaced with an interactive
  **"How it works" roadmap** and a **"Try it yourself"** hands-on preview. The roadmap has
  **two tracks** (Autogeneration / Recommendations) with a toggle; steps auto-advance with a
  **progress bar that fills along the connector** toward the next step, and when a track ends
  it **auto-switches** to the other so a visitor sees the whole tour. The preview mirrors the
  real flow's user control: **Edit / Preview** the plan, **Request correction**, or a red
  **Reject**.
- **Single consent.** The two consent checkboxes were merged into one — the service usage
  policy (which itself covers "AI output may be wrong — trust, but verify" and how the repo
  and access token are used).
- **Repository file tree is now interactive.** Every file/folder has an **Exclude / Include**
  toggle (excluded rows show an **Include** call-to-action with an open-eye icon); excluding a
  whole folder collapses its files to `folder/**`; the `.ai.yml` file itself is protected. All
  folders start **collapsed**. Toggles edit the config **draft** and are two-way with the
  Config **Exclude paths** picker.
- **Config draft persists; `.ai.yml` changes only on Save.** Edits in the Config form (and the
  file-tree toggles) are held in a working draft that **survives switching tabs**; the saved
  `.ai.yml` — and the tab gating — only change when you press **Save**. A dirty-aware Save
  button and an "Unsaved · review in Config" hint on the Repository tab make pending changes
  visible.
- **Recommendations.** Opening the tab with no stored report **runs the analysis
  automatically**; the manual button is only **Re-run**. Dismissing the **last** card no longer
  re-runs immediately (the code/config are unchanged) — it re-runs automatically the next time
  the tab is opened.
- **Refresh no longer drops you to the landing.** The session (repository, config, draft) is
  snapshotted to `sessionStorage`, so a page refresh restores the workspace and re-fetches
  data. The access **token is never persisted**, so on refresh a **mandatory, non-dismissable
  token gate** appears (purple-accented token field). If the backend reports the token
  expired/invalid (401/403), the same gate shows an explicit **error** so the user isn't left
  guessing.
- **Live GitFlame events.** The webhook URL points at the backend receiver
  (`/api/integrations/gitflame/webhooks/issues`), is **copyable**, and has an **"i" tooltip**
  explaining its purpose; a demo "Simulate a push" control refreshes the tree/issues in place.
- **Start with.** The landing keeps a **Start with** choice (Autogeneration / Recommendations)
  with a tooltip clarifying it isn't a restriction. Routing: no saved config → the **Config**
  tab; config present → the chosen capability tab.
- **Layout & responsive.** Consistent centered block widths (the Connect card now matches the
  other blocks, with its header outside the card); a **sticky** workspace top bar so the repo
  name / config status stay visible; tab switches scroll to the header; the AI disclaimer is
  wide enough to fit on one line; Config spacing tightened to fit one screen; **All / None**
  for categories, **Clear all** for excludes, retention clamped to 1–365; the "or" between the
  issue pickers is vertically centered; the contract shows the **base branch** and uses the
  real `pull_request_url`. A responsive pass removes right-overflow at high zoom / narrow
  widths (grids, rows and overlays wrap or scroll instead of overflowing).

Full detail: `docs/frontend/sprint_4_frontend.md`.

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
2. On the landing screen, read the **roadmap**, try the **"Try it yourself"** preview, then
   fill the connect form (enter a repository URL and any access token, tick the single
   consent box — leaving it blank shows the red-underline validation), press **Continue**.
3. **Repository:** click **Exclude** on any file or folder (it strikes through and updates
   `.ai.yml`); copy the **webhook URL**; press **Simulate a push (demo)** to watch the tree
   and issues update in place.
4. **Config:** note the "i-in-circle" hints, adjust exclude paths (**Clear all** available)
   and categories (**All / None**), then **Save .ai.yml** (unlocks the last two tabs).
5. **Autogeneration:** pick an existing issue or create a new one, **Generate plan**, edit
   the plan, then **Approve & generate code** to see the generated file operations, the base
   branch and the PR contract.
6. **Recommendations:** the analysis **runs automatically** on first open; browse the card
   grid, sort by confidence or filter by category & severity, open a card to page through,
   delete, or **Create issue** (jumps to Autogeneration pre-filled). Use **Re-run** to refresh.

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
