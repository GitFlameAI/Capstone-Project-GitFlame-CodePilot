-- Sprint 7: indexes for the operational endpoints.
--
-- /ops/tasks lists the most recent agent tasks and /ops/status counts the tasks
-- of the last 24 hours. Both sort or filter by created_at, and agent_tasks only
-- had an index on status, so both queries degraded into a sequential scan as the
-- table grew.

CREATE INDEX IF NOT EXISTS idx_agent_tasks_created_at ON agent_tasks (created_at DESC);
