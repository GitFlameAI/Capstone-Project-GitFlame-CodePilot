# README section to add

Your repository's `README.md` was not included in the archive I was given, so I
did not rewrite it. Paste the section below into it (a good place is right after
the run instructions) and delete this file.

---

## Running the stack

```bash
git submodule update --init
cp .env.example .env    # set GITFLAME_CREDENTIAL_KEY and OPENAI_*
make up                 # equivalent to docker compose up -d --build
```

One compose file starts everything: frontend, backend, agent worker, Agent
Engine, recommendation service, CodeRAG and both databases. `make help` lists the
other shortcuts.

## Monitoring and operations

Every service writes structured JSON logs to stdout and exposes Prometheus
metrics. One request is traceable end to end by its `request_id`, which is
returned in the `X-Request-ID` response header and propagated to every internal
service.

| What | Where |
|---|---|
| Operations screen | `http://<host>/ops` (requires a session) |
| Build identity | `GET /api/version` |
| Readiness | `GET /api/ready` |
| Metrics | `/metrics` on each service, internal network only |
| Runbook | [docs/OPERATIONS.md](docs/OPERATIONS.md) |
| Metric and log reference | [docs/OBSERVABILITY.md](docs/OBSERVABILITY.md) |

Trace one user action across all services:

```bash
docker compose logs --since 1h | grep '<request_id>'
```

Optional monitoring stack (Prometheus + Alertmanager + Grafana), off by default:

```bash
make observability
```

Grafana on `:3000`, Prometheus on `:9090`. If you already run Prometheus, do not
deploy this: add the four scrape targets and copy
`infra/observability/alerts.yml` into your rules — see
[docs/OBSERVABILITY.md](docs/OBSERVABILITY.md#connecting-your-own-prometheus).

Build with version information so `/version` is meaningful:

```bash
make build
```

## Repository indexing is asynchronous

Connecting a repository returns immediately; CodeRAG indexing runs in the
background. The first plan or recommendation request waits for that job to
finish (bounded by `RAG_INDEX_WAIT_TIMEOUT_SECONDS`), so users never sit on a
blank connect screen. Progress is available at
`GET /api/integrations/gitflame/connections/{id}/index`.
