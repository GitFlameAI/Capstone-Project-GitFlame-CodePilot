# Infrastructure

This folder stores Sprint 1 infrastructure notes. The runnable Docker setup is defined in the root `docker-compose.yml`.

## Services

The current Compose setup starts:

- `backend`: Go backend API on port `8000`.
- `ml-service`: mock ML service on port `8001`.
- `database`: PostgreSQL 16 on port `5432`.

The backend receives the internal Compose database connection string through:

```text
DATABASE_URL=postgresql://gitflame:gitflame@database:5432/gitflame_codepilot
```

For local host tools such as `psql` or pgAdmin, use:

```text
DATABASE_URL=postgresql://gitflame:gitflame@localhost:5432/gitflame_codepilot
```

## Run With Docker Compose

From the repository root:

```bash
docker compose up --build
```

After startup, verify the backend:

```text
http://localhost:8000/health
```

Open Swagger/OpenAPI docs:

```text
http://localhost:8000/swagger/
```

## Apply Database Schema

Automatic migrations are not configured in Sprint 1. Apply the PostgreSQL schema manually after the database container is running:

```bash
psql postgresql://gitflame:gitflame@localhost:5432/gitflame_codepilot -f backend/db/schema.sql
```

Optional storage verification:

```bash
psql postgresql://gitflame:gitflame@localhost:5432/gitflame_codepilot -f backend/db/verification.sql
```

## Sprint 1 Notes

- The Git workflow is implemented as a mock service interface.
- The mock Git workflow returns a branch name, pull request URL, reviewer, and provider.
- The `.yml` config service validates Sprint 1 branch rules, include/exclude patterns, approval commands, and reviewer policy.

