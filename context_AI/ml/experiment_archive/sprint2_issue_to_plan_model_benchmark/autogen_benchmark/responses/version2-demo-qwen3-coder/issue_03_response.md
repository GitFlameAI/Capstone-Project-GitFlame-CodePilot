# Implementation Plan

## Issue Summary
The issue requests extending the repository AI configuration (.yml) to include RAG limits for maximum retrieved files and maximum snippets per file. It also requires validating positive bounds for these limits and exposing the parsed values to the issue analysis flow. The user must not be able to configure model or plan format, and allowed_actions should not be part of the public configuration.

## Goal
To enhance the AI configuration schema with RAG limits, validate their positive bounds, and make them available for use in the issue analysis flow while maintaining existing configuration structure and validation logic.

## Relevant Files
- `backend/internal/app/config_service.go`: Contains the AI configuration parsing and validation logic that needs to be extended.
- `backend/internal/app/models.go`: Defines the data structures used in the application, including the IssueAnalyzeRequest which includes YAMLConfig.
- `docs/config/ai_config.example.yml`: Provides example configuration showing the current structure and where new fields should be added.

## Proposed Changes
1. Extend the `AIConfig` struct to include `MaxRAGFiles` and `MaxSnippetsPerFile` fields.
2. Update the `ParseAIConfig` function to extract these new fields from the YAML configuration.
3. Add validation logic to ensure both values are positive integers.
4. Ensure the new fields are properly exposed through the `IssueAnalyzeRequest` structure for use in the analysis flow.

## Implementation Steps
1. Modify the `AIConfig` struct in `config_service.go` to add `MaxRAGFiles` and `MaxSnippetsPerFile` integer fields.
2. Update the `ParseAIConfig` function to extract `max_files` and `max_snippets_per_file` from the `rag` section of the YAML configuration.
3. Add validation checks in `validateAIConfig` to ensure both `MaxRAGFiles` and `MaxSnippetsPerFile` are positive integers.
4. Verify that the updated configuration structure works correctly with the existing `IssueAnalyzeRequest` flow.

## Expected Files to Change
- `backend/internal/app/config_service.go`: Modified to support new RAG limit fields and validation.
- `backend/internal/app/models.go`: No changes needed as the IssueAnalyzeRequest already includes YAMLConfig.

## Tests and Verification
- Create a test case that parses a valid YAML config with the new RAG limits set to positive integers and verifies they are correctly extracted and validated.
- Create a test case that attempts to parse a YAML config with negative or zero values for the RAG limits and ensures validation fails appropriately.
- Verify that the existing functionality remains intact by running all existing tests for the configuration parsing and validation.

## Risks and Open Questions
- TBD: Need to confirm if there are any existing configurations that might conflict with the new fields or if additional validation is needed for other parts of the rag section.
- The current implementation assumes that the rag section exists in the YAML; we should ensure robustness against missing sections or malformed entries.
- Need to verify that the new fields are properly integrated into the issue analysis flow without breaking existing functionality.
