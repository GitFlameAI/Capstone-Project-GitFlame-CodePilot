# User Stories & Acceptance Criteria — Frontend (Sprint 1)

These cover the frontend-facing slice of the MVP. They can be lifted directly
into the LaTeX report (User Stories / Acceptance Criteria sections). Each story
maps to a concrete component so the demo is traceable.

---

### US-1 — Enable AI for a repository
**As a** repository owner, **I want** an obvious entry point to enable AI on my
repo, **so that** I can start using code generation and recommendations.

**Acceptance criteria**
- A purple **Work with AI** button is visible on the repository Code page, next
  to History / Access.
- Clicking it opens a wizard with two actions: configure AI and work on an issue.
- _Component:_ `RepoToolbar.vue` → `WorkWithAiWizard.vue`

---

### US-2 — Configure `.ai.yml`
**As a** user, **I want** to choose AI settings through a form, **so that** I get
a valid `.ai.yml` without writing YAML by hand.

**Acceptance criteria**
- The form exposes: default branch, target branch prefix, analysis include/exclude
  paths, code-generation toggle and approval requirement, recommendation
  categories, severity threshold, and RAG max files.
- A live preview of the generated `.ai.yml` updates as the form changes.
- The output matches the Sprint 1 draft `.yml` structure and can be copied.
- _Component:_ `YamlConfigPanel.vue`

---

### US-3 — Get an implementation plan from an issue
**As a** user, **I want** the system to turn an issue into a Markdown plan, **so
that** I can see how the change would be implemented before any code is written.

**Acceptance criteria**
- The user enters issue title/body and submits.
- A loading state is shown while the plan is generated.
- The returned plan is rendered as Markdown with a status of "Plan generated".
- _Component:_ `IssuePlanPanel.vue`

---

### US-4 — Approve / correct / reject a plan
**As a** user, **I want** to approve, request corrections to, or reject a plan,
**so that** I stay in control of what the AI does.

**Acceptance criteria**
- Approve: shows a mock branch name, PR URL and reviewer.
- Correct: accepts feedback text and returns a regenerated plan.
- Reject: closes the plan.
- _Component:_ `IssuePlanPanel.vue`

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

---

### US-6 — Open the detailed analysis
**As a** user, **I want** a full analysis page, **so that** I can review every
recommendation with its file, line, problem and suggestion.

**Acceptance criteria**
- A "View detailed analysis" link opens the report page (`/recommendations`).
- Cards can be filtered by severity; resolved cards can be shown or hidden.
- Each card shows severity, optional category, file:line, problem, suggestion and
  confidence.
- _Component / view:_ `DetailedAnalysisView.vue`

---

### US-7 — Resolve or dismiss a recommendation
**As a** user, **I want** to mark a recommendation resolved or dismiss it, **so
that** the list reflects what I have handled.

**Acceptance criteria**
- "Mark resolved" sets the card to a resolved (greyed) state and updates counts.
- "Dismiss" removes the card from the list.
- Both actions show a per-card busy state and survive a page refresh against the
  live backend.
- _Components:_ `RecommendationCard.vue`, `RecommendationsWidget.vue`,
  `DetailedAnalysisView.vue`

---

### US-8 — Connect the demo to the backend
**As a** developer, **I want** the demo to run standalone or against the real Go
backend, **so that** I can demo without infrastructure and still validate the
contracts.

**Acceptance criteria**
- With no configuration, the app runs in mock mode (in-browser backend).
- Setting `VITE_API_BASE` switches every call to the real backend with no code
  change; `/api` is proxied to port 8000 in development.
- _Files:_ `api/index.js`, `api/client.js`, `api/mock.js`, `vite.config.js`

---

> Note: this file supports the report's User Stories / Acceptance Criteria
> sections. If the report owns the canonical copy, this can be linked rather than
> duplicated.
