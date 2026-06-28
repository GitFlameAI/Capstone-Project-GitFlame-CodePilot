# Generated Files Contract

Agent Engine Version 3 adds a post-approval code generation contract. The Agent Engine receives an
approved `plan.md`, repository metadata, YAML configuration, issue metadata, and GitFlame-supplied
repository files. It returns file operations only. It does not write files, create branches, commit,
open pull requests, or assign reviewers.

## Endpoint

`POST /v1/files/generate`

## Request

```json
{
  "request_id": "codegen-123",
  "issue": {
    "id": "issue-42",
    "title": "Validate token expiration",
    "body": "Reject expired tokens."
  },
  "repository": {
    "id": "repo-7",
    "default_branch": "main",
    "commit_sha": "abc123"
  },
  "approved_plan_markdown": "# Implementation Plan\n...",
  "configuration_yaml": "include:\n  - src/**\nexclude:\n  - vendor/**\n",
  "repository_files": [
    {
      "path": "src/auth.py",
      "content": "def validate(token):\n    return token.valid\n"
    }
  ]
}
```

## Response

```json
{
  "request_id": "codegen-123",
  "status": "completed",
  "summary": "Generated file operations for the approved plan.",
  "files": [
    {
      "action": "modify",
      "path": "src/auth.py",
      "content": "def validate(token):\n    if token.expired:\n        return False\n    return token.valid\n",
      "diff": "--- a/src/auth.py\n+++ b/src/auth.py\n@@\n+    if token.expired:\n+        return False\n",
      "explanation": "Adds the approved expiration guard."
    }
  ],
  "model": "Qwen/Qwen3-Coder-30B-A3B-Instruct",
  "usage": {
    "prompt_tokens": 1200,
    "completion_tokens": 420,
    "total_tokens": 1620,
    "tool_calls": 0,
    "reasoning_chars": 240,
    "generation_time_seconds": 8.4
  }
}
```

## File Operation Rules

- `action` must be one of `create`, `modify`, or `delete`.
- `path` must be a safe repository-relative path. Absolute paths, parent traversal, Windows drive
  paths, and `.git` paths are rejected.
- `create` and `modify` require non-empty `content`.
- `delete` must not include `content` or `diff`.
- Each operation requires `explanation`.
- Duplicate generated file paths are rejected.
- `modify` and `delete` can target only files supplied in the request after YAML filtering.
- `create` must target a non-existing supplied file and still pass YAML include/exclude rules.

The backend/GitFlame layer owns branch creation, commit construction, pull request payloads, and
reviewer assignment.
