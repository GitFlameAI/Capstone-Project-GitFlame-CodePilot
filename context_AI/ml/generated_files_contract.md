# Generated Files Contract

## Contract Owner

The generated files contract describes only file changes produced after the user approves the final `plan.md`.

Responsibilities:

- Agent Engine changes files in a temporary local repository workspace.
- Agent Engine returns a structured list of changed files.
- Backend validates and stores this list.
- Backend creates GitFlame-specific wrapper data: branch name, commit message, PR title, reviewer, and target branch.

This contract must not include branch name, commit message, PR title, reviewer, or target branch.

## Backend Responsibilities

Backend must:

- validate file paths and actions;
- store generated file metadata;
- reject unsafe paths;
- create branch/commit/PR metadata;
- pass file changes to GitFlame;
- expose code generation task status to frontend;
- store failure reason if generation fails.

## Agent Engine Responsibilities

Agent Engine must:

- implement only the final approved `plan.md`;
- change only files required by the plan;
- return a structured list of changed files;
- return failure information if generation cannot be completed;
- never create branch, commit, push, or Pull Request;
- never return GitFlame workflow metadata.

## Required Fields

| Field | Required | Possible values | Meaning |
|---|---:|---|---|
| `status` | Yes | `success`, `failed` | Result of code generation |
| `summary` | Yes | text | Short description of the result |
| `files` | Yes for `success` | array | List of changed files |
| `files[].path` | Yes for `success` | repository-relative path | File changed by Agent Engine |
| `files[].action` | Yes for `success` | `create`, `modify`, `delete` | Type of file operation |
| `files[].description` | Yes for `success` | text | Short explanation of the change |
| `reason` | Yes for `failed` | text | Why generation failed |
| `missing_information` | No | array of text | Missing files/context/blockers |
| `recommended_next_step` | No | text | What should happen next |
