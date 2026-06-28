# Implementation Plan Format

Every issue-to-plan response must be valid Markdown and use the sections below in the same order.

```markdown
# Implementation Plan

## Issue Summary

Briefly describe what the issue asks for.

## Goal

Describe the expected final behavior after implementation.

## Implementation Steps

Provide ordered actionable steps.
Each step should describe what should be changed and where.

## Expected Files to Change

List expected files or areas.

Format:

- `path/to/file`: expected change
- `path/to/new_file`: expected change `(create)`

## Risks and Open Questions

List only real risks, blockers, missing information, or assumptions.
```

## Rules

- Return only the implementation plan.
- Do not include source code, patches, diffs, shell commands, branch names, commit messages, or pull request claims.
- Reference existing files only when their paths are present in `repository_file_tree` or `repository_snippets`.
- Mark proposed new files with `(create)`.
- Use `TBD` instead of inventing missing repository details.
- Keep implementation steps concrete, ordered, and understandable without extra explanation.
