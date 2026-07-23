# Sprint 6 — Frontend (Version 6)

Owner: Roman Titov · Branch: `sprint-6/roman-frontend`

Sprint 6 is the final usability polish on the workspace UI. It introduces **no new
dependencies, routes, stores, or API/config contracts** — every change is a targeted fix
against feedback, and the frontend stays thin (talks only to the Go backend). `npm run
build` passes and mock mode still runs with no backend.

The whole product flow is unchanged; see `docs/frontend/sprint_5_frontend.md` for the
end-to-end connection / autogeneration / recommendations behaviour.

## 1. Real "Exclude paths" suggestions (no mock data in the workspace)

**Problem.** The Config → *Exclude paths* picker offered a hard-coded list of generic
globs (`node_modules/**`, `dist/**`, `target/**`, …) regardless of the connected
repository. Those patterns often do not exist in the repo, so the suggestions were
placeholder/mock data leaking into the live product.

**Change.** The picker options are now derived from the **real repository tree**
(`session.fileTree`), so they reflect the repository the user actually connected — in
both live and demo mode:

- `folder/**` for every directory in the tree (top-level and nested);
- `*.min.js`, `*.lock`, `*.map` **only** when the repository really contains such files;
- top-level files as exact paths;
- the `.ai.yml` config file is never suggested (it is protected from exclusion).

Before the tree has loaded the list is simply empty; the picker still lets the user type
any custom pattern and press Enter, so nothing is lost.

The generic-glob constant was removed from `data/demo.js`. This was the only mock data
that reached the workspace; the demo GitFlame host page at `/` is *intentionally* a
simulation (we do not control `gitflame.ru`), so it keeps its demo repository data.

Files: `src/utils/excludePaths.js` (new `excludePathOptionsFromTree(tree)`),
`src/components/workspace/ConfigTab.vue`, `src/data/demo.js`.

## 2. Correct file-tree indentation (Repository tab)

**Problem.** When a real repository tree was loaded, expanding a nested folder showed its
files with no left indentation — a file deep in the tree lined up with a top-level file,
so it was unclear which folder the files belonged to. Folder rows indented by depth; file
rows used a fixed spacer that ignored depth.

**Change.** File rows now receive the same depth-based left padding as folder rows
(`padding-left: 8 + depth * 16 px`), and the caret spacer's fixed margin was removed, so a
file's icon and name align exactly under its parent folder's icon and name. Purely visual;
the exclude/include logic is untouched.

Files: `src/components/FileTree.vue`.

## 3. Clearer "How it works" auto-play control (`/codepilot`)

**Problem.** The roadmap paused auto-advance while the cursor hovered the block, but the
top-left play/pause icon kept showing "playing", so the paused state was invisible; and
there was no way to keep it running while reading a step with the cursor on the block.

**Change.** The control now reflects and drives the **effective** running state:

- default: running;
- hovering the block: paused, and the icon switches to ▶ to show it;
- pressing **Resume** while hovering: keeps advancing even with the cursor on the block
  (a one-shot override);
- pressing **Pause**: stops;
- moving the cursor out and back onto the block: paused again (the override is cleared on
  leave).

Implemented with a `hoverOverride` flag folded into the existing single-timer `running`
computed, so the CSS progress bar and the step change stay in sync as before.

Files: `src/components/landing/Roadmap.vue`.

## 4. De-duplicated Re-run (Recommendations tab)

**Problem.** When the repository changed, the amber "repository changed" banner carried a
*Re-run now* button, duplicating the **Re-run** button in the Summary block immediately
below it.

**Change.** The banner's button was removed; its text now points to the single Re-run in
the Summary ("Use **Re-run** below to refresh the recommendations"). One obvious action
instead of two identical ones.

Files: `src/components/workspace/RecommendationsTab.vue`.

## Changed files

- `src/components/FileTree.vue`
- `src/components/workspace/ConfigTab.vue`
- `src/components/workspace/RecommendationsTab.vue`
- `src/components/landing/Roadmap.vue`
- `src/utils/excludePaths.js`
- `src/data/demo.js`
- `README.md` (frontend)

## Verified scenarios (Sprint 6)

- `npm run build` succeeds; the app still runs in mock mode with no backend.
- Repository tab: files inside nested folders are indented under their parent folder.
- Config tab: *Exclude paths* suggestions match the real connected repository (folders as
  `folder/**`, real lock/min files as globs); typing a custom pattern still works; the
  list is empty before the tree loads rather than showing fake globs.
- `/codepilot`: hovering the "How it works" block flips the icon to ▶ and pauses; Resume
  keeps it running with the cursor on the block; leaving and re-entering pauses again.
- Recommendations tab: the "repository changed" banner has no button; Re-run lives only in
  the Summary block.

## Cross-team notes / dependencies

None. This sprint touches only the frontend and depends on no backend/API changes. The
exclude-path suggestions read the same `session.fileTree` the Repository tab already
loads from the backend (`GET` repository files), so no new call is introduced.
