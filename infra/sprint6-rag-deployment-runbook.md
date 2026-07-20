# Sprint 6 RAG Deployment Runbook

Sprint 6 adds the CodeRAG HTTP service to the GitFlame CodePilot Docker Compose
deployment. CodeRAG is connected as a Git submodule, so the main repository stores a
pointer to the external RAG repository instead of copying its source files.

## Submodule

CodeRAG is mounted under:

```text
coderag
```

Clone the project with submodules:

```bash
git clone --recurse-submodules https://github.com/GitFlameAI/Capstone-Project-GitFlame-CodePilot.git
```

If the repository was already cloned, initialize the submodule with:

```bash
git submodule update --init --recursive
```

The submodule tracks:

```text
https://github.com/GitFlameAI/GitFlame-CodeRAG.git
branch: sprint-6/http-rag-service
```

On Windows, enable long paths if submodule checkout fails on benchmark dataset files:

```bash
git config core.longpaths true
```

## Services

The Sprint 6 Compose stack includes:

```text
frontend
backend
database
redis
ml-service
agent-engine
agent-worker
recommendation-service
coderag-database
rag-service
```

`coderag-database` uses `pgvector/pgvector:pg16` and stores CodeRAG indexing data in
its own volume. It is separate from the main CodePilot PostgreSQL database.

`rag-service` builds from the CodeRAG submodule and exposes:

```text
GET  /health
POST /search
```

The Agent Engine receives the internal Docker URL:

```text
RAG_BASE_URL=http://rag-service:8004
```

Host-side checks use:

```text
http://localhost:8004
```

## Environment

Create local environment values from the root template:

```bash
cp .env.example .env
```

Important Sprint 6 RAG variables:

```text
RAG_PORT=8004
CODERAG_DATABASE_PORT=5433
RAG_BASE_URL=http://localhost:8004
RAG_API_KEY=
EMBEDDING_MODEL=jinaai/jina-embeddings-v2-base-code
RAG_USE_DENSE=true
RAG_CANDIDATE_TOP_K=50
RAG_MIN_RELEVANCE_SCORE=
RAG_MAX_CONTEXT_FILES=20
RAG_MAX_CHUNKS_PER_FILE=3
RAG_MAX_CONTEXT_TOKENS=12000
RAG_DEDUPLICATE_OVERLAPS=true
RAG_OVERLAP_THRESHOLD=0.8
MODEL_CONTEXT_LIMIT=32768
```

Use the same private `RAG_API_KEY` for CodeRAG and Agent Engine when RAG auth is
enabled. Keep it empty for local unauthenticated smoke tests only. Do not commit real
keys.

## Startup

Run the complete stack from the repository root:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  up -d --build
```

Validate the combined Compose file:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  config --quiet
```

## Clean Reproducibility Check

Use this only when local PostgreSQL, Redis, and CodeRAG data can be discarded:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  down -v

docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  up -d --build
```

The main CodePilot database is initialized from:

```text
backend/db/migrations/initial_schema.sql
```

The CodeRAG database is initialized from:

```text
coderag/migrations/001_initial.sql
```

## Health Checks

Check container states:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  ps
```

HTTP checks:

```bash
curl http://localhost/
curl http://localhost:8000/ready
curl http://localhost:8002/ready
curl http://localhost:8003/ready
curl http://localhost:8004/health
```

Redis:

```bash
docker compose \
  -f docker-compose.yml \
  -f backend/deploy/docker-compose.sprint2.override.yml \
  exec redis redis-cli ping
```

Expected response:

```text
PONG
```

## RAG Search Smoke Test

CodeRAG search requires an indexed repository/revision. The service does not clone or
index a repository during a search request.

When an indexed revision exists, test the contract with:

```bash
curl --fail http://localhost:8004/search \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $RAG_API_KEY" \
  -d '{
    "query": "where is authentication validated?",
    "top_k": 5,
    "filters": {
      "repository_id": "owner/repository",
      "commit_sha": "indexed-commit-sha",
      "include": ["**/*"],
      "exclude": ["node_modules/**", ".git/**"]
    }
  }'
```

Expected response shape:

```json
{
  "results": [
    {
      "path": "src/example.py",
      "start_line": 1,
      "end_line": 10,
      "score": 0.91,
      "content": "..."
    }
  ]
}
```

An unknown repository or revision should return:

```json
{"results": []}
```

## Report Evidence

Provide Roma with:

```text
docker compose ps output
backend /ready output
agent-engine /ready output
recommendation-service /ready output
rag-service /health output
Redis PONG output
notes about indexed or empty RAG search behavior
VM/demo URL if deployed
```
