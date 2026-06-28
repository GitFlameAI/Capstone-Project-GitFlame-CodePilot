# Backend Version 3 - Weekly Report Material

## New and changed endpoints

| Endpoint | Change in Version 3 |
| --- | --- |
| `POST /ai/issues/{id}/approve` | Approves the latest generated plan and enqueues a separate `code_generation` Agent Engine task. Returns `202 Accepted`, `task_id`, `status_url`, and the prepared GitFlame branch/commit/PR payload. |
| `GET /ai/issues/{id}/code-generation` | New endpoint for the latest code-generation task status. Completed tasks include the generated-files contract. |
| `GET /ai/tasks/{taskId}` | Extended to include `type=code_generation` and `generated_files_contract` for code-generation tasks. |
| `GET /openapi.json` | Updated with Sprint 3 task type, code-generation endpoint, generated file operation schema, and generated-files contract schema. |

## Backend flow

```text
Validated plan.md
      |
      v
POST /ai/issues/{id}/approve
      |
      v
code_generation task queued
      |
      v
Agent Worker -> Agent Engine POST /v1/files/generate
      |
      v
backend generated-files validation
      |
      v
Generated Files Contract + Branch / Commit / PR Payload
```

The backend sends the approved plan, raw YAML configuration, issue metadata, repository metadata, and repository files to Agent Engine. Agent Engine returns only file operations. The backend remains responsible for preparing the GitFlame payload and does not apply files directly.

## Generated-files validation

The backend rejects:

- unsafe paths, including absolute paths, parent traversal, and `.git` paths;
- unsupported actions outside `create`, `modify`, and `delete`;
- duplicate generated file paths;
- empty `content` for `create` and `modify`;
- `delete` operations that include `content` or `diff`;
- `modify` or `delete` operations for files absent from supplied repository context;
- generated operations without an explanation.

## Storage changes

Sprint 3 reuses `issue_sessions.git_workflow_json` to persist the generated-files contract. `agent_tasks.task_type` now supports `code_generation`, and `issue_sessions.status` supports:

- `code_generation_queued`
- `code_generation_processing`
- `code_generated`

Migration: `backend/db/migrations/003_sprint3_code_generation.sql`.

## Implementation links

| Artifact | File |
| --- | --- |
| Workflow orchestration | `backend/internal/service/workflow.go` |
| Generated-files validation | `backend/internal/service/workflow.go` |
| Agent Engine client contract | `backend/internal/agent/client.go` |
| HTTP endpoints | `backend/internal/httpapi/server.go` |
| OpenAPI spec | `backend/internal/httpapi/openapi.json` |
| Domain contracts | `backend/internal/domain/domain.go` |
| PostgreSQL migration | `backend/db/migrations/003_sprint3_code_generation.sql` |
| Backend architecture | `backend/docs/architecture/backend.md` |
| Integration tests | `backend/internal/httpapi/integration_test.go` |
| Agent client tests | `backend/internal/agent/client_test.go` |
| Validation tests | `backend/internal/service/plan_validation_test.go` |

## Verification

- `go test ./...`
- `internal/httpapi/integration_test.go` covers analyze, correction, approve, code-generation status, generated-files contract response, and OpenAPI presence.
- `internal/agent/client_test.go` covers both `/v1/plans/generate` and `/v1/files/generate`.
- `internal/service/plan_validation_test.go` covers generated file operation validation.
