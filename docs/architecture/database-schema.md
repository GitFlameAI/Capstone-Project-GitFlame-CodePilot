# Database Schema

![Initial database ER diagram](./db-er-diagram.png)

This document describes the PostgreSQL storage structure used by GitFlame CodePilot in Sprint 2.

The main change from Sprint 1 is that workflow state is no longer stored only in backend memory. Issue sessions, generated plans, plan revisions, Agent Engine task states, and recommendation retention are persisted in PostgreSQL.

## Main Idea

`repositories` is the root entity for both product flows. The backend stores GitFlame repository identifiers as `external_id`, while PostgreSQL keeps its own internal UUID primary keys.

`ai_configs` stores snapshots of the `.yml` configuration. The original YAML is stored in `raw_yml`, while the parsed snapshot is stored in `parsed_config_json`. This lets old sessions point to the exact configuration that was used when the plan or recommendation was created.

The issue workflow is represented by:

- `issue_sessions`
- `generated_plans`
- `plan_revisions`
- `agent_tasks`
- `user_responses`

The recommendation workflow is represented by:

- `recommendation_runs`
- `recommendations`
- `recommendation_statuses`

## Issue Workflow

`issue_sessions` stores one workflow session for one GitFlame issue. It keeps the external issue id, title, body, author, current workflow status, and the current plan revision number.

`generated_plans` stores the current Markdown plan for an issue session. The backend reads this table when it needs to return the latest plan to the UI or GitFlame integration.

`plan_revisions` stores the history of generated plans. Each correction creates a new revision with its own `revision_number`, full `plan_markdown`, and optional `correction_feedback`. Revisions store full Markdown content instead of diffs, because this is simpler to restore and verify in Sprint 2.

`agent_tasks` stores the Agent Engine task state. The planned statuses are `queued`, `processing`, `completed`, and `failed`. The table also stores a short `tool_execution_summary`, but it does not store full model reasoning.

`user_responses` stores user decisions: approve, correct, or reject.

## Recommendation Workflow

`recommendation_runs` stores the recommendation summary for a repository analysis run. Sprint 2 adds `retention_days` and `expires_at` so the backend can keep recommendation reports only for the period selected by the user in the `.yml` configuration.

`recommendations` stores individual recommendation cards with file, line, severity, problem, suggestion, confidence, and current status.

`recommendation_statuses` stores status history for recommendation cards.

## Migration File

The schema source of truth is:

```text
backend/db/migrations/initial_schema.sql
```

Docker Compose mounts this file into the PostgreSQL container initialization directory. If the database volume already exists, recreate the volume before expecting the initialization file to run again.

## Verification

The verification script is:

```text
backend/db/verification.sql
```

It inserts sample records and checks that:

- an issue session can be saved;
- a generated plan can be linked to the session;
- a plan revision can store correction feedback;
- an agent task can store its status;
- a recommendation run can store retention data.
