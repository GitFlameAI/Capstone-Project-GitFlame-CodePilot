ALTER TABLE issue_sessions DROP CONSTRAINT IF EXISTS issue_sessions_status_check;
ALTER TABLE issue_sessions
    ADD CONSTRAINT issue_sessions_status_check CHECK (
        status IN (
            'created',
            'queued',
            'processing',
            'plan_generated',
            'approved',
            'code_generation_queued',
            'code_generation_processing',
            'code_generated',
            'correction_requested',
            'rejected',
            'failed'
        )
    );

ALTER TABLE agent_tasks DROP CONSTRAINT IF EXISTS agent_tasks_type_check;
ALTER TABLE agent_tasks
    ADD CONSTRAINT agent_tasks_type_check CHECK (
        task_type IN (
            'initial_plan',
            'plan_revision',
            'code_generation',
            'recommendation_analysis'
        )
    );

ALTER TABLE plan_revisions DROP CONSTRAINT IF EXISTS plan_revisions_source_check;
ALTER TABLE plan_revisions
    ADD CONSTRAINT plan_revisions_source_check CHECK (
        source IN ('initial', 'correction', 'retry', 'user_edit')
    );

CREATE TABLE IF NOT EXISTS repository_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    file_name TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL DEFAULT '',
    commit_sha TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT repository_files_repository_path_unique UNIQUE (
        repository_id,
        file_path
    )
);

CREATE TABLE IF NOT EXISTS git_workflow_payloads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_session_id UUID NOT NULL UNIQUE REFERENCES issue_sessions(id) ON DELETE CASCADE,
    agent_task_id UUID REFERENCES agent_tasks(id) ON DELETE SET NULL,
    branch_name TEXT NOT NULL,
    base_branch TEXT NOT NULL DEFAULT 'main',
    commit_message TEXT NOT NULL,
    pr_title TEXT NOT NULL,
    reviewer TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT git_workflow_payloads_status_check CHECK (
        status IN ('pending', 'generated', 'applied', 'failed')
    )
);

CREATE TABLE IF NOT EXISTS generated_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_session_id UUID NOT NULL REFERENCES issue_sessions(id) ON DELETE CASCADE,
    agent_task_id UUID REFERENCES agent_tasks(id) ON DELETE SET NULL,
    file_path TEXT NOT NULL,
    action TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    diff TEXT NOT NULL DEFAULT '',
    explanation TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    validation_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT generated_files_action_check CHECK (
        action IN ('create', 'modify', 'delete')
    ),
    CONSTRAINT generated_files_status_check CHECK (
        status IN ('pending', 'valid', 'invalid', 'applied')
    )
);

CREATE INDEX IF NOT EXISTS idx_repository_files_repository_id
    ON repository_files(repository_id);

CREATE INDEX IF NOT EXISTS idx_repository_files_file_path
    ON repository_files(file_path);

CREATE INDEX IF NOT EXISTS idx_git_workflow_payloads_issue_session_id
    ON git_workflow_payloads(issue_session_id);

CREATE INDEX IF NOT EXISTS idx_generated_files_issue_session_id
    ON generated_files(issue_session_id);

CREATE INDEX IF NOT EXISTS idx_generated_files_agent_task_id
    ON generated_files(agent_task_id);
