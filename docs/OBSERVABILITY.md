# Observability reference

What this service exposes, and how to plug it into your own monitoring. For
incident procedures see [OPERATIONS.md](OPERATIONS.md).

Three interfaces, all standard, so nothing here obliges you to adopt our tooling:

| Interface | Where | Format |
|---|---|---|
| Logs | container stdout | one JSON object per line |
| Metrics | `GET /metrics` on every service | Prometheus text exposition 0.0.4 |
| Health | `GET /health`, `GET /ready`, `GET /version` | JSON |

---

## Logs

Every service — Go and Python — writes the same shape:

```json
{"time":"2026-08-07T09:12:44.106Z","level":"INFO","msg":"http_request","service":"backend",
 "request_id":"9243288fcb8337ef2f920d1789592b4f","event":"http_request","method":"POST",
 "path":"/ai/issues/42/plan","status":202,"duration_ms":37}
```

Fields that are always present: `time`, `level`, `msg`, `service`. Fields that
are present whenever they are known: `request_id`, `event`, plus event-specific
keys.

### Correlation

The backend generates a request id for every inbound request, or reuses the
inbound `X-Request-ID` header, and returns it in the response. That id is
propagated as `X-Request-ID` to GitFlame, CodeRAG, the Agent Engine and the
recommendation service, and is carried through Redis into the worker. One grep
returns the full story of a user action:

```bash
docker compose logs --since 1h | grep '<request_id>'
```

For asynchronous work started from a request — background repository indexing,
webhook processing — the id of the originating request is kept, so the job is
greppable together with the action that started it.

### Event catalogue

| `event` | Service | Meaning |
|---|---|---|
| `http_request` | all | one inbound request; carries `method`, `path`, `route`, `status`, `duration_ms` |
| `http_panic` | backend | a handler panicked; carries the stack |
| `gitflame_http` | backend | one call to the GitFlame API |
| `gitflame_contents_update` / `gitflame_contents_delete` | backend | write to a repository, including retries on conflict |
| `gitflame_webhook` | backend | webhook received or processed |
| `rag_index` | backend | a repository was indexed |
| `rag_index_job` | backend | a background indexing job finished |
| `rag_index_job_retry_inline` | backend | a failed background job is being retried inside a request |
| `dependency_down` | backend | a readiness check failed during metric collection |
| `worker_started`, `metrics_server_started` | worker | startup |
| `shutdown_started`, `shutdown_complete` | backend, worker | graceful shutdown began / finished |
| `shutdown_background_work_abandoned` | backend | indexing jobs were still running when the shutdown budget ran out |
| `task_retry_scheduled` | worker | a task failed and was re-queued |
| `task_failed_permanently`, `task_dead_letter_failed` | worker | a task was abandoned |
| `server_started`, `startup_failed` | backend | startup |

### What is deliberately *not* logged

Prompts, model outputs, repository file contents, GitFlame tokens and session
tokens. If you add logging, keep this rule: an operator needs identifiers,
statuses and error codes, never user content. Error details from upstreams are
truncated to 300 characters.

### Levels and format

`LOG_LEVEL` (`debug`, `info`, `warn`, `error`; default `info`) and `LOG_FORMAT`
(`json` or `text`; default `json`). An unrecognised value falls back to the
default rather than failing to start. `text` is for local debugging only.

### Rotation

Every service in every compose file sets `max-size: 10m, max-file: 3` on the
json-file driver, capping container logs at roughly 400 MB in total. Without
that, JSON logs fill a small VM's disk in weeks.

---

## Metrics

Scrape targets:

| Service | Endpoint |
|---|---|
| backend | `backend:8000/metrics` |
| agent-worker | `agent-worker:9100/metrics` (`WORKER_METRICS_PORT`) |
| agent-engine | `agent-engine:8001/metrics` |
| recommendation-service | `recommendation-service:7860/metrics` |

CodeRAG is not instrumented (it lives in a separate repository). It is measured
from the caller side instead, as `codepilot_upstream_requests_total{upstream="rag"}`
and `codepilot_rag_requests_total`.

`/metrics` is **not** reachable from the outside: nginx returns 404 for
`/api/metrics` and no metrics port is published. Prometheus reaches it over the
Docker network.

### Catalogue

**HTTP (all services)**

| Metric | Type | Labels |
|---|---|---|
| `codepilot_http_requests_total` | counter | `method`, `route`, `status` |
| `codepilot_http_request_duration_seconds` | histogram | `method`, `route` |

`route` is the route template (`/ai/issues/{id}/plan`), never the concrete path.

**Agent pipeline (backend, worker)**

| Metric | Type | Labels |
|---|---|---|
| `codepilot_agent_tasks_executed_total` | counter | `task_type`, `outcome` |
| `codepilot_agent_task_duration_seconds` | histogram | `task_type` |
| `codepilot_agent_task_retries_total` | counter | `task_type` |
| `codepilot_agent_tasks_dead_lettered_total` | counter | `task_type` |
| `codepilot_agent_task_tokens_total` | counter | `task_type`, `kind` |
| `codepilot_agent_tasks` | gauge | `status` (last 24 h, from the database) |

**Queue (backend, worker)**

| Metric | Type | Labels |
|---|---|---|
| `codepilot_queue_depth` | gauge | `stream` (`tasks`, `dead_letter`) |
| `codepilot_queue_pending` | gauge | `stream` |

**Upstreams and dependencies (backend)**

| Metric | Type | Labels |
|---|---|---|
| `codepilot_upstream_requests_total` | counter | `upstream`, `outcome` |
| `codepilot_upstream_request_duration_seconds` | histogram | `upstream` |
| `codepilot_dependency_up` | gauge | `component` |
| `codepilot_gitflame_connections` | gauge | `token_status` |

**Repository indexing (backend)**

| Metric | Type | Labels |
|---|---|---|
| `codepilot_repository_index_jobs_total` | counter | `outcome` |
| `codepilot_repository_index_duration_seconds` | histogram | — |
| `codepilot_repository_index_jobs_running` | gauge | — |

**Model and analysis (agent-engine, recommendation-service)**

| Metric | Type | Labels |
|---|---|---|
| `codepilot_llm_requests_total` | counter | `model`, `operation`, `outcome` |
| `codepilot_llm_request_duration_seconds` | histogram | `model`, `operation` |
| `codepilot_llm_tokens_total` | counter | `model`, `operation`, `kind` |
| `codepilot_rag_requests_total` | counter | `operation`, `outcome` |
| `codepilot_rag_request_duration_seconds` | histogram | `operation` |
| `codepilot_analyzer_runs_total` | counter | `tool`, `outcome` |
| `codepilot_analyzer_duration_seconds` | histogram | `tool` |
| `codepilot_agent_tool_calls_total` | counter | `tool` |

**Meta (all services)**

| Metric | Type | Labels |
|---|---|---|
| `codepilot_build_info` | gauge (always 1) | `service`, `version`, `commit`, `go_version` |
| `codepilot_collector_errors_total` | counter | `source` |

`operation` is one of `plan`, `code_generation`, `recommendations`, `unknown`.
`outcome` is one of `success`, `failure`, `timeout`, `unavailable`,
`empty_output`, `client_error`, `server_error`, `unreachable`.

### Label discipline

Repository ids, issue ids, task ids, file paths and user names are **never**
metric labels: they are unbounded and would eventually take Prometheus down. They
belong in the logs, where the request id ties them together. Keep this rule when
adding metrics.

### Implementation note

The Go services expose metrics through a small in-house implementation of the
Prometheus text format (`backend/internal/observability/metrics.go`) rather than
`prometheus/client_golang`. This keeps `go.mod` free of five transitive
dependencies, two of which are hosted outside GitHub, so the project can be
rebuilt from a clean checkout without surprises. The trade-off is no Go runtime
metrics (use container metrics instead) and no exemplars. The output is verified
against `promtool` and the unit tests in `metrics_test.go`.

The Python services use `prometheus_client`, which is why they must run with a
single uvicorn worker: counters live in process memory.

---

## Health endpoints

| Endpoint | Answers |
|---|---|
| `GET /health` | the process is alive |
| `GET /ready` | every dependency answered; 503 with the failing component otherwise |
| `GET /version` | version, git commit, build time, Go version |

`/version` is populated at link time from build arguments. Pass them when
building, otherwise it honestly reports `unknown`:

```bash
make build     # or: docker compose build --build-arg GIT_COMMIT=$(git rev-parse --short HEAD)
```

---

## Operational endpoints

Read-only, session-authenticated, and safe to expose to whoever operates the
service. They return counts, identifiers and error codes only.

| Endpoint | Returns |
|---|---|
| `GET /ops/status` | build identity, dependency health, queue depth, tasks in the last 24 h, connections by token status, running indexing jobs |
| `GET /ops/tasks?limit=&status=` | recent agent tasks, newest first |
| `GET /ops/dead-letter` | tasks that exhausted their retries |

The `/ops` screen in the frontend renders exactly these three responses.

---

## Connecting your own Prometheus

Nothing in this repository needs to be deployed for this. Add the scrape targets
and copy the rules:

```yaml
scrape_configs:
  - job_name: codepilot-backend
    static_configs: [{ targets: ["backend:8000"] }]
  - job_name: codepilot-agent-worker
    static_configs: [{ targets: ["agent-worker:9100"] }]
  - job_name: codepilot-agent-engine
    static_configs: [{ targets: ["agent-engine:8001"] }]
  - job_name: codepilot-recommendation-service
    static_configs: [{ targets: ["recommendation-service:7860"] }]
```

Then copy `infra/observability/alerts.yml` into your `rule_files`. The rules use
only the metrics listed above and carry no dependency on our Prometheus
configuration.

Verify the rules before shipping them:

```bash
docker run --rm -v "$PWD/infra/observability:/rules" prom/prometheus:v2.55.1 \
  promtool check rules /rules/alerts.yml

docker run --rm -v "$PWD/infra/observability:/rules" prom/prometheus:v2.55.1 \
  promtool test rules /rules/tests/alerts_test.yml
```

### Shipped alerts

| Alert | Severity | Fires when |
|---|---|---|
| `CodePilotServiceDown` | critical | a service stops answering scrapes for 2 min |
| `CodePilotDependencyDown` | warning | a readiness check fails for 5 min |
| `CodePilotHTTP5xxRate` | warning | >5% of requests are 5xx for 10 min |
| `CodePilotHighTaskFailureRate` | warning | >20% of agent tasks fail for 15 min |
| `CodePilotDeadLetterGrowing` | critical | any task is dead-lettered |
| `CodePilotQueueBacklog` | warning | >50 tasks waiting for 10 min |
| `CodePilotLLMLatencyHigh` | warning | model p95 above 5 min for 15 min |
| `CodePilotRepositoryIndexingFailing` | warning | >2 indexing failures in an hour |
| `CodePilotGitFlameTokensNeedReauth` | warning | a stored token is invalid for 30 min |
| `CodePilotMetricsCollectorFailing` | warning | gauge collection keeps failing |

Alertmanager ships with a null receiver: it starts cleanly and delivers nothing
until you uncomment the webhook receiver in
`infra/observability/alertmanager.yml` and set a real URL. That is deliberate —
Alertmanager does not expand environment variables in its config, and a config
that fails to parse would take the profile down.

---

## Environment variables

| Variable | Default | Effect |
|---|---|---|
| `LOG_LEVEL` | `info` | log verbosity, all services |
| `LOG_FORMAT` | `json` | `json` or `text`, Go and Python services |
| `WORKER_METRICS_PORT` | `9100` | the worker's metrics/health port |
| `RAG_INDEX_TIMEOUT_SECONDS` | `600` | bounds one background indexing job |
| `RAG_INDEX_WAIT_TIMEOUT_SECONDS` | `600` | how long a request waits for a running indexing job before returning `503 rag_indexing_in_progress` |
| `PROMETHEUS_PORT` | `9090` | observability profile only |
| `PROMETHEUS_RETENTION` | `15d` | observability profile only |
| `GRAFANA_PORT` | `3000` | observability profile only |
| `GRAFANA_PASSWORD` | `admin` | observability profile only — change it if the port is reachable |
| `ALERTMANAGER_PORT` | `9093` | observability profile only |
