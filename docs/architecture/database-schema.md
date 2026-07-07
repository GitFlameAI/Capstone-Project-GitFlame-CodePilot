# Database Schema

![Initial database ER diagram](./db-er-diagram.png)

This document describes the PostgreSQL storage structure used by GitFlame CodePilot in Sprint 4.

The main change from Sprint 1 is that workflow state is no longer stored only in backend memory. Issue sessions, generated plans, plan revisions, Agent Engine task states, GitFlame integration metadata, generated file application results, and recommendation retention are persisted in PostgreSQL.

## Main Idea

`repositories` is the root entity for both product flows. The backend stores GitFlame repository identifiers as `external_id`, while PostgreSQL keeps its own internal UUID primary keys.

`ai_configs` stores snapshots of the `.yml` configuration. The original YAML is stored in `raw_yml`, while the parsed snapshot is stored in `parsed_config_json`. This lets old sessions point to the exact configuration that was used when the plan or recommendation was created.

GitFlame integration storage is represented by:

- `gitflame_connections`
- `gitflame_webhooks`
- `gitflame_webhook_events`
- `repository_snapshots`
- `repository_snapshot_files`

The issue workflow is represented by:

- `issue_sessions`
- `generated_plans`
- `plan_revisions`
- `agent_tasks`
- `agent_task_statuses`
- `user_responses`

The recommendation workflow is represented by:

- `recommendation_runs`
- `recommendations`
- `recommendation_statuses`

## Issue Workflow

`issue_sessions` stores one workflow session for one GitFlame issue. It keeps the external issue id, title, body, author, current workflow status, and the current plan revision number.

`generated_plans` stores the current Markdown plan for an issue session. The backend reads this table when it needs to return the latest plan to the UI or GitFlame integration.

`plan_revisions` stores the history of generated plans. Each correction creates a new revision with its own `revision_number`, full `plan_markdown`, and optional `correction_feedback`. Revisions store full Markdown content instead of diffs, because this is simpler to restore and verify in Sprint 2.

`agent_tasks` stores the current Agent Engine task state. The planned statuses are `queued`, `processing`, `completed`, and `failed`. The table also stores a short `tool_execution_summary`, but it does not store full model reasoning.

`agent_task_statuses` stores the task transition history. A single task can therefore show that it moved from `queued` to `processing` and then to `completed` or `failed`.

`user_responses` stores user decisions: approve, correct, or reject.

`git_workflow_payloads` stores the branch/commit/pull-request payload prepared after code generation. In Sprint 4 it also stores the GitFlame apply result: `commit_sha`, `pull_request_id`, `pull_request_url`, `apply_error`, and `applied_at`. File-level operations are stored in `generated_files`.

## GitFlame Integration

`gitflame_connections` stores the repository connection created in CodePilot when the user enters a repository URL and access token. The token is stored as encrypted ciphertext, not plaintext. `token_last4` supports safe UI display, and `token_status` records whether the token is currently considered active, invalid, or revoked.

`gitflame_webhooks` stores the webhook callback URL owned by CodePilot, the hashed webhook secret used for request verification, the subscribed event names, and optional GitFlame-side registration id. GitFlame stores and calls this URL; CodePilot owns the endpoint.

`gitflame_webhook_events` stores inbound webhook deliveries in a generic event log. The service does not try to force every GitFlame event into one external id column. Instead, it stores `event_type`, `action`, optional `delivery_id`, repository external id, optional issue-session link, raw payload JSON, processing status, and error JSON.

For the agreed Sprint 4 flow, webhooks are required for GitFlame-side issue events. The user creates or updates issues in GitFlame, and GitFlame calls CodePilot. Config creation, approve/correct/reject/apply, and recommendation analysis are initiated inside CodePilot and therefore use normal backend endpoints plus outgoing GitFlame API calls.

`repository_snapshots` stores fetch metadata for one repository analysis context: repository, connection, ref, commit SHA, `.ai.yml` config snapshot, file count, status, and fetch timestamp. `repository_snapshot_files` stores the file paths and content hashes included in that exact snapshot. This makes the analysis context reproducible even when the repository changes later.

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
- an agent task can store its current status and transition history;
- a recommendation run can store retention data;
- GitFlame integration metadata can record repository connections, webhook events, repository snapshots, and pull request application results.
