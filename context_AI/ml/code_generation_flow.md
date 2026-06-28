# Code Generation Flow

This document describes what happens after the user approves the final `plan.md`.

## Flow

```text
1. User approves final plan.md
2. Backend creates code generation task
3. Agent Worker sends task to Agent Engine
4. Agent Engine receives final plan and repository context
5. Agent Engine modifies files in temporary workspace
6. Agent Engine returns generated files list
7. Backend validates and stores result
8. Backend prepares GitFlame branch/PR wrapper
9. Frontend shows code generation status/result
```

## Step Inputs And Outputs

| Step | Input | Output |
|---|---|---|
| 1. User approves final `plan.md` | Final approved plan, issue/session id, user action | Approved plan status |
| 2. Backend creates code generation task | Approved session, final plan, issue metadata, YAML config reference | Code generation task with `queued` status |
| 3. Agent Worker sends task to Agent Engine | Queued task, task id, session id | Agent Engine code generation request |
| 4. Agent Engine receives final plan and repository context | Final approved `plan.md`, issue metadata, YAML config, repository file tree, relevant files/snippets | Prepared local generation context |
| 5. Agent Engine modifies files in temporary workspace | Local repository workspace, approved plan, relevant context | Created/modified/deleted files in temporary workspace |
| 6. Agent Engine returns generated files list | Workspace changes, changed file paths, action types | Generated files result: `path`, `action`, `description` |
| 7. Backend validates and stores result | Generated files result, task id, session id | Stored result or failed task with validation error |
| 8. Backend prepares GitFlame branch/PR wrapper | Stored generated files result, issue metadata, repository settings, YAML config | Branch name, commit message, PR title, reviewer, file operations payload |
| 9. Frontend shows code generation status/result | Code generation task status and result from backend | User-visible status, generated files summary, errors if any |
