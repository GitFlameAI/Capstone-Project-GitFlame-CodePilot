# Container Smoke Model Outputs

This file contains consolidated model outputs for the smoke benchmark.
It is intentionally stored as one file rather than separate per-model response files.

## poolside/Laguna-XS.2

- Issue: `issue_02`
- Status: `ok`
- Format valid: `True`
- Latency seconds: `57.805`
- Tokens per second: `11.054`
- Peak VRAM GB: `61.881`

````markdown
# Implementation Plan

## Issue Summary
The backend's ML client currently fails immediately when the ML service is slow or temporarily unavailable. The issue requests adding configurable request timeout and bounded retries for connection errors, HTTP 429, and HTTP 5xx responses, while avoiding retries for validation errors or cancelled requests. Errors should be useful without exposing sensitive response bodies.

## Goal
Make the ML client resilient to transient failures by implementing configurable timeouts and retry logic with exponential backoff, while ensuring sensitive data is not leaked in error responses.

## Relevant Files
- `backend/internal/app/services.go`: Contains the `MLClient` struct and its methods that need retry logic and timeout configuration.
- `backend/internal/app/config.go`: Contains the `Config` struct where retry and timeout settings should be added.
- `backend/internal/app/server.go`: Instantiates the `MLClient` and may need updates to pass configuration.

## Proposed Changes
- Add retry configuration fields to the `Config` struct (timeout, max retries, base delay).
- Modify `MLClient` to accept retry configuration and implement retry logic with exponential backoff.
- Update `postJSON` to handle retryable errors (connection errors, 429, 5xx) and non-retryable errors (4xx validation errors, context cancellation).
- Sanitize error responses to avoid exposing sensitive data.
- Update `NewMLClient` to accept configuration and apply timeout settings.

## Implementation Steps
1. Add retry configuration fields to `Config` struct in `config.go` (timeout, max retries, base delay).
2. Update `LoadConfig` to read retry settings from environment variables with sensible defaults.
3. Modify `MLClient` struct to include retry configuration fields.
4. Update `NewMLClient` to accept configuration and apply timeout settings.
5. Implement retry logic in `postJSON` with exponential backoff for retryable errors.
6. Add helper function to determine if an error is retryable based on status code or error type.
7. Add helper function to sanitize error responses before returning them.
8. Update `NewServer` in `server.go` to pass configuration to `NewMLClient`.

## Expected Files to Change
- `backend/internal/app/config.go`: modify to add retry configuration.
- `backend/internal/app/services.go`: modify to implement retry logic and error sanitization.
- `backend/internal/app/server.go`: modify to pass configuration to `NewMLClient`.

## Tests and Verification
- Unit test for retry logic with mock HTTP server returning 503 errors.
- Unit test for retry logic with 429 status code.
- Unit test verifying no retry on 400/422 status codes.
- Unit test verifying context cancellation stops retries.
- Unit test verifying error messages do not contain response body content.

## Risks and Open Questions
- TBD: The exact retry strategy (exponential backoff with what max delay?) needs to be decided.
- TBD: Whether to add metrics or logging for retry attempts.
</assistant>
````

## CohereLabs/North-Mini-Code-1.0

- Issue: `issue_02`
- Status: `ok`
- Format valid: `False`
- Latency seconds: `81.148`
- Tokens per second: `15.774`
- Peak VRAM GB: `56.771`

````markdown
We need to produce an implementation plan for making the ML client resilient to transient failures. The issue: currently backend fails immediately when ML service is slow or temporarily unavailable. Need to add configurable request timeout and bounded retries for connection errors, HTTP 429, and HTTP 5xx responses. Do not retry validation errors or cancelled requests. Return useful errors without exposing sensitive response bodies.

We have repository context: backend/internal/app/services.go (contains MLClient struct and methods), config.go (Config includes MLServiceURL), server.go (uses MLClient). We need to modify MLClient to have configurable timeout and retry logic. Also need to add configuration for retries (max attempts, backoff). The config currently only has Addr, MLServiceURL, DatabaseURL. We need to add fields for timeout and retry config. Possibly add a new config struct for MLClient config, or extend Config. The issue says "configurable request timeout and bounded retries". So we need to add config fields, maybe in Config: MLServiceTimeout, MLServiceMaxRetries, MLServiceRetryDelay (or backoff). Or we could have a separate struct MLConfig. But we need to keep changes minimal and within existing code.

We need to modify NewMLClient to accept config parameters, maybe from Config struct. The server creates NewMLClient(cfg.MLServiceURL). We could extend Config to include MLServiceTimeout and MLServiceMaxRetries. Then NewMLClient can read those from Config (maybe pass Config). Or we could add a method to set httpClient timeout and retry logic.

We also need to modify postJSON to implement retry logic for certain errors: connection errors, HTTP 429, HTTP 5xx. Not retry validation errors (i.e., 4xx other than 429?) and cancelled requests (context cancelled). Also need to return useful errors without exposing sensitive response bodies. So we should wrap errors, maybe include status code and retry count.

We need to ensure that the fallback functions (fallbackIssuePlan, fallbackRecommendations) are used when ML client fails after retries? Currently server calls ml.GenerateIssuePlan, if err != nil, it uses fallbackIssuePlan. So we need to ensure that after retries exhausted, we return an error that triggers fallback. So we need to modify postJSON to return error after retries exhausted, and server will fallback.

We also need to add tests. But we only need to produce implementation plan, not code. So we need to list relevant files: services.go, config.go, maybe server.go (if changes needed). Also maybe a new file for retry logic (e.g., retry.go) but we can implement within services.go.

We need to propose changes: add config fields, modify NewMLClient to accept config, add retry logic in postJSON, maybe add a helper function for retry.

We need to ensure we respect configuration.include/exclude? Not relevant.

We need to produce plan sections.

Let's outline:

## Issue Summary: Currently ML client fails immediately on transient failures; need timeout and retries.

## Goal: Add configurable request timeout and bounded retries for connection errors, HTTP 429, HTTP 5xx; not retry validation errors or cancelled requests; return useful errors without exposing sensitive response bodies.

## Relevant Files:
- backend/internal/app/services.go: MLClient struct, NewMLClient, postJSON, GenerateIssuePlan, GenerateRecommendations.
- backend/internal/app/config.go: Config struct; need to add fields for timeout and retry config.
- backend/internal/app/server.go: creation of MLClient; may need to pass config.

## Proposed Changes:
- Extend Config with MLServiceTimeout (time.Duration), MLServiceMaxRetries (int), MLServiceRetryDelay (time.Duration) maybe.
- Update NewMLClient to accept Config (or separate MLConfig) and set httpClient.Timeout accordingly.
- Add retry logic in postJSON: for each attempt, execute request; if error (network) or status code in [429, 5xx], retry up to max attempts with backoff; if context cancelled/timeout, stop; if validation errors (4xx not 429) or other non-retryable, return error immediately.
- Ensure error messages are generic, not exposing response body.
- Possibly add a new type for retryable errors.

## Implementation Steps:
1. Add fields to Config struct in config.go.
2. Update LoadConfig to read environment variables for new fields (e.g., ML_SERVICE_TIMEOUT, ML_SERVICE_MAX_RETRIES, ML_SERVICE_RETRY_DELAY). Provide defaults (maybe timeout 4s, max retries 3, delay 1s).
3. Modify NewMLClient signature to accept Config (or extract ML-specific config). Could create a new struct MLConfig inside services.go that holds timeout, maxRetries, retryDelay, and baseURL. Or we can keep NewMLClient taking baseURL and config fields from a global config? Simpler: modify NewMLClient to accept Config (or just read from a package-level config). But server already has cfg; we can pass cfg to NewMLClient. So change NewMLClient(cfg Config) *MLClient. Then set httpClient.Timeout = cfg.MLServiceTimeout (if zero, default). Also store retry config.
4. Implement retry logic in postJSON: define a loop for attempt from 0 to maxRetries; use time.Sleep between attempts (exponential backoff maybe). Use context for cancellation.
5. Ensure that after retries exhausted, return error with status code or generic message.
6. Update server.go to pass cfg to NewMLClient (change call).
7. Add unit tests for retry behavior (but we only need to describe tests in plan).

## Expected Files to Change:
- backend/internal/app/config.go (add fields, update LoadConfig)
- backend/internal/app/services.go (MLClient struct, NewMLClient, postJSON, maybe add retry helper)
- backend/internal/app/server.go (NewServer: pass cfg to NewMLClient)

## Tests and Verification:
- Test that timeout is respected (mock slow service).
- Test that retry occurs for 429 and 5xx.
- Test that validation errors (4xx) are not retried.
- Test that context cancellation stops retries.
- Test that error messages do not expose response bodies.

## Risks and Open Questions:
- Need to decide backoff strategy (linear vs exponential). Use linear
````

## mistralai/Devstral-Small-2-24B-Instruct-2512

- Issue: `issue_02`
- Status: `ok`
- Format valid: `True`
- Latency seconds: `29.034`
- Tokens per second: `22.663`
- Peak VRAM GB: `46.878`

````markdown
```markdown
# Implementation Plan

## Issue Summary
The ML client currently fails immediately when the ML service is slow or temporarily unavailable. The request needs to be made resilient to transient failures by adding configurable request timeout and bounded retries for connection errors, HTTP 429, and HTTP 5xx responses. Validation errors and cancelled requests should not be retried. Useful errors should be returned without exposing sensitive response bodies.

## Goal
Implement a resilient ML client that handles transient failures gracefully by introducing configurable timeouts and retries, while ensuring sensitive information is not exposed in error messages.

## Relevant Files
- `backend/internal/app/services.go`: Contains the `MLClient` struct and its methods, including `postJSON` which handles HTTP requests to the ML service.
- `backend/internal/app/config.go`: Contains the `Config` struct and `LoadConfig` function, where configuration values are loaded from environment variables.

## Proposed Changes
- Add configuration options for request timeout and maximum retry attempts.
- Modify the `MLClient` to implement retry logic for transient failures.
- Ensure error messages do not expose sensitive response bodies.
- Do not retry on validation errors or cancelled requests.

## Implementation Steps
1. **Update Configuration**:
   - Add `MLRequestTimeout` and `MLMaxRetries` fields to the `Config` struct in `config.go`.
   - Update `LoadConfig` to read these values from environment variables with sensible defaults.

2. **Modify MLClient**:
   - Update the `MLClient` struct to include the new configuration fields.
   - Modify the `NewMLClient` function to accept and store the new configuration values.
   - Implement a retry mechanism in the `postJSON` method for transient failures (connection errors, HTTP 429, and HTTP 5xx responses).
   - Ensure that validation errors and cancelled requests are not retried.
   - Sanitize error messages to avoid exposing sensitive response bodies.

3. **Testing**:
   - Write unit tests to verify the retry logic and error handling.
   - Test edge cases such as cancelled requests and validation errors to ensure they are not retried.

## Expected Files to Change
- `backend/internal/app/config.go`: Modify to include new configuration fields.
- `backend/internal/app/services.go`: Update `MLClient` and `postJSON` method to implement retry logic and error handling.

## Tests and Verification
- **Unit Tests**:
  - Verify that the `MLClient` correctly retries on transient failures.
  - Ensure that validation errors and cancelled requests are not retried.
  - Check that error messages do not expose sensitive response bodies.
- **Integration Tests**:
  - Simulate transient failures in the ML service to verify retry behavior.
  - Test the client with different timeout and retry configurations.

## Risks and Open Questions
- **Configuration Defaults**: Ensure that default values for `MLRequestTimeout` and `MLMaxRetries` are reasonable and do not cause performance issues.
- **Error Handling**: Verify that all error cases are properly handled and that sensitive information is not exposed.
- **Performance Impact**: Assess the impact of retries on system performance, especially under high load.
```
````

## JetBrains/Mellum2-12B-A2.5B-Thinking

- Issue: `issue_02`
- Status: `ok`
- Format valid: `False`
- Latency seconds: `57.924`
- Tokens per second: `22.098`
- Peak VRAM GB: `23.276`

````markdown
<think>
We are given an issue: "Make ML client resilient to transient failures"

The issue requires:
- Adding configurable request timeout and bounded retries for connection errors, HTTP 429, and HTTP 5xx responses.
- Not retrying validation errors or cancelled requests.
- Returning useful errors without exposing sensitive response bodies.

Looking at the repository context, we have two files that are relevant to the ML client:

1. `backend/internal/app/services.go` - Contains the `MLClient` struct and its methods, including the `postJSON` method that is used to make HTTP requests to the ML service.
2. `backend/internal/app/config.go` - Contains configuration loading, but we might need to add new configuration options for the timeout and retry settings.

However, note that the issue says "configurable", so we should consider adding configuration for the timeout and retry parameters.

In `services.go`, the `MLClient` currently has:
- A `baseURL` and an `httpClient` with a fixed timeout of 4 seconds.

We need to change the `MLClient` to allow configurable timeout and retry settings.

Steps for the plan:

1. **Update the configuration**: We need to add new configuration options for the ML client's timeout and retry settings. This would be in `config.go`.
   - We can add two new fields to the `Config` struct: `MLClientTimeout` and `MLClientMaxRetries`.
   - Then, in `LoadConfig`, we can read these from environment variables.

2. **Update the `MLClient` struct and constructor**:
   - The `NewMLClient` function should take the timeout and max retries as parameters (or read from the global config).
   - We'll create an `http.Client` with the configured timeout and a custom `Transport` that implements retry logic.

3. **Implement retry logic in the `postJSON` method**:
   - We'll use a retry mechanism that:
        - Retries on connection errors (like network errors, DNS failures), HTTP 429, and HTTP 5xx (but not 4xx validation errors or 400, 401, etc., and not on cancelled requests).
   - We'll limit the number of retries to the configured max.

4. **Error handling**:
   - We should not expose sensitive response bodies. Currently, when there's an error, we return the status code and the error message. We need to ensure that we don't include the response body in the error.

Now, let's break down the implementation steps:

### Relevant Files

- `backend/internal/app/config.go`: To add configuration for the ML client.
- `backend/internal/app/services.go`: To update the `MLClient` to use the new configuration and implement retries.

### Proposed Changes

#### In `config.go`:
We'll add two new fields to the `Config` struct:
- `MLClientTimeout` (time.Duration)
- `MLClientMaxRetries` (int)

And update `LoadConfig` to read these from environment variables.

#### In `services.go`:
- Modify the `NewMLClient` function to accept `timeout` and `maxRetries` (or read from the global config, but since the `MLClient` is created in `server.go` with `cfg.MLServiceURL`, we can pass the timeout and max retries from the server's config).
- In the `postJSON` method, we'll implement a retry mechanism. We can use a loop that retries on the specified errors, up to `maxRetries` times.

### Implementation Steps

1. **Update configuration** (`config.go`):
   - Add `MLClientTimeout` and `MLClientMaxRetries` to the `Config` struct.
   - In `LoadConfig`, read these from environment variables (with defaults).

2. **Update server configuration** (`server.go`):
   - When creating the `MLClient` in `NewServer`, pass the new timeout and max retries from the loaded config.

3. **Update `MLClient` constructor** (`services.go`):
   - Change `NewMLClient` to take `timeout` and `maxRetries` as parameters.
   - Create an `http.Client` with the given timeout and a custom `Transport` that handles retries.

4. **Implement retry logic in `postJSON`** (`services.go`):
   - Use a loop that attempts the request up to `maxRetries+1` times (original attempt + retries).
   - On each attempt, if the error is a connection error, or HTTP 429, or HTTP 5xx (but not 4xx), then retry.
   - Do not retry on validation errors (4xx) or cancelled requests (context.Canceled, context.DeadlineExceeded).

5. **Error handling**:
   - When returning an error, make sure we don't include the response body. Currently, we only return the status code, which is safe.

### Risks and Open Questions

- **Retry backoff**: Should we implement exponential backoff? The issue doesn't specify, but it's a common practice. However, the issue says "bounded retries", so we might just do fixed retries without backoff to keep it simple. But note: without backoff, we might overwhelm the service. We should consider adding a small backoff.
- **Configuration defaults**: What should be the default timeout and max retries? We can set timeout to 4 seconds (as currently) and max retries to 2 (so 3 attempts total).
- **Context cancellation**: We must not retry if the context is cancelled.

Given the above, let's write the implementation plan.

Note: We are only generating the plan, not the code.
````

## Qwen/Qwen3.6-35B-A3B

- Issue: `issue_02`
- Status: `ok`
- Format valid: `False`
- Latency seconds: `129.355`
- Tokens per second: `9.895`
- Peak VRAM GB: `63.924`

````markdown
The user wants to make the ML client resilient to transient failures.
Current behavior: `MLClient` uses a hardcoded 4-second timeout and fails immediately on connection errors or HTTP errors.
Requested behavior:
- Configurable request timeout.
- Bounded retries for connection errors, HTTP 429, and HTTP 5xx.
- Do not retry validation errors or cancelled requests.
- Return useful errors without exposing sensitive response bodies.

I need to analyze the provided repository context:
- `backend/internal/app/services.go`: Contains `MLClient`, `NewMLClient`, `postJSON`, `GenerateIssuePlan`, `GenerateRecommendations`.
- `backend/internal/app/config.go`: Contains `Config` struct and `LoadConfig`.
- `backend/internal/app/server.go`: Contains `Server`, `NewServer`, routes, handlers.

I will create an implementation plan following the required format.

Key changes needed:
1. Update `Config` to include ML client timeout and retry settings.
2. Update `MLClient` struct to hold retry configuration.
3. Modify `NewMLClient` to accept configuration or read from env/config.
4. Implement a retry mechanism in `postJSON` (or a wrapper) that handles connection errors, 429, and 5xx.
5. Ensure validation errors (4xx except 429) and context cancellations are not retried.
6. Update error handling to mask sensitive response bodies.
7. Update `server.go` to pass config to `NewMLClient`.

Let's structure the plan:

## Issue Summary
The backend currently fails immediately when the ML service is slow or temporarily unavailable. The request is to add configurable request timeouts and bounded retries for transient failures (connection errors, HTTP 429, HTTP 5xx), while avoiding retries for validation errors or cancelled requests. Sensitive response bodies must be masked in errors.

## Goal
Implement a resilient ML client with configurable timeouts and bounded retries that gracefully handles transient network and service errors, improving backend stability during ML service outages or rate limiting.

## Relevant Files
- `backend/internal/app/services.go`: Contains `MLClient` and `postJSON`; needs retry logic and configuration updates.
- `backend/internal/app/config.go`: Contains `Config` struct; needs new fields for ML client timeout and retry limits.
- `backend/internal/app/server.go`: Instantiates `MLClient`; needs to pass configuration.

## Proposed Changes
- Extend `Config` with `MLClientTimeout`, `MLClientMaxRetries`, and `MLClientRetryDelay` fields.
- Update `MLClient` to store retry configuration.
- Modify `postJSON` to implement bounded retries for connection errors, HTTP 429, and HTTP 5xx.
- Skip retries for context cancellations (`context.Canceled`, `context.DeadlineExceeded`), validation errors (4xx except 429), and non-retryable conditions.
- Mask sensitive response bodies in returned errors.
- Update `NewServer` to initialize `MLClient` with config values.

## Implementation Steps
1. Add `MLClientTimeout`, `MLClientMaxRetries`, and `MLClientRetryDelay` to `Config` in `config.go`. Update `LoadConfig` to read from environment variables with sensible defaults.
2. Update `MLClient` struct in `services.go` to include retry configuration fields.
3. Modify `NewMLClient` to accept a `Config` or specific retry parameters, and initialize the `http.Client` with the configured timeout.
4. Refactor `postJSON` to implement a retry loop:
   - Check for context cancellation/deadline; return immediately if present.
   - Execute the HTTP request.
   - On connection error or HTTP 429/5xx, log the attempt, wait for the configured delay (exponential backoff or fixed), and retry up to `MaxRetries` times.
   - On success or non-retryable error (e.g., 4xx other than 429), break the loop and return the error.
   - Ensure errors do not expose raw response bodies; wrap them with a generic message or sanitized details.
5. Update `NewServer` in `server.go` to pass the relevant config fields to `NewMLClient`.
6. Verify that `GenerateIssuePlan` and `GenerateRecommendations` correctly propagate the new error types and that fallbacks still trigger on persistent failures.

## Expected Files to Change
- `backend/internal/app/config.go`: modify (add ML client config fields and env loading)
- `backend/internal/app/services.go`: modify (update `MLClient`, `NewMLClient`, `postJSON`)
- `backend/internal/app/server.go`: modify (pass config to `NewMLClient`)

## Tests and Verification
- Unit test `postJSON` to verify it retries exactly the configured number of times on 5xx and 429, and stops on 400/401/403/404.
- Unit test `postJSON` to verify it does not retry when context is cancelled or deadline exceeded.
- Integration test verifying that a mock ML server returning 503 triggers retries and eventually falls back to the fallback function, returning a non-sensitive error message.
- Verify that environment variables `ML_CLIENT_TIMEOUT`, `ML_CLIENT_MAX_RETRIES`, and `ML_CLIENT_RETRY_DELAY` correctly configure the client.

## Risks and Open Questions
- `TBD` on whether exponential backoff or fixed delay is preferred for retry delays.
- `TBD` on how to handle rate limit reset headers (e.g., `Retry-After`) if the ML service provides them.
- Ensure retry logic does not violate context deadlines; the loop must check `ctx.Err()` before each retry.
- Existing tests for `MLClient` may need updates to account for the new retry behavior.

Check against constraints:
- Only use supplied files.
- No code generation.
- Follow exact markdown format
````
