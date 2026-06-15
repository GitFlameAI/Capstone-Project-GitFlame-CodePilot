# Backend Database Schema

This folder contains the first PostgreSQL schema files for the GitFlame CodePilot backend storage layer.

`schema.sql` creates the Sprint 1 tables:

- `repositories`
- `ai_configs`
- `issue_sessions`
- `generated_plans`
- `user_responses`
- `recommendation_runs`
- `recommendations`
- `recommendation_statuses`

The schema uses PostgreSQL-friendly types:

- `UUID` for identifiers
- `JSONB` for parsed `.yml` configuration
- `TIMESTAMPTZ` for timestamps
- `TEXT CHECK (...)` constraints for MVP status values

After the PostgreSQL container is running, the schema can be applied with:

```bash
psql postgresql://gitflame:gitflame@localhost:5432/gitflame_codepilot -f backend/db/schema.sql
```

The next verification step is to insert sample issue workflow and recommendation records, then confirm them with `SELECT` queries or a pgAdmin screenshot.
