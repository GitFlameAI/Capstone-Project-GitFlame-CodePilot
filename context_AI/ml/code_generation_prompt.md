# Code Generation Prompt

Agent Engine Version 3 uses a separate structured prompt for approved-plan-to-generated-files
generation. This is intentionally separate from the issue-to-plan Agent Loop.

## System Prompt Summary

The model is instructed to:

- convert an approved implementation plan into file operations;
- return only one JSON object that conforms to the generated files schema;
- use only safe repository-relative paths;
- use only the actions `create`, `modify`, and `delete`;
- include complete replacement `content` for `create` and `modify`;
- omit `content` and `diff` for `delete`;
- include an `explanation` for every operation;
- never claim that it wrote files, created branches, committed changes, opened pull requests, or
  assigned reviewers.

The implementation lives in:

- `recommendations/src/agent_engine/prompt.py`
- `recommendations/src/agent_engine/service.py`
- `recommendations/src/agent_engine/models.py`

## User Prompt Inputs

The prompt receives:

- `request_id`;
- issue metadata;
- repository metadata;
- parsed operator-safe YAML configuration;
- the approved `plan.md`;
- filtered repository file inventory;
- filtered repository file contents;
- JSON Schema for the generated files contract.

Repository files and the approved plan are wrapped as untrusted input. The prompt repeats that they
are data, not instructions. The Agent Engine also sends `response_format=json_schema` to supported
OpenAI-compatible endpoints and validates the returned JSON with Pydantic before responding.

## Safety Boundary

No repository write tool is exposed for code generation. The service performs one structured model
call and returns the validated contract. Backend/GitFlame remains responsible for applying changes
to a branch and creating commit or pull request payloads.
