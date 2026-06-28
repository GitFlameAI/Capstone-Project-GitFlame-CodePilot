# Redis Agent Task Contract

Redis Streams transports Agent Engine work. PostgreSQL remains the authoritative source for sessions, plans, revisions, and task status.

## Streams and consumer group

```text
task stream:        gitflame:agent:tasks
dead-letter stream: gitflame:agent:tasks:dead-letter
consumer group:     gitflame-agent-workers
```

The names are configurable through `AGENT_QUEUE_NAME` and `AGENT_CONSUMER_GROUP`.

## Job message

Each task stream entry contains one `job` field with JSON:

```json
{
  "task_id": "uuid",
  "session_id": "uuid",
  "type": "initial_plan",
  "attempt": 1,
  "request": {
    "request_id": "uuid",
    "issue": {},
    "repository": {},
    "configuration_yaml": "version: 1\nanalysis:\n  include: [internal/**]\n",
    "repository_files": [],
    "previous_plan": null,
    "correction_feedback": null
  }
}
```

For Sprint 3, `type` is `initial_plan`, `plan_revision`, or `code_generation`. Plan tasks use
`request`; code-generation tasks use `code_generation_request`:

```json
{
  "task_id": "uuid",
  "session_id": "uuid",
  "type": "code_generation",
  "attempt": 1,
  "code_generation_request": {
    "request_id": "uuid",
    "issue": {},
    "repository": {},
    "approved_plan_markdown": "# Implementation Plan\n...",
    "configuration_yaml": "version: 1\n",
    "repository_files": []
  }
}
```

`request_id` must equal `task_id`.

## Delivery rules

1. Backend persists the session and `queued` task in PostgreSQL.
2. Backend publishes the job with `XADD`.
3. Worker consumes with `XREADGROUP` and changes the persisted state to `processing`.
4. Worker calls Agent Engine, validates `plan.md` or generated file operations, and persists either `completed` or `failed`.
5. Worker acknowledges successful or permanently failed messages with `XACK`.
6. HTTP `502`, `503`, and `504` errors are retried up to `WORKER_MAX_RETRIES`.
7. Permanent failures and exhausted retries are copied to the dead-letter stream before acknowledgement.
8. `AGENT_QUEUE_MAX_LENGTH` rejects new work when the configured queue limit is reached.

No model reasoning is placed in Redis or PostgreSQL. Only the bounded usage and tool execution summary are retained.
