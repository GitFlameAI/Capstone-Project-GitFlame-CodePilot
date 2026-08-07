# Operations runbook

This document is written for whoever maintains GitFlame CodePilot after the
original team is gone. It assumes no prior knowledge of the codebase.

If you are looking for *what* is measured rather than *what to do*, read
[OBSERVABILITY.md](OBSERVABILITY.md).

---

## 1. What this system is made of

| Container | What it does | Fails how |
|---|---|---|
| `frontend` | nginx serving the Vue app and proxying `/api/` to the backend | users see a blank page or 502 |
| `backend` (Go) | orchestration, sessions, GitFlame integration, task queueing | everything stops |
| `agent-worker` (Go) | consumes agent tasks from Redis and runs them | plans and code generation stay `queued` forever |
| `agent-engine` (Python) | LLM planning and code generation | tasks fail with `agent_engine_unreachable` |
| `recommendation-service` (Python) | repository analysis and recommendations | recommendation runs fail |
| `rag-service` (CodeRAG) | repository indexing and semantic search | plans get worse, indexing fails |
| `database` (PostgreSQL) | all persistent state | everything stops |
| `coderag-database` (PostgreSQL + pgvector) | embeddings | indexing and search fail |
| `redis` | task queue and dead-letter stream | tasks cannot be queued or picked up |

The backend and the worker are the same binary image with different entrypoints.

---

## 2. First five minutes of any incident

Run these in order. Each one narrows the problem down.

```bash
# 1. What is actually running, and what version?
docker compose ps
curl -s localhost:8000/version

# 2. Does the service consider itself healthy?
curl -s localhost:8000/ready | jq

# 3. The one screen that summarises everything (needs a browser session):
#    http://<host>/ops
#    or from the shell, with a session cookie:
curl -s --cookie "codepilot_session=<token>" localhost:8000/ops/status | jq

# 4. What do the logs say for the failing request?
docker compose logs --since 15m backend | grep '"level":"ERROR"'
```

Every log line is one JSON object and every request carries a `request_id`. If a
user reports a problem, ask them for it — it is returned in the `X-Request-ID`
response header — and then:

```bash
docker compose logs --since 1h | grep '<request_id>'
```

This returns the whole story of that action across the backend, the worker and
the Python services, in order.

---

## 3. Symptom → cause → fix

### "Plans never finish, tasks stay queued"

**Look at:** `/ops/status` → `queue.stream` is growing, `queue.pending` is 0.

**Cause:** the worker is not consuming. Either the container is down, or it
cannot reach Redis or Postgres.

**Fix:**
```bash
docker compose ps agent-worker
docker compose logs --tail 50 agent-worker
docker compose restart agent-worker
```
A worker that starts correctly logs `worker_started` with the stream name.

---

### "Tasks fail immediately with agent_engine_unreachable"

**Look at:** `/ops/status` → `dependencies` shows `agent_engine: down`.

**Cause:** the Agent Engine container is down, or the model endpoint behind it
(`OPENAI_BASE_URL`) is unreachable.

**Fix:**
```bash
docker compose logs --tail 100 agent-engine
curl -s localhost:8002/ready       # engine's own view of the model
```
If the engine is up but `/ready` reports the model as unavailable, the problem is
the model endpoint, not this system. Check `OPENAI_BASE_URL` and `OPENAI_API_KEY`
in `.env`.

---

### "Tasks fail with inference_timeout"

**Cause:** the model is answering, but too slowly for
`MODEL_REQUEST_TIMEOUT_SECONDS` / `AGENT_TIMEOUT_SECONDS`.

**Fix:** this is usually load on the model endpoint, not a bug. Confirm with
`codepilot_llm_request_duration_seconds` (or the Grafana panel "Model latency
p95"). Raising the timeouts only helps if the client polling timeout in the
frontend is raised too — prefer fixing the model endpoint.

---

### "The dead-letter count is not zero"

**Look at:** `/ops/dead-letter`.

**Meaning:** a task exhausted `WORKER_MAX_RETRIES` and was abandoned. **A user
request was lost** — nobody is retrying it automatically.

**Fix:** read the `error` field of each entry to find the root cause, fix that,
then ask the user to re-run the action. There is deliberately no "requeue"
button: replaying a task whose cause has not been fixed just burns model budget.

---

### "Connecting a repository is fast but plans take forever the first time"

**This is by design.** Repository indexing runs in the background so the user is
not blocked on the connect screen. The first plan request waits for that
indexing job to finish (up to `RAG_INDEX_WAIT_TIMEOUT_SECONDS`, default 600).

Check progress: `GET /integrations/gitflame/connections/{id}/index`, or the
"Indexing jobs running" tile on `/ops`.

If a request returns `503 rag_indexing_in_progress`, indexing exceeded the wait
budget. It keeps running; retrying the action a minute later normally succeeds.

---

### "Everything worked yesterday, today every GitFlame call fails"

**Look at:** `/ops/status` → `connections_by_token_status` has entries that are
not `active`.

**Cause:** the stored GitFlame access token expired or was revoked.

**Fix:** nothing on the server can repair this. The repository owner has to
reconnect the repository in the UI, which stores a fresh token.

---

### "The VM ran out of disk"

**Cause, historically:** unbounded container logs.

**Check:** `docker system df` and `du -sh /var/lib/docker/containers`.

Every service in every compose file now has `max-size: 10m, max-file: 3`, which
caps container logs at roughly 400 MB total. If a container was created before
that change, recreate it: `docker compose up -d --force-recreate <service>`.

Also check the Postgres volumes and CodeRAG embeddings, which grow with the
number of indexed repositories.

---

### "The service is up but /ready returns 503"

`/ready` fails if any dependency check fails. The response body names the
component. Work down the list in section 1: a failing `storage` check means
Postgres is unreachable **or the migrations were never applied**.

---

## 4. Routine procedures

### Deploying a new version

```bash
git pull
git submodule update --init
make up            # builds with the git commit stamped into /version
curl -s localhost:8000/version    # must show the commit you just built
```

`make up` is a wrapper around `docker compose up -d --build` with the build
arguments filled in. Plain `docker compose up -d --build` works too, but then
`/version` reports `unknown`.

If `/version` still shows the old commit, the image was not rebuilt.

### Applying database migrations

The consolidated schema in `backend/db/migrations/initial_schema.sql` is applied
automatically by Postgres on **first** start of an empty volume. For an existing
database, apply the numbered migrations manually, newest last:

```bash
docker compose exec -T database \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" < backend/db/migrations/006_observability_indexes.sql
```

The backend refuses to start against a database without the schema, which is
intentional: a silently empty database is worse than a failed boot.

### Turning the monitoring stack on

```bash
make observability
```

Grafana on `:3000` (admin / `GRAFANA_PASSWORD`), Prometheus on `:9090`. It is
off by default and costs roughly 400–600 MB of RAM.

To use an existing Prometheus instead, see
[OBSERVABILITY.md](OBSERVABILITY.md#connecting-your-own-prometheus).

### Rotating the credential key

`GITFLAME_CREDENTIAL_KEY` encrypts stored GitFlame tokens. Changing it makes
every stored token undecryptable, so every user has to reconnect. If you must
rotate it, bump `GITFLAME_CREDENTIAL_KEY_VERSION` at the same time and warn
users; there is no re-encryption tool.

---

## 5. Known limits worth writing down

- **One backend instance.** Background indexing jobs are tracked in the memory of
  the backend process. Running two backend replicas would let both index the same
  repository at once. Scaling out requires moving that registry into Postgres
  (`backend/internal/httpapi/repository_index_jobs.go` explains where).
- **One uvicorn worker per Python service.** Prometheus counters live in process
  memory; several workers would each report their own numbers and the totals
  would be wrong.
- **Shutdown is graceful but bounded.** The backend stops accepting connections,
  finishes in-flight requests and waits up to 30 s for background indexing; the
  worker lets the task it is currently running finish. A task that takes longer
  than the `stop_grace_period` in `docker-compose.yml` is killed, and because it
  was never acknowledged it stays in the Redis pending list and is not
  redelivered automatically. If that happens, the user has to re-run the action.
- **CodeRAG is a git submodule** (`coderag/`). Clone with
  `git clone --recurse-submodules`, or run `git submodule update --init` after
  cloning, otherwise the RAG image cannot be built.
- **One compose file.** `docker-compose.yml` is the whole stack;
  `docker-compose.observability.yml` adds the optional monitoring profile. The
  older three-file split (base + dependencies + `backend/deploy/full`) is gone,
  as is the mock `ml_service`, which advertised endpoints the backend never
  called.
