# CodePilot Agent Engine Architecture

## Project Context

GitFlame CodePilot supports two product scenarios:

1. Issue-to-plan and future code generation.
2. Repository recommendations.

The current Agent Engine is designed around the primary autogen flow:

```text
issue
    -> plan.md
    -> approve / correct / reject
    -> future code generation
    -> GitFlame branch and pull request
```

The Agent Engine handles AI inference and repository context analysis. It does not
own GitFlame integration, permanent workflow state, user interface, branch creation,
or pull-request creation.

The user does not select the model. Model ID, quantization, runtime, and credentials
are operator settings. The plan format is always Markdown.

## SERGE Component Reuse

| Component | Decision | CodePilot usage |
| --- | --- | --- |
| OpenAI-compatible LLM Client | Reuse | Model calls, streaming, retries, timeouts, tool calling, reasoning output, and usage metrics. |
| Prompt System | Adapt | Replace PR review prompts with issue-to-plan and later plan-to-code prompts. |
| Agent Loop | Adapt | Return `plan.md` or generated files instead of a PR review. |
| Read-only Repository Tools | Adapt | Keep `read_file`, `list_dir`, and `grep`; add external RAG search. |
| Repository Clone Cache | Reference / Adapt | Use only if GitFlame provides Git or archive access. |
| Context Script | Reference | External RAG provides additional context; repository-defined scripts are not executed. |
| Context Compression | Reuse | Reduce oversized issue and repository context. |
| Sandbox | Reuse | Isolate tools and remove credentials from their environment. |
| Prompt Injection Protection | Adapt | Protect issue text, YAML, repository files, comments, and RAG output. |
| Code Generation Task Engine | Adapt | Return generated files to the Go backend instead of publishing to GitHub. |
| Patch Processing | Reference / Minimal Adapt | Reuse path and change validation; skip GitHub patch publishing. |
| GitHub Client and Authentication | Reference | Replace with GitFlame API and authentication contracts. |
| Triggers | Adapt | Reimplement event validation and idempotency for GitFlame. |
| GitHub Action Runner | Skip | CodePilot runs as an external service. |
| Configuration | Adapt | Separate operator settings from user-controlled repository YAML. |
| Storage | Adapt | Use PostgreSQL through the Go backend instead of SERGE SQLite. |
| Queue and Job States | Adapt | Use Redis and persisted task states. |
| SERGE Web Applications | Reference | The existing Vue frontend handles user interaction. |
| Docker and Tests | Adapt | Build and verify CodePilot-specific Agent Engine services. |

## Architecture

```text
GitFlame
    |
    v
Go Backend
    |-- PostgreSQL
    |-- Redis Queue
    |
    v
Agent Worker
    |
    v
SERGE-based Agent Engine
    |-- Prompt Builder
    |-- Agent Loop
    |-- OpenAI-compatible LLM Client
    |-- Context Compression
    |-- Sandbox
    |-- read_file / list_dir / grep
    |-- search_repository
    |
    +--> External RAG API
    `--> Open-source Model Endpoint
```

## Service Responsibilities

### GitFlame

- sends issue, repository, and user-action events;
- provides repository files or repository access information;
- stores final branches, commits, and pull requests;
- displays generated plans and workflow statuses.

### Go Backend

- authenticates GitFlame requests;
- validates issue data and repository YAML;
- creates idempotent Agent Engine tasks;
- publishes tasks to Redis;
- stores task states and generated plans in PostgreSQL;
- exposes task status and approve/correct/reject endpoints;
- returns structured results to GitFlame.

### Agent Worker

- consumes queued tasks;
- calls the Agent Engine;
- updates `queued`, `processing`, `completed`, or `failed` state;
- applies timeout and retry policy;
- limits concurrent GPU tasks.

### Agent Engine

- builds the model conversation;
- runs the adapted SERGE Agent Loop;
- inspects repository context through bounded tools and RAG;
- calls the selected model through an OpenAI-compatible endpoint;
- validates the generated `plan.md`;
- returns the plan and usage metadata to the Go backend;
- remains stateless between completed tasks.

### External RAG

- retrieves issue-relevant repository snippets;
- returns file paths, line ranges, scores, and content;
- does not control workflow state or GitFlame actions.

## Workflow State

The Go backend and PostgreSQL own the authoritative state:

```text
queued -> processing -> completed
                     `-> failed
```

After successful plan generation, the issue workflow continues independently:

```text
plan_generated -> approved
               -> correction_requested -> plan_generated
               `-> rejected
```

Redis transports tasks but is not the source of truth. The Agent Engine receives one
task, returns one result, and does not keep permanent session state.

## POST /v1/plans/generate

```http
POST /v1/plans/generate
Content-Type: application/json
```

Request:

```json
{
  "request_id": "task-123",
  "issue": {
    "id": "issue-42",
    "title": "Add token expiration validation",
    "body": "Validate token expiration before authentication."
  },
  "repository": {
    "id": "repo-7",
    "default_branch": "main",
    "commit_sha": "abc123"
  },
  "configuration": {
    "include": ["internal/**"],
    "exclude": ["vendor/**"],
    "max_files": 20,
    "max_snippets_per_file": 3
  },
  "repository_files": [
    {
      "path": "internal/auth/token.go",
      "content": "package auth"
    }
  ],
  "previous_plan": null,
  "correction_feedback": null
}
```

Response:

```json
{
  "request_id": "task-123",
  "status": "completed",
  "plan_markdown": "# Implementation Plan\n...",
  "relevant_files": [
    {
      "path": "internal/auth/token.go",
      "reason": "Contains token validation logic"
    }
  ],
  "model": "selected-model-id",
  "usage": {
    "prompt_tokens": 8200,
    "completion_tokens": 1200,
    "tool_calls": 4
  }
}
```

## Agent Loop

```text
Build initial prompt
    -> call model
    -> receive tool calls or final answer
    -> validate tool calls
    -> execute repository tools or RAG
    -> append bounded tool results
    -> call model again
    -> validate final plan.md
```

The loop stops when:

- a valid `plan.md` is produced;
- the maximum number of steps or tool calls is reached;
- the task timeout expires;
- the model, RAG, or repository source fails permanently.

Initial limits:

```text
AGENT_MAX_STEPS=12
AGENT_MAX_TOOL_CALLS=20
AGENT_TIMEOUT_SECONDS=600
MAX_TOOL_OUTPUT_CHARS=8192
MODEL_CONTEXT_LIMIT=65536
WORKER_CONCURRENCY=1
```

## Repository Tools And RAG

### read_file

```text
read_file(path, start_line, end_line)
```

Reads a bounded line range from an allowed repository file.

### list_dir

```text
list_dir(path, max_entries)
```

Lists files and directories inside the repository workspace.

### grep

```text
grep(pattern, path, max_results)
```

Returns bounded `path:line:text` search results.

### search_repository

```text
search_repository(query, top_k, filters)
```

Calls the external RAG API and returns:

```json
{
  "results": [
    {
      "path": "internal/auth/token.go",
      "start_line": 30,
      "end_line": 75,
      "score": 0.91,
      "content": "..."
    }
  ]
}
```

RAG results are treated as untrusted input. Paths and result sizes are validated
before content is added to the model conversation.

RAG finds semantically relevant candidates. `read_file`, `list_dir`, and `grep`
verify exact repository content. The Agent Loop may combine both mechanisms but must
remain within configured step, tool-call, context, and timeout limits.

## Security Limits

- Issue text, YAML, repository files, comments, tool output, and RAG output are untrusted.
- Repository tools are read-only and confined to the repository workspace.
- Path traversal and access to `.git`, credentials, binaries, and excluded files are blocked.
- GitFlame tokens, model keys, database credentials, and webhook secrets are unavailable to tools.
- Tool output size, file ranges, Agent Loop steps, tool calls, context size, and task duration are limited.
- Full private reasoning traces are not returned to users or stored in PostgreSQL.
- The Agent Engine cannot create branches, commits, or pull requests directly.
- Code generation cannot start before the Go backend records user approval.
- Final model output is validated before storage or publication.

## Current Decisions And Open Interfaces

The following decisions are fixed:

- SERGE is reused only as the basis of the Python Agent Engine.
- Go remains the integration and orchestration backend.
- PostgreSQL replaces SERGE SQLite storage.
- Redis is used for queued Agent Engine work.
- External RAG is accessed through `search_repository`.
- Repository tools are read-only.
- GitHub-specific SERGE components are not part of the runtime flow.
- Code generation cannot run before explicit user approval.

The following interfaces remain implementation-dependent:

- repository delivery through provided files, GitFlame API, archive, or clone cache;
- final open-source model and quantized checkpoint;
- exact RAG authentication and filtering contract;
- generated-file limits agreed with GitFlame;
- callback or polling mechanism used to return completed task results.

## Apache 2.0 Attribution

SERGE is licensed under the Apache License 2.0. Reused or adapted code must:

1. Keep a copy of the Apache 2.0 license.
2. Be listed in `THIRD_PARTY_NOTICES.md`.
3. Preserve relevant copyright and notice text.
4. Mark substantial CodePilot modifications.
5. Reference the original repository: <https://github.com/huggingface/serge>.

Recommended repository files:

```text
LICENSES/SERGE-APACHE-2.0.txt
THIRD_PARTY_NOTICES.md
```
