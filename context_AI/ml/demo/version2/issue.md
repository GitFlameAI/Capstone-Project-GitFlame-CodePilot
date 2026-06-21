# Demo Issue

## Make ML client resilient to transient failures

The backend currently fails immediately when the ML service is slow or temporarily unavailable. Add configurable request timeout and bounded retries for connection errors, HTTP 429, and HTTP 5xx responses. Do not retry validation errors or cancelled requests. Return useful errors without exposing sensitive response bodies.

## Repository Context

The following files were supplied to the model:

- `backend/internal/app/services.go`
- `backend/internal/app/config.go`
- `backend/internal/app/server.go`

