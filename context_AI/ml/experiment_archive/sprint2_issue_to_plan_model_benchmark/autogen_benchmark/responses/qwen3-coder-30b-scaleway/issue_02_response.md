# Implementation Plan

## Issue Summary
The ML client needs to become resilient to transient failures by adding configurable request timeouts and bounded retries for specific error types (connection errors, HTTP 429, HTTP 5xx), while avoiding retries for validation errors or cancelled requests. The system should return useful errors without exposing sensitive information from the ML service.

## Goal
Implement retry logic with configurable timeouts and retry limits for the ML client to handle temporary service unavailability gracefully, ensuring the backend remains responsive during transient ML service issues.

## Relevant Files
- `backend/internal/app/services.go`: Contains the MLClient implementation that needs retry logic added.
- `backend/internal/app/config.go`: Needs to be extended to support timeout and retry configurations.
- `backend/internal/app/server.go`: Uses the MLClient and may need adjustments for error handling.

## Proposed Changes
1. Add timeout and retry configuration fields to the Config struct.
2. Modify MLClient to accept retry configuration parameters.
3. Implement retry logic in the postJSON method using backoff strategy.
4. Add error categorization to distinguish retryable vs non-retryable errors.
5. Enhance error messages to avoid exposing sensitive ML service responses.

## Implementation Steps
1. Extend Config struct with timeout and retry settings.
2. Update MLClient constructor to accept retry configuration.
3. Implement retryable error detection logic.
4. Add exponential backoff retry mechanism in postJSON method.
5. Modify error handling to sanitize error messages.
6. Update server.go to pass retry config to MLClient.

## Expected Files to Change
- `backend/internal/app/config.go`: Add timeout and retry configuration fields.
- `backend/internal/app/services.go`: Modify MLClient to support retries and sanitization.
- `backend/internal/app/server.go`: Pass retry configuration to MLClient.

## Tests and Verification
- Unit tests for MLClient with various retry scenarios (429, 5xx, connection errors).
- Integration tests verifying timeout and retry behavior.
- Verify error message sanitization prevents sensitive data exposure.
- Test that validation errors and cancelled requests are not retried.

## Risks and Open Questions
- Risk: Adding retry logic might increase latency for legitimate failures. Solution: Use bounded retries with exponential backoff.
- Risk: Error sanitization might mask important debugging information. Solution: Log original errors internally but return generic messages.
- TBD: What specific retry limits and backoff strategy should be used? (Default to 3 retries with 1s base delay).
- TBD: Should cancellation errors be treated as retryable? (No, they indicate client intent to stop).
