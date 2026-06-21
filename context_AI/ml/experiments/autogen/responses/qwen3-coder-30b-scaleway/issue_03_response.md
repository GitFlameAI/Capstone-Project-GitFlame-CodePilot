# Implementation Plan

## Issue Summary
Add snippet limits to the repository AI configuration by extending the `.yml` schema with `max_retrieved_files` and `max_snippets_per_file` fields under a new `rag` section. These values should be validated as positive integers and exposed to the issue analysis flow. The user must not be able to configure model or plan format, and `allowed_actions` must not be part of the public configuration.

## Goal
Extend the AI configuration schema to support RAG limits while maintaining backward compatibility and enforcing validation rules. The parsed values must be accessible during issue analysis processing.

## Relevant Files
- `backend/internal/app/config_service.go`: Main parsing and validation logic for AI configuration.
- `backend/internal/app/models.go`: Data structures used in API requests/responses that may need to include new config values.
- `docs/config/ai_config.example.yml`: Example configuration showing current structure and where new fields should be added.

## Proposed Changes
1. Extend `AIConfig` struct to include `MaxRetrievedFiles` and `MaxSnippetsPerFile` fields.
2. Add parsing logic in `ParseAIConfig` to read these new fields from YAML under `rag.max_retrieved_files` and `rag.max_snippets_per_file`.
3. Add validation to ensure both values are positive integers.
4. Remove `allowed_actions` from the public configuration exposure (already handled via separate fields).
5. Update `validateAIConfig` to validate the new fields.
6. Ensure new fields are passed through to the issue analysis flow via `IssueAnalyzeRequest`.

## Implementation Steps
1. Modify `AIConfig` struct in `config_service.go` to add `MaxRetrievedFiles` and `MaxSnippetsPerFile` integer fields.
2. Update `ParseAIConfig` function to extract `rag.max_retrieved_files` and `rag.max_snippets_per_file` from YAML using existing helper functions.
3. Add validation checks in `validateAIConfig` to ensure both values are positive integers.
4. Update `IssueAnalyzeRequest` in `models.go` to pass through the parsed configuration values if needed for downstream processing.
5. Verify that the example configuration in `docs/config/ai_config.example.yml` includes the new fields.

## Expected Files to Change
- `backend/internal/app/config_service.go`: Modify `AIConfig` struct and update `ParseAIConfig` and `validateAIConfig` functions.
- `backend/internal/app/models.go`: Potentially update `IssueAnalyzeRequest` if new fields need to be passed through.
- `docs/config/ai_config.example.yml`: Add example values for the new fields.

## Tests and Verification
- Add unit tests for `ParseAIConfig` to verify parsing of new fields.
- Add validation tests for negative or zero values for both new fields.
- Ensure existing functionality remains intact after changes.
- Test that the new fields are correctly included in the parsed configuration object.
- Verify that the example configuration validates properly.

## Risks and Open Questions
- Risk: If `max_retrieved_files` or `max_snippets_per_file` are not provided in the YAML, they might default to zero or negative values, causing validation failures. Solution: Set appropriate defaults in `ParseAIConfig`.
- Risk: The new fields may not be fully utilized in the issue analysis flow yet, which could lead to unused code. Solution: Ensure they are integrated into the relevant processing paths.
- TBD: Whether the new fields should be passed through to `IssueAnalyzeRequest` or if they're purely for internal validation. Based on context, they likely need to be available for processing.
- TBD: What are the reasonable default values for `max_retrieved_files` and `max_snippets_per_file`? Need to define sensible defaults in case they're omitted.
