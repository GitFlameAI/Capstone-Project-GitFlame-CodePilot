# Backend Database

The backend uses PostgreSQL as the main storage layer for Sprint 3. Runtime state is no longer expected to live only in `MemoryStore`.

## Files

- `migrations/initial_schema.sql` creates the PostgreSQL schema.
- `migrations/003_sprint3_code_generation.sql` extends workflow/task constraints for `code_generation`.
- `verification.sql` inserts sample data and checks that issue workflow state, plan revisions, agent task status, and recommendation retention are stored correctly.

## Schema Scope

The migration creates:

- `repositories`
- `ai_configs`
- `issue_sessions`
- `generated_plans`
- `plan_revisions`
- `agent_tasks`
- `agent_task_statuses`
- `user_responses`
- `recommendation_runs`
- `recommendations`
- `recommendation_statuses`

`issue_sessions` stores the GitFlame issue workflow state. Its `git_workflow_json` field stores the Sprint 3 generated-files contract with file operations and branch/commit/PR metadata. `generated_plans` stores the current plan for a session. `plan_revisions` stores the plan history, including correction feedback. `agent_tasks` stores the current Agent Engine task status, including `initial_plan`, `plan_revision`, and `code_generation`, while `agent_task_statuses` stores the transition history for `queued`, `processing`, `completed`, and `failed`.

Recommendation retention is stored on `recommendation_runs` with `retention_days` and `expires_at`. The backend takes `retention_days` from the validated `.yml` configuration; it is not chosen by the database.

## Docker Initialization

`docker-compose.yml` mounts the migration into the PostgreSQL container:

```yaml
./backend/db/migrations/initial_schema.sql:/docker-entrypoint-initdb.d/initial_schema.sql:ro
```

PostgreSQL applies this file when a new database volume is created.

If the `postgres_data` volume already exists, PostgreSQL will not re-run the initialization file automatically. For a clean local database, recreate the volume:

```bash
docker compose down -v
docker compose up --build
```

## Manual Verification

After the database is running, run:

```bash
psql postgresql://gitflame:gitflame@localhost:5432/gitflame_codepilot -f backend/db/verification.sql
```

Expected result:

- the issue workflow query returns a saved issue session, generated plan, revision, and completed agent task;
- the recommendation query returns a saved retention period and a future expiration timestamp.
