# GitFlame CodePilot — Frontend (Sprint 1)

Vue 3 demo UI that imitates a GitFlame repository page and demonstrates the two
AI integration points planned for the product:

- a purple **Work with AI** button (next to "History") that opens a wizard
  for configuring `.ai.yml` and for running the issue -> plan -> approve/correct/reject loop;
- an **AI Recommendations** widget on the Code tab, with a dedicated **detailed
  analysis** page (filters, resolve, dismiss).

The UI is intentionally independent of GitFlame's internal code: GitFlame is
treated as an external product that sends us `.yml`, issue data and repo metadata,
and renders our structured responses. This matches the Sprint 1 integration
boundaries.

## Tech stack

- Vue 3 (`<script setup>` SFCs) + Vue Router 4
- Vite 6 build tooling
- Plain JavaScript (no TypeScript) so the code runs as-is after `npm install`
- No UI/icon libraries — icons are inline SVG, styling follows the GitFlame
  palette (signature purple `#905BFB`, Geologica font)

## Requirements

- Node.js 18+ (tested on Node 22)
- npm 9+

## Quick start

```bash
cd frontend
npm install
npm run dev
```

Open the printed URL (default http://localhost:5173).

The app runs **standalone in mock mode** by default — no backend required. The
mock backend lives entirely in the browser, seeds a demo report, and adds small
delays so loading states are visible. This is what the Sprint 1 screenshots/GIFs
are captured from.

## Mock mode vs. live backend

Mode is selected automatically by the `VITE_API_BASE` environment variable.

| Mode | When | What it does |
| --- | --- | --- |
| Mock (default) | `VITE_API_BASE` is **unset** | In-browser backend, no server needed |
| Live | `VITE_API_BASE` is set | Real HTTP calls to the Go backend |

To run against the team's Go backend (Artur's service, port 8000):

```bash
# 1. start the Go backend so it listens on :8000
#    (see backend/ — runs on BACKEND_PORT, default 8000)

# 2. point the frontend at it
cp .env.example .env
# .env already contains: VITE_API_BASE=/api

npm run dev
```

`vite.config.js` proxies `/api` -> `http://localhost:8000`, so the browser is not
affected by CORS during development. The frontend uses exactly the endpoint
shapes the Go backend exposes, so no code change is needed to switch modes.

## Endpoints consumed

Recommendations:

- `POST /integrations/gitflame/repositories/{id}/recommendations/analyze`
- `GET  /repositories/{id}/recommendations/status`
- `GET  /repositories/{id}/recommendations/summary`
- `GET  /repositories/{id}/recommendations`
- `PATCH /recommendations/{id}/close`
- `DELETE /recommendations/{id}`

Issue -> plan workflow:

- `POST /integrations/gitflame/issues/analyze`
- `GET  /ai/issues/{id}/plan`
- `POST /ai/issues/{id}/approve`
- `POST /ai/issues/{id}/correct`
- `POST /ai/issues/{id}/reject`

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
│   │   ├── index.js        # selects mock vs http by VITE_API_BASE
│   │   ├── client.js       # real fetch client + ApiError
│   │   └── mock.js         # in-browser backend + seed data
│   ├── data/demo.js        # demo repo, files, default .ai.yml
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
