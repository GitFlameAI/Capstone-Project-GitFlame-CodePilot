# Implementation Plan

## Issue Summary
Replace in-memory storage with PostgreSQL persistence for issue-to-plan sessions and generated plans, while maintaining existing API behavior. This includes storing plan revisions and user actions (approve, correct, reject).

## Goal
Implement PostgreSQL-backed persistence for issue sessions and recommendation reports that survives backend restarts, preserving all current API endpoints and behaviors.

## Relevant Files
- `backend/internal/app/storage.go`: Contains current in-memory store implementation that needs to be replaced.
- `backend/internal/app/server.go`: Uses the memory store and needs to be updated to use PostgreSQL.
- `backend/db/schema.sql`: Defines the database schema for persistence.
- `backend/internal/app/models.go`: Data models used by both storage and server components.

## Proposed Changes
- Replace `MemoryStore` with a PostgreSQL-based implementation
- Add database connection management to server initialization
- Implement all CRUD operations for issue sessions and recommendations using PostgreSQL
- Maintain identical API responses and behaviors
- Ensure proper transaction handling and error propagation

## Implementation Steps
1. Create PostgreSQL connection management in server initialization
2. Implement PostgreSQL store interface that mirrors MemoryStore methods
3. Migrate data access logic from server to use PostgreSQL store
4. Add database migration support for schema initialization
5. Update server startup to initialize database connections
6. Verify all existing tests continue to pass with new persistence layer

## Expected Files to Change
- `backend/internal/app/server.go`: Add DB connection setup and replace MemoryStore usage
- `backend/internal/app/storage.go`: Replace MemoryStore with PostgreSQL implementation
- `backend/internal/app/models.go`: No changes needed, models already compatible
- `backend/db/schema.sql`: Already contains required schema (no changes needed)

## Tests and Verification
- Run existing unit tests to ensure API behavior unchanged
- Test all issue workflow endpoints (approve, correct, reject, get plan)
- Test recommendation analysis and retrieval endpoints
- Verify data persists across server restarts
- Validate database schema migrations work correctly
- Test concurrent access scenarios with multiple sessions

## Risks and Open Questions
- Risk: Database connection pool exhaustion under high load - need to configure appropriate pool settings
- Risk: Schema migration failures during deployment - need robust migration strategy
- Risk: Performance degradation due to database roundtrips vs in-memory access - need profiling
- TBD: What is the expected concurrency pattern for issue sessions? Need to understand if multiple simultaneous workflows per issue are supported
- TBD: How should we handle database connection failures during operation? Need to define retry/backoff strategy
- TBD: Are there any specific indexes needed beyond those defined in schema.sql?
