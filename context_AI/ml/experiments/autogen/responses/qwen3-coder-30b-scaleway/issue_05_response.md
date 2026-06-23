# Implementation Plan

## Issue Summary
Implement automatic expiration of recommendation results based on repository retention settings. Recommendation results should be retained for a configurable number of days and then automatically removed. The solution must ensure expired results aren't returned by read APIs and provide a safe cleanup operation.

## Goal
Add automatic expiration of recommendation results based on repository retention configuration while maintaining existing close/delete functionality and ensuring read APIs filter out expired results.

## Relevant Files
- `backend/db/schema.sql`: Database schema needs modification to add expiration timestamps and indexes.
- `backend/models/recommendation_run.go`: Model needs to support expiration timestamp.
- `backend/models/recommendation.go`: Model needs to support expiration timestamp.
- `backend/internal/app/storage.go`: In-memory storage needs to track expiration and filter expired items.
- `backend/internal/app/server.go`: API endpoints need to filter expired recommendations.
- `docs/config/ai_config.example.yml`: Configuration example shows `recommendation_ttl_days` setting.

## Proposed Changes
1. Add `expires_at` timestamp column to `recommendation_runs` table
2. Add `expires_at` timestamp column to `recommendations` table  
3. Modify recommendation retrieval logic to filter out expired items
4. Add cleanup operation to remove expired recommendations
5. Update memory store to track and filter expired recommendations
6. Ensure existing close/delete behavior remains unchanged

## Implementation Steps
1. Modify database schema to add `expires_at` columns to recommendation runs and recommendations tables
2. Update model structs to include expiration timestamp fields
3. Modify storage layer to track expiration timestamps when saving recommendations
4. Update recommendation retrieval methods to filter out expired items
5. Implement cleanup operation that removes expired recommendations from both DB and memory store
6. Update API handlers to ensure expired recommendations aren't returned
7. Add tests for expiration filtering and cleanup operations

## Expected Files to Change
- `backend/db/schema.sql`: Add expires_at columns and indexes
- `backend/models/recommendation_run.go`: Add ExpiresAt field
- `backend/models/recommendation.go`: Add ExpiresAt field
- `backend/internal/app/storage.go`: Track and filter expired recommendations
- `backend/internal/app/server.go`: Filter expired recommendations in API responses

## Tests and Verification
- Test that recommendations expire after configured TTL
- Test that expired recommendations aren't returned by read APIs
- Test that cleanup operation safely removes expired items repeatedly
- Test that existing close/delete functionality works unchanged
- Test that non-expired recommendations are still returned correctly
- Verify database indexes exist for efficient expiration queries

## Risks and Open Questions
- Risk: Memory store expiration tracking may not be consistent with database state during cleanup operations
- Risk: Cleanup operation might miss recommendations if they're added between cleanup cycles
- TBD: How to handle repository config updates that change TTL values for existing recommendations
- TBD: What happens to recommendations that were already expired but not yet cleaned up when TTL changes
- Question: Should we implement a background job for cleanup or make it part of regular operations?
