# Implementation Plan Format

Every issue-to-plan response must be valid Markdown and use the sections below in the same order.

```markdown
# Implementation Plan

## Issue Summary
Brief restatement of the requested change.

## Goal
The expected product or technical outcome.

## Relevant Files
- `path/to/existing_file`: why it is relevant.
- `path/to/new_file` (create): why it is needed.

## Proposed Changes
- Concrete behavior or interface changes.

## Implementation Steps
1. Ordered implementation step.
2. Ordered implementation step.

## Expected Files to Change
- `path/to/file`: create or modify.

## Tests and Verification
- Test or verification step.

## Risks and Open Questions
- Risk, dependency, or `TBD` when repository context is insufficient.
```

## Rules

- Return only the implementation plan, without source code or patches.
- Reference only files present in the supplied repository context.
- Mark proposed new files with `(create)`.
- Use `TBD` instead of inventing missing repository details.
- Keep steps concrete, ordered, and testable.

