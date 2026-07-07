# Frontend — Sprint 4 (Version 4)

Owner: Roman (frontend)
Branch: `sprint-4/roman-frontend`

The frontend stays **thin**: no business logic, talks **only** to the Go backend, and
exists to visualise, demo and make the integration usable. Sprint 4 is a usability pass
driven by TA / peer feedback, plus alignment with the new backend GitFlame-integration
endpoints (`arthur-backend`, `amir-db-storage`).

## 1. What changed since Sprint 3

| Area | Sprint 3 | Sprint 4 |
| --- | --- | --- |
| Landing | Hero + capability toggle with two text lists | Hero + interactive **two-track roadmap** (auto-advancing, progress bar along the connector, auto-switch between Autogeneration/Recommendations) + **"Try it yourself"** hands-on preview |
| Preview control | — | The preview lets you **Edit / Preview** the plan, **Request correction**, or **Reject** (red) — mirroring "you approve" |
| Consent | Two checkboxes (AI + policy) | **One** checkbox — the usage policy (covers "trust, but verify" + repo/token use) |
| File tree | Read-only names/paths | **Interactive Exclude / Include** per file & folder; excluded rows show an **Include** call-to-action (open-eye); **all folders collapsed** by default |
| Config edits | Local form; lost on tab switch; exclude synced live to `.ai.yml` | Edits held in a **draft** that persists across tab switches; the saved **`.ai.yml` changes only on Save**; dirty-aware Save |
| Recommendations | Manual **Run analysis** | **Auto-runs** on first open; dismissing the **last** card does **not** re-run immediately — it re-runs on the next visit |
| Refresh | Dropped to the landing (state lost) | Session snapshot in `sessionStorage` restores the workspace; a **mandatory token gate** appears (token is never persisted) |
| Token problems | Silent / generic errors | Explicit **token expired/invalid** state (from backend 401/403) shown in the token gate |
| Webhook | Copyable receiver URL | + an **"i" tooltip** explaining its purpose on the Repository tab |
| Layout | Per-tab widths | One centered width (Connect card matches other blocks, header outside the card); **sticky** top bar; scroll-to-header on tab switch; responsive pass for high zoom |

## 2. Feedback -> change (usability)

| Feedback / observation | Change |
| --- | --- |
| "Show a moving indicator so I know when the roadmap advances" | Progress bar **fills along the connector** toward the next circle |
| "The Recommendations block in the roadmap looked bad; show its steps too" | Roadmap is now a **two-track** switcher; the Recommendations track has its own steps (analyse -> browse -> open a card -> dismiss / create issue), and it **auto-switches** after the Autogeneration track |
| "Selecting Autogeneration in *Start with* didn't route there; and *Start with* reads like a restriction" | Fixed routing (no config -> Config; config -> chosen tab) and added a tooltip + "You can switch anytime" hint |
| "Excluded files should invite re-including" | Excluded rows now say **Include** with an open-eye icon |
| "Folders should start collapsed" | All folders collapsed by default |
| "Config edits shouldn't vanish when I switch tabs; `.ai.yml` should change only on Save" | Config **draft** persists; Save is the only commit point |
| "Don't re-run recommendations right after I clear the last card" | Re-runs on the **next** tab visit instead |
| "Refresh shouldn't kick me to the landing; ask for the token instead and show token problems" | Session restore + **mandatory token gate** + explicit invalid-token state |
| "Connect card is narrower than the rest; move its title out; add an open-in-new icon by GitFlame" | Width matched, header moved outside the card, `external` icon added to the eyebrow |
| "The AI disclaimer wraps awkwardly / header hidden after switching tabs / Config doesn't fit one screen" | Wider disclaimer, sticky top bar + scroll-to-header, tighter Config spacing |
| "Some elements overflow to the right at very high zoom" | Responsive pass: grids use `minmax(min(px,100%),1fr)`, rows wrap, overlays scroll |

## 3. Key implementation notes

- **Config draft** (`store/session.js`): `configForm` is the last **saved** config (drives the
  real `.ai.yml` + tab gating + the recommendation analysis); `configDraft` is the **working**
  copy edited by the Config form and the file-tree toggles. `configDirty()` compares the two;
  `saveConfig()` is the only place the saved YAML and `configExists` change.
- **Exclude sync** (`utils/excludePaths.js`): the editing source of truth is the set of concrete
  excluded file paths; toggles mutate it, then it is serialised back to a minimal pattern list
  (fully-excluded folders collapse to `folder/**`, custom globs like `node_modules/**` are kept,
  `.ai.yml` is protected). Config picker and tree both read `configDraft.excludePaths`.
- **Session persistence** (`store/session.js`): a snapshot (repository, intent, saved config and
  draft - **never the token**) is written to `sessionStorage` and rehydrated at module load,
  before the router guard, so a refresh stays in the workspace. The file tree / issues are
  re-derived, not stored.
- **Token gate** (`views/WorkspaceView.vue` + `api/index.js`): the raw token lives only in memory.
  On refresh `tokenStatus` becomes `missing` -> a mandatory overlay. In live mode, any backend
  401/403 is caught in the API facade -> `markTokenInvalid()` -> `tokenStatus = invalid` -> the
  same overlay shows the error. Mock mode has no real auth, so the *invalid* path is live-mode only.
- **Roadmap** (`components/landing/Roadmap.vue`): two tracks; a CSS `scaleX` animation on the
  connector shows time-to-next-step; auto-advance pauses on hover and respects
  `prefers-reduced-motion`.

## 4. New / changed files

New: `src/utils/clipboard.js`, `src/utils/excludePaths.js`,
`src/components/landing/Roadmap.vue`, `src/components/landing/TryDemo.vue`.

Changed: `src/store/session.js`, `src/api/index.js`, `src/api/mock.js`, `src/data/demo.js`,
`src/styles/theme.css`, `src/views/LandingView.vue`, `src/views/WorkspaceView.vue`,
`src/components/FileTree.vue`, `src/components/ContextPicker.vue`, `src/components/RepoChrome.vue`,
`src/components/workspace/{RepositoryTab,ConfigTab,AutogenTab,RecommendationsTab}.vue`,
`README.md`.

## 5. Acceptance criteria

- Landing shows a two-track roadmap with a moving progress bar and auto-switch, plus a working
  preview with Edit/Preview, Request correction and a red Reject; a single consent gates Continue.
- Excluding a file/folder strikes it through and updates the draft; a fully-excluded folder shows
  as `folder/**`; `.ai.yml` cannot be excluded; folders start collapsed; excluded rows say **Include**.
- Config edits survive switching tabs; the `.ai.yml` and tab unlock change only on **Save**.
- Opening Recommendations with no report auto-runs; dismissing the last card re-runs on next visit.
- A page refresh keeps the workspace and shows a mandatory token gate; an invalid/expired token
  (live mode) shows an explicit error.
- No element overflows to the right at high zoom / narrow widths.
- `npm run build` succeeds; the app runs in mock mode with no backend.
