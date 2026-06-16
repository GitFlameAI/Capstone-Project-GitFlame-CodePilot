# Storage Verification

This note records a basic PostgreSQL storage verification for the Sprint 1 database schema.

## Test Setup

For the local check, a separate PostgreSQL test container was started from an already available local image:

```bash
docker run --name gitflame-codepilot-postgres \
  -e POSTGRES_USER=gitflame \
  -e POSTGRES_PASSWORD=gitflame \
  -e POSTGRES_DB=gitflame_codepilot \
  -p 55432:5432 \
  -d postgres:15
```

The schema was applied with:

```bash
psql -h localhost -p 55432 -U gitflame -d gitflame_codepilot -f backend/db/schema.sql
```

The verification data was inserted and selected with:

```bash
psql -h localhost -p 55432 -U gitflame -d gitflame_codepilot -f backend/db/verification.sql
```

## Tables Created

The database contains the expected Sprint 1 tables:

```text
ai_configs
generated_plans
issue_sessions
recommendation_runs
recommendation_statuses
recommendations
repositories
user_responses
```

## Verification Result

The first verification query checks the issue workflow storage. It joins repository data, issue session, generated plan, and user response.

```text
verification_case: issue workflow saved
repository: gitflame/codepilot-demo
issue_title: Add recommendation widget
session_status: plan_generated
response_type: approve
```

The second verification query checks the recommendation workflow storage. It joins repository data, recommendation run, and recommendation card.

```text
verification_case: recommendation workflow saved
repository: gitflame/codepilot-demo
run_status: completed
file_path: backend/app/api/routes/recommendations.go
severity: medium
current_status: open
```

## Conclusion

The PostgreSQL schema can be applied successfully, and sample records for both required storage flows can be inserted and retrieved:

- issue session with generated plan and user response;
- recommendation run with recommendation card and status data.

This is enough for a basic Sprint 1 storage verification. The full backend persistence layer can use the same tables in the next implementation step.
