# Implementation Plan

## Issue Summary
The ML client currently fails immediately on network or service issues instead of handling transient failures gracefully. We need to add configurable timeouts and bounded retries for specific error types while avoiding retries for validation or cancellation errors.

## Goal
Make the ML client resilient by implementing configurable request timeouts and bounded retries for connection errors, HTTP 429, and HTTP 5xx responses, without retrying validation errors or cancelled requests. Return meaningful errors without exposing sensitive response content.

## Relevant Files
- `backend/internal/app/services.go`: Contains the MLClient implementation that needs resilience enhancements.
- `backend/internal/app/config.go`: May need extension to support new timeout and retry configurations.
- `backend/internal/app/server.go`: Uses the MLClient and handles error responses.

## Proposed Changes
- Add timeout and retry configuration parameters to MLClient initialization
- Implement retry logic in `postJSON` method with bounded attempts for specific error types
- Modify error handling to distinguish between retryable and non-retryable errors
- Ensure error messages don't expose sensitive response bodies
- Add new error types for retryable vs non-retryable conditions

## Implementation Steps
1. Extend MLClient struct to include timeout and retry configuration parameters
2. Add retry configuration fields to Config struct in config.go
3. Modify NewMLClient to accept and initialize retry settings from configuration
4. Update postJSON method to implement retry logic with bounded attempts for connection errors, 429, and 5xx responses
5. Add helper functions to identify retryable HTTP status codes and errors
6. Modify error handling in GenerateIssuePlan and GenerateRecommendations to avoid retrying validation/cancellation errors
7. Update error responses to sanitize sensitive information from error messages
8. Add tests for retry behavior and error handling scenarios

## Expected Files to Change
- `backend/internal/app/services.go`: Modify MLClient and postJSON method
- `backend/internal/app/config.go`: Extend Config struct with retry settings
- `backend/internal/app/server.go`: Update error handling in handlers

## Tests and Verification
- Test that connection errors trigger retries up to configured limit
- Verify HTTP 429 and 5xx responses are retried appropriately
- Confirm validation errors and cancelled requests are not retried
- Validate that error messages do not expose sensitive response content
- Test timeout behavior with slow ML service responses
- Ensure successful requests work without modification

## Risks and Open Questions
- TBD: How many retry attempts should be configured by default?
- TBD: What is the appropriate backoff strategy for retries?
- TBD: Should retry delays be configurable or fixed?
- The current implementation uses a simple 4-second timeout; we need to make this configurable via environment variables or config structure
- Need to determine if we should add retry configuration to the existing Config struct or create a separate MLClientConfig struct
