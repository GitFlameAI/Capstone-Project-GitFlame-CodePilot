# Frontend — Sprint 3 (Version 3)

Owner: Roman (frontend)
Branch: `sprint-3/roman-frontend`

The frontend is deliberately **thin**: no business logic, talks **only** to the Go
backend (never to the SERGE-based Agent Engine directly), and exists to visualise and
demo the integration. This document reflects the Sprint 3 UI after the Week-4 revisions.

## 1. What changed since Sprint 2

Sprint 2 was a single GitFlame-like page with a modal wizard. Sprint 3 reframes CodePilot
as a service GitFlame connects to, and rebuilds the UI as a landing screen plus a
four-tab workspace.

| Area | Sprint 2 | Sprint 3 |
| --- | --- | --- |
| Structure | Repo page + modal wizard | Mock GitFlame page → service **landing** → 4-tab **workspace** |
| Onboarding | none | Connect form + **AI disclaimer / consent** with a readable **usage policy** modal |
| Config | YAML-ish panel, many fields | **Minimal contract** (4 fields), exclude paths as a chip picker, live preview |
| Issue input | free-text context paths | **Pick an existing issue** (auto-fill) or **create new**; no user-entered context |
| Plan | read-only | **Editable Markdown** (Edit / Preview) |
| After approval | contract metadata | polled **code-generation task** → file operations `{action, path, description}`; **Back to issues** + **Go to PR** |
| Recommendations | widget + list with resolve/dismiss | **card grid** + **detail overlay** (←/→, delete, create issue) + **sort / category / severity filters**; no "resolved" |
| Gating | none | Autogeneration & Recommendations **locked** until `.ai.yml` saved |

## 2. Screens and routes

| Route | View | Purpose |
| --- | --- | --- |
| `/` | `GitFlameDemoView.vue` | Mock GitFlame repository page. The only integration point is the purple **Work with AI** button; the hero eyebrow and the top chip link to `gitflame.ru`. |
| `/codepilot` | `LandingView.vue` | Hero + autogeneration/recommendations explainer, the **connect form** (repo URL, default branch, masked access token, read-only webhook URL), the **AI disclaimer + consent** with a **service usage policy** modal, and **Continue** (centered, validates before navigating). |
| `/workspace` | `WorkspaceView.vue` | Four tabs with an AI-disclaimer banner and lock gating. |

## 3. Workspace tabs

**Repository** — three vertical blocks: *Connection* (editable via a modal that calls
`updateConnection`; switching repository clears the per-repository `.ai.yml` and re-locks
the AI tabs), *Files* (recursive name/path tree), and *Recommendations* (a short analysis summary, or a prompt to configure / analyse).

**Config** — the user-facing `.ai.yml` editor. Follows `docs/config/ai_config_spec.md`:
`repository.default_branch`, `analysis.exclude` (chip multi-select), `recommendations.categories`
(toggles), `storage.recommendation_ttl_days`. Everything the old form had (include paths,
RAG limits, severity threshold, code-generation toggles, reviewer) was dropped from the
contract and is not shown. No category selected ⇒ no recommendations. Live YAML preview;
**Save** unlocks the last two tabs.

**Autogeneration** — issue source step (existing issue dropdown that auto-fills, or a new
empty issue), then the issue form (title / description / author — no context field). The
plan is editable; **Approve** polls a code-generation task and renders file operations
`{action, path, description}` plus the backend wrapper data (branch / commit / PR /
reviewer). The result panel offers **Back to issues** and **Go to pull request**. Approve
and Reject have independent loading states.

**Recommendations** — a grid of small cards (category, confidence, severity, truncated
problem). A card opens a **detail overlay** (`GfModal`, dim backdrop) with the full
problem/suggestion, file:line, ←/→ navigation, **Delete**, and **Create issue** (sets
`session.pendingIssue` and switches to Autogeneration, which reads it on mount). Filters sit in one row: a confidence sort
toggle, a Categories multi-select and a Severity multi-select (each with All / None). No "resolved"
concept. Severity is kept and explained in a legend/tooltip and in the overlay.

## 4. Async flow (`AutogenTab.vue`)

```
select issue → form → analyze → task → poll → editable plan
plan → approve → code-generation task → poll → done (files + PR)
plan → correct → revision task → poll → plan
plan → reject  → done (rejected)
```

Polling is centralised in `pollTask()` (`src/api/index.js`): polls `GET /ai/tasks/{taskId}`
until terminal, supports an `AbortSignal`, and raises `client_timeout` after 120 s. Both
the plan task and the code-generation task are polled the same way. Approve / correct /
reject are addressed by `session_id`.

## 5. State and handoff (`store/session.js`)

A plain `reactive()` singleton holds the connection and saved config (no Pinia). New in
Sprint 3: `updateConnection({url, defaultBranch, token})` for switching repositories, and
`pendingIssue` for the Recommendations → Autogeneration "create issue" handoff. `webhookFor(id)`
centralises the webhook URL shown on the landing screen and the Repository tab.

## 6. Run modes

- **Mock (default):** `npm run dev` — fully demoable, no backend.
- **Live (dev):** `cp .env.example .env` (`VITE_API_BASE=/api`); Vite proxies `/api` → `:8000`.
- **Docker:** built with `VITE_API_BASE=/api`; nginx proxies `/api/` → `backend:8000` and
  serves the SPA (`try_files … /index.html`), so `/codepilot` and `/workspace` resolve.

## 7. Quality gates

- `npm run build` compiles cleanly (70 modules, no warnings) in both modes.
- No new runtime dependencies (still only `vue` + `vue-router`).
- The Markdown renderer HTML-escapes input before re-introducing markup.
- Superseded Sprint 2 components and now-orphaned recommendation components were removed
  to avoid dead code.
