# Version 2 Features — Frontend (Sprint 2)

Owner: Roman (frontend) · Branch: `sprint-2/roman-frontend`

This document lists the **Version 2** frontend features and how they differ from the
Sprint 1 MVP. It backs the weekly report section "Version 2 Features" and links each
feature to a user story and the matching board task.

## 1. New / changed features vs Sprint 1 MVP

1. **Live backend mode.** The UI now talks to the real Go backend (`VITE_API_BASE`),
   not only the in-memory mock. The mock is kept as an offline demo fallback so the
   master branch stays runnable without GPU / Redis / PostgreSQL.
2. **Asynchronous issue → plan flow.** `analyze` returns an agent **task**; the UI polls
   `GET /ai/tasks/{taskId}` and renders the live status (**queued → processing →
   completed / failed**) instead of blocking on one synchronous call.
3. **Plan feedback loop.** Approve, **Request correction** (with a feedback field), and
   Reject are wired to the backend. Corrections are asynchronous and produce a new plan
   revision.
4. **Generated-files contract.** On approve the UI shows the backend's
   `generated_files_contract` (branch name, commit message, PR title, reviewer) — the
   payload a future GitFlame-side worker will use to open a branch and PR.
5. **Full UI state coverage.** loading, empty, success, **validation error**, **Agent
   Engine error**, **timeout**, and **retry** states are all implemented.
6. **Recommendations on the live backend.** Cards, summary, status counts and the
   close/dismiss actions call the real endpoints; repository context was corrected to
   file paths.

## 2. User stories ↔ board tasks

> Format: **US-Fx** (user story) → board task → acceptance.

- **US-F1 — Submit an issue to the AI.**
  *As a developer, I want to describe an issue (title, description, repository context)
  and start AI plan generation, so I get an implementation plan without leaving the repo.*
  → Board task: *"Реализовать форму нового issue: title, description, repository context"*.
  → Acceptance: form validates required fields client-side and on the backend (422);
  submit creates a task and moves to the progress view.

- **US-F2 — See generation progress.**
  *As a developer, I want to see whether my plan is queued, processing, or done, so I am
  not left staring at a frozen screen.*
  → Board task: *"После отправки issue показывать status: queued, processing, completed, failed"*.
  → Acceptance: the UI polls the task and shows the live status; transitions are visible.

- **US-F3 — Read the generated plan.**
  *As a developer, I want to read the Markdown implementation plan once it is ready.*
  → Board task: *"После completed отображать generated plan.md"*.
  → Acceptance: completed task plan is rendered.

- **US-F4 — Act on the plan.**
  *As a developer, I want to approve, correct, or reject the plan.*
  → Board task: *"Подключить кнопки approve/correct/reject; для correct поле feedback"*.
  → Acceptance: approve shows the generated-files contract; correct (with feedback)
  produces a new revision; reject ends the flow.

- **US-F5 — Recover from failures.**
  *As a developer, when the Agent Engine errors or times out, I want a clear message and
  a retry option.*
  → Board task: *"Добавить loading, empty, success, validation error, Agent Engine error,
  timeout и retry states"*.
  → Acceptance: failed (recoverable) tasks show Retry; client timeouts show "keep waiting".

- **US-F6 — Review repository recommendations.**
  *As a developer, I want to see AI recommendations and mark them resolved or dismiss them.*
  → Board tasks: *"Подключить recommendation cards к backend API"*, *"close/delete actions"*.
  → Acceptance: cards load from the backend; close marks resolved; delete removes the card.

## 3. Screenshots / GIFs (to attach in the PDF report)

> Capture these from the running demo (`npm run dev`). Suggested shots:

- `issue-form.png` — the new issue form with the repository-context field.
- `task-processing.gif` — queued → processing → plan generated.
- `plan-actions.png` — generated plan with Approve / Request correction / Reject.
- `approve-contract.png` — generated-files contract after approve.
- `agent-error-retry.gif` — failed task + Retry (title containing "fail" in mock mode).
- `recommendations.png` — recommendations widget with cards and resolve/dismiss.

## 4. Architecture diagram (for the report)

```
[Vue Frontend]  ──REST──▶  [Go Backend]  ──Redis queue──▶  [Agent Worker]
      ▲                          │                               │
      │   poll GET /ai/tasks     │                               ▼
      └──────────────────────────┘                  [SERGE-based Agent Engine]
                                                       │  Repository tools / RAG
                                                       ▼
                                                     Model → validated plan.md
```

The frontend only ever calls the Go backend. It never calls the Agent Engine directly.
