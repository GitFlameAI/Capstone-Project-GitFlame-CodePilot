+ALTER TABLE repositories
    ADD COLUMN IF NOT EXISTS external_id TEXT;

UPDATE repositories
SET external_id = id::text
WHERE external_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_repositories_external_id
    ON repositories(external_id);

ALTER TABLE issue_sessions
    ADD COLUMN IF NOT EXISTS external_issue_id TEXT,
    ADD COLUMN IF NOT EXISTS request_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS plan_markdown TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS revision INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS feedback_history JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS generated_files JSONB,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE issue_sessions DROP CONSTRAINT IF EXISTS issue_sessions_status_check;
ALTER TABLE issue_sessions
    ADD CONSTRAINT issue_sessions_status_check CHECK (
        status IN (
            'created',
            'generating',
            'plan_generated',
            'approved',
            'correction_requested',
            'rejected'
        )
    );

CREATE UNIQUE INDEX IF NOT EXISTS idx_issue_sessions_repository_external_issue
    ON issue_sessions(repository_id, external_issue_id)
    WHERE external_issue_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS plan_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_session_id UUID NOT NULL REFERENCES issue_sessions(id) ON DELETE CASCADE,
    revision INTEGER NOT NULL,
    plan_markdown TEXT NOT NULL,
    correction_feedback TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT plan_revisions_session_revision_unique
        UNIQUE (issue_session_id, revision),
    CONSTRAINT plan_revisions_revision_positive CHECK (revision > 0)
);

CREATE TABLE IF NOT EXISTS agent_tasks (
    id UUID PRIMARY KEY,
    issue_session_id UUID NOT NULL REFERENCES issue_sessions(id) ON DELETE CASCADE,
    external_issue_id TEXT NOT NULL,
    task_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    attempt INTEGER NOT NULL DEFAULT 1,
    plan_markdown TEXT NOT NULL DEFAULT '',
    error_json JSONB,
    relevant_files JSONB NOT NULL DEFAULT '[]'::jsonb,
    model TEXT NOT NULL DEFAULT '',
    usage_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    tool_execution_summary TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_tasks_type_check CHECK (
        task_type IN ('generate', 'correction')
    ),
    CONSTRAINT agent_tasks_status_check CHECK (
        status IN ('queued', 'processing', 'completed', 'failed')
    ),
    CONSTRAINT agent_tasks_attempt_positive CHECK (attempt > 0)
);

CREATE INDEX IF NOT EXISTS idx_agent_tasks_session_created
    ON agent_tasks(issue_session_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_agent_tasks_status
    ON agent_tasks(status);

CREATE INDEX IF NOT EXISTS idx_plan_revisions_session
    ON plan_revisions(issue_session_id, revision DESC);

+ALTER TABLE recommendation_runs
    ADD COLUMN IF NOT EXISTS retention_until TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '30 days');

ALTER TABLE recommendations
    ADD COLUMN IF NOT EXISTS confidence DOUBLE PRECISION;

CREATE INDEX IF NOT EXISTS idx_recommendation_runs_retention
    ON recommendation_runs(retention_until);

