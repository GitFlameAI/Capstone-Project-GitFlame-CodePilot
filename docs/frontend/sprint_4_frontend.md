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
| Landing | Hero + capability toggle with two text lists | Hero + interactive **two-track roadmap** behind a **sliding toggle**, a **progress bar along the connector**, a **play/pause** control and **auto-switch** between tracks, plus a **"Try it yourself"** hands-on preview |
| Preview control | — | The preview renders Markdown in **Preview**, and lets you **Edit / Preview**, **Request correction** (type what to change, like the real tab) or **Reject** (red) |
| Consent | Two checkboxes (AI + policy) | **One** checkbox — the usage policy |
| File tree | Read-only names/paths | **Interactive Exclude / Include** per file & folder; excluded rows show an **Include** call-to-action; **all folders collapsed** by default |
| Config edits | Local form; lost on tab switch; exclude synced live to `.ai.yml` | Edits held in a **draft** that persists across tab switches; the saved **`.ai.yml` changes only on Save**; dirty-aware Save; a **Discard changes** button; a green first-save banner |
| Recommendations | Manual **Run analysis** | **Auto-runs** on first open; dismissing the **last** card re-runs on the next visit, not immediately |
| Routing | Capability chosen in a **"Start with"** control | **"Start with" removed** — the workspace always opens on **Config** |
| Connect form | Default branch pre-filled; plain submit | Default branch **empty**; **Continue** button **dimmed until complete** (still clickable) |
| Refresh | Dropped to the landing (state lost) | Session snapshot restores the workspace; a **mandatory token gate** appears (token never persisted) — **in mock mode too** |
| Layout | Per-tab widths | One centered width; **sticky** top bar; scroll-to-header; locked-tab hint links to **Config**; responsive pass for high zoom (toggle stacks, long exclude tokens wrap) |

## 2. Feedback -> change (usability)

| Feedback / observation | Change |
| --- | --- |
| "Give the roadmap toggle an outline on hover and a smooth Telegram-style slide" | The toggle uses a **sliding pill** (animated `translateX`) and shows a purple hover outline on the inactive tab |
| "Some step transitions fire long after the bar has already finished" | Timing is now driven by a **single timer** whose remaining time is tracked across pauses; the CSS progress bar pauses/resumes on the same events, so the bar and the step change stay in sync (the old double-interval / `*2`-on-resume drift is gone) |
| "Show whether auto-switching is running, and let me stop/start it" | A **play/pause** control (dim grey-purple, top-left) toggles auto-advance; hover-pause is kept |
| "Preview in the landing demo shows raw code, not Markdown; make Request correction match the real tab" | The preview reuses the real **MarkdownView** (working Edit/Preview) and the same **Request correction** interaction (textarea + Submit correction + revision counter) |
| "'Start with' didn't route correctly and reads like a restriction — just always open Config" | Removed **"Start with"**; the workspace always opens **Config** |
| "Dim the Continue button until the form is complete; don't pre-fill Default branch" | Added a **dimmed** (but still clickable) state driven by a `formComplete` check; Default branch starts empty |
| "The Config save hint is illogical when the config already exists; bring back the green 'Go to Autogeneration' banner" | Hint now: *Save to unlock…* only before the first save, *Save changes to the X branch* when dirty, *All changes are saved…* otherwise; a green **first-save banner** returns |
| "Add a button to discard unsaved config changes" | **Discard changes** reverts the draft to the saved config (`resetDraft`) |
| "In the locked-tab hint, make 'Config' a link and shrink the plate" | *Config* is now a link to the tab and the plate is `width: fit-content` |
| "A long token in Exclude paths stretches the whole page" | Chips ellipsise, the menu/hint wrap, and the column can shrink (`min-width: 0`) |
| "At very high zoom the roadmap 'Recommendations' label overflows" | At narrow widths the toggle stacks vertically so labels never clip |
| "Refresh lost the token but showed no gate (mock)" | The token gate now shows on refresh **in mock mode too** (`tokenStatus === 'missing'`) |

## 3. Key implementation notes

- **Config draft** (`store/session.js`): `configForm` = last **saved** config (drives the real
  `.ai.yml`, the tab gating and the recommendation analysis); `configDraft` = the **working** copy
  edited by the Config form and the file-tree toggles. `configDirty()` compares the two;
  `saveConfig()` is the only place the saved YAML / `configExists` change; `resetDraft()` reverts
  the draft to the saved config.
- **Save banner** (`components/workspace/ConfigTab.vue`): a single `watch(dirty)` clears the
  "saved" banner only when the draft becomes dirty again, so the green banner survives the
  draft-reassignment that `saveConfig()` performs (the previous field-watchers reset it too early).
- **Roadmap timing** (`components/landing/Roadmap.vue`): `running = autoOn && !hovering &&
  !reduceMotion`; a single `setTimeout` carries `remaining` ms and is held/resumed with the same
  wall-clock as the CSS bar (`.rm_paused` → `animation-play-state: paused`). The sliding pill is a
  CSS `translateX(0 / 100%)`; at ≤440px the toggle stacks and the pill is hidden.
- **Session persistence** (`store/session.js`): a snapshot (repository, saved config, draft —
  **never the token**) is written to `sessionStorage` and rehydrated at module load, before the
  router guard. On refresh `tokenStatus` becomes `missing` → a mandatory overlay (mock and live).
- **Token gate** (`views/WorkspaceView.vue` + `api/index.js`): the raw token lives only in memory;
  a live 401/403 is caught in the API facade → `markTokenInvalid()` → the gate shows an error.
- **Exclude sync** (`utils/excludePaths.js`): concrete excluded paths are the editing source of
  truth; serialised back to a minimal pattern list (fully-excluded folders → `folder/**`, custom
  globs kept, `.ai.yml` protected).

## 4. New / changed files

New: `src/utils/clipboard.js`, `src/utils/excludePaths.js`,
`src/components/landing/Roadmap.vue`, `src/components/landing/TryDemo.vue`.

Changed: `src/store/session.js`, `src/api/index.js`, `src/api/mock.js`, `src/data/demo.js`,
`src/styles/theme.css`, `src/views/LandingView.vue`, `src/views/WorkspaceView.vue`,
`src/components/FileTree.vue`, `src/components/ContextPicker.vue`, `src/components/RepoChrome.vue`,
`src/components/ui/GfIcon.vue`,
`src/components/workspace/{RepositoryTab,ConfigTab,AutogenTab,RecommendationsTab}.vue`,
`README.md`.

## 5. Acceptance criteria

- The roadmap toggle slides, shows a hover outline, has a working play/pause, and the progress
  bar and step change stay in sync (no late jumps).
- The landing preview renders Markdown in Preview and offers a type-your-own Request correction.
- Excluding a file/folder updates the draft; a fully-excluded folder shows as `folder/**`;
  `.ai.yml` cannot be excluded; folders start collapsed; excluded rows say **Include**.
- Config edits survive switching tabs; `.ai.yml` and the tab unlock change only on **Save**;
  **Discard changes** reverts; the first save shows a green "Go to Autogeneration" banner; the
  save hint matches the state.
- The workspace always opens on Config; Default branch is empty; Continue is dimmed until the
  form is complete but still clickable.
- A page refresh keeps the workspace and shows the mandatory token gate (mock and live).
- No element overflows to the right at high zoom / narrow widths (roadmap toggle stacks, long
  exclude tokens wrap).
- `npm run build` succeeds; the app runs in mock mode with no backend.
