# Git Wrapper Rules

## Purpose

These rules define how backend wraps generated files into GitFlame branch, commit, and Pull Request metadata.

Agent Engine must not generate these fields. Backend creates them from issue data, repository settings, and YAML configuration.

## Rules

| Field | Rule |
|---|---|
| `branch_name` | `ai/{issue_id}-{slug}` |
| `slug` | Built from issue title: lowercase, spaces become `-`, special symbols are removed, max 40 characters |
| `commit_message` | `Implement: {issue_title}` |
| `pr_title` | `{issue_title}` |
| `pr_description` | Short template with issue id, summary, changed files, and AI disclaimer |
| `reviewer` | Issue author by default |
| `target_branch` | From `.yml` if defined; otherwise repository default branch |
| `source_branch` | Generated `branch_name` |
| `labels` | `ai-generated`, `needs-review` |
| `draft` | `true` by default, so a human reviews before merge |

## Example

Issue:

```text
id: 42
title: Add retry handling for model endpoint
author: danil
```

Generated wrapper:

```json
{
  "branch_name": "ai/42-add-retry-handling-for-model-endpoint",
  "commit_message": "Implement: Add retry handling for model endpoint",
  "pr_title": "Add retry handling for model endpoint",
  "reviewer": "danil",
  "target_branch": "main",
  "source_branch": "ai/42-add-retry-handling-for-model-endpoint",
  "labels": ["ai-generated", "needs-review"],
  "draft": true
}
```
