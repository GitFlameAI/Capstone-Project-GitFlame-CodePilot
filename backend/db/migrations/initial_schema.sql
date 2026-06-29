CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS repositories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    owner TEXT NOT NULL DEFAULT 'gitflame',
    default_branch TEXT NOT NULL DEFAULT 'main',
    web_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
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

CREATE TABLE IF NOT EXISTS ai_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    raw_yml TEXT NOT NULL,
    parsed_config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_valid BOOLEAN NOT NULL DEFAULT false,
    retention_days INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ai_configs_retention_days_check CHECK (
        retention_days BETWEEN 1 AND 365
    )
);

CREATE TABLE IF NOT EXISTS issue_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    ai_config_id UUID NOT NULL REFERENCES ai_configs(id),
    external_issue_id TEXT NOT NULL,
    issue_title TEXT NOT NULL,
    issue_body TEXT NOT NULL DEFAULT '',
    issue_author TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'created',
    current_revision INTEGER NOT NULL DEFAULT 0,
    git_workflow_json JSONB,
    request_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT issue_sessions_status_check CHECK (
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
    ),
    CONSTRAINT issue_sessions_current_revision_check CHECK (current_revision >= 0),
    CONSTRAINT issue_sessions_repository_issue_unique UNIQUE (
        repository_id,
        external_issue_id
    )
);

CREATE TABLE IF NOT EXISTS generated_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_session_id UUID NOT NULL UNIQUE REFERENCES issue_sessions(id) ON DELETE CASCADE,
    plan_markdown TEXT NOT NULL,
    current_revision INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT generated_plans_current_revision_check CHECK (current_revision >= 1)
);

CREATE TABLE IF NOT EXISTS agent_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_session_id UUID REFERENCES issue_sessions(id) ON DELETE CASCADE,
    generated_plan_id UUID REFERENCES generated_plans(id) ON DELETE CASCADE,
    task_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    error_message TEXT NOT NULL DEFAULT '',
    tool_execution_summary TEXT NOT NULL DEFAULT '',
    attempt INTEGER NOT NULL DEFAULT 1,
    error_json JSONB,
    relevant_files JSONB NOT NULL DEFAULT '[]'::jsonb,
    model TEXT NOT NULL DEFAULT '',
    usage_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_tasks_type_check CHECK (
        task_type IN (
            'initial_plan',
            'plan_revision',
            'code_generation',
            'recommendation_analysis'
        )
    ),
    CONSTRAINT agent_tasks_status_check CHECK (
        status IN ('queued', 'processing', 'completed', 'failed')
    ),
    CONSTRAINT agent_tasks_attempt_positive CHECK (attempt > 0)
);

CREATE TABLE IF NOT EXISTS agent_task_statuses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_task_id UUID NOT NULL REFERENCES agent_tasks(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_task_statuses_status_check CHECK (
        status IN ('queued', 'processing', 'completed', 'failed')
    )
);

CREATE TABLE IF NOT EXISTS plan_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    generated_plan_id UUID NOT NULL REFERENCES generated_plans(id) ON DELETE CASCADE,
    issue_session_id UUID NOT NULL REFERENCES issue_sessions(id) ON DELETE CASCADE,
    agent_task_id UUID REFERENCES agent_tasks(id) ON DELETE SET NULL,
    revision_number INTEGER NOT NULL,
    plan_markdown TEXT NOT NULL,
    correction_feedback TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'initial',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT plan_revisions_number_check CHECK (revision_number >= 1),
    CONSTRAINT plan_revisions_source_check CHECK (
        source IN ('initial', 'correction', 'retry', 'user_edit')
    ),
    CONSTRAINT plan_revisions_unique_revision UNIQUE (
        generated_plan_id,
        revision_number
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

CREATE TABLE IF NOT EXISTS user_responses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_session_id UUID NOT NULL REFERENCES issue_sessions(id) ON DELETE CASCADE,
    response_type TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT user_responses_type_check CHECK (
        response_type IN ('approve', 'correct', 'reject')
    )
);

CREATE TABLE IF NOT EXISTS recommendation_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    ai_config_id UUID NOT NULL REFERENCES ai_configs(id),
    summary TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    retention_days INTEGER NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT recommendation_runs_status_check CHECK (
        status IN ('pending', 'completed', 'failed')
    ),
    CONSTRAINT recommendation_runs_retention_days_check CHECK (
        retention_days BETWEEN 1 AND 365
    )
);

CREATE TABLE IF NOT EXISTS recommendations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recommendation_run_id UUID NOT NULL REFERENCES recommendation_runs(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    line_number INTEGER,
    category TEXT NOT NULL,
    severity TEXT NOT NULL,
    problem TEXT NOT NULL,
    suggestion TEXT NOT NULL,
    confidence DOUBLE PRECISION,
    current_status TEXT NOT NULL DEFAULT 'open',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT recommendations_line_number_check CHECK (
        line_number IS NULL OR line_number > 0
    ),
    CONSTRAINT recommendations_confidence_check CHECK (
        confidence IS NULL OR (confidence >= 0 AND confidence <= 1)
    ),
    CONSTRAINT recommendations_current_status_check CHECK (
        current_status IN ('open', 'closed', 'deleted')
    )
);

CREATE TABLE IF NOT EXISTS recommendation_statuses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recommendation_id UUID NOT NULL REFERENCES recommendations(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    changed_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT recommendation_statuses_status_check CHECK (
        status IN ('open', 'closed', 'deleted')
    )
);

CREATE INDEX IF NOT EXISTS idx_repositories_external_id
    ON repositories(external_id);

CREATE INDEX IF NOT EXISTS idx_repository_files_repository_id
    ON repository_files(repository_id);

CREATE INDEX IF NOT EXISTS idx_repository_files_file_path
    ON repository_files(file_path);

CREATE INDEX IF NOT EXISTS idx_ai_configs_repository_id
    ON ai_configs(repository_id);

CREATE INDEX IF NOT EXISTS idx_issue_sessions_repository_id
    ON issue_sessions(repository_id);

CREATE INDEX IF NOT EXISTS idx_issue_sessions_external_issue_id
    ON issue_sessions(external_issue_id);

CREATE INDEX IF NOT EXISTS idx_issue_sessions_ai_config_id
    ON issue_sessions(ai_config_id);

CREATE INDEX IF NOT EXISTS idx_generated_plans_issue_session_id
    ON generated_plans(issue_session_id);

CREATE INDEX IF NOT EXISTS idx_agent_tasks_issue_session_id
    ON agent_tasks(issue_session_id);

CREATE INDEX IF NOT EXISTS idx_agent_tasks_status
    ON agent_tasks(status);

CREATE INDEX IF NOT EXISTS idx_agent_task_statuses_task_id
    ON agent_task_statuses(agent_task_id);

CREATE INDEX IF NOT EXISTS idx_plan_revisions_issue_session_id
    ON plan_revisions(issue_session_id);

CREATE INDEX IF NOT EXISTS idx_plan_revisions_generated_plan_id
    ON plan_revisions(generated_plan_id);

CREATE INDEX IF NOT EXISTS idx_git_workflow_payloads_issue_session_id
    ON git_workflow_payloads(issue_session_id);

CREATE INDEX IF NOT EXISTS idx_generated_files_issue_session_id
    ON generated_files(issue_session_id);

CREATE INDEX IF NOT EXISTS idx_generated_files_agent_task_id
    ON generated_files(agent_task_id);

CREATE INDEX IF NOT EXISTS idx_user_responses_issue_session_id
    ON user_responses(issue_session_id);

CREATE INDEX IF NOT EXISTS idx_recommendation_runs_repository_id
    ON recommendation_runs(repository_id);

CREATE INDEX IF NOT EXISTS idx_recommendation_runs_ai_config_id
    ON recommendation_runs(ai_config_id);

CREATE INDEX IF NOT EXISTS idx_recommendation_runs_expires_at
    ON recommendation_runs(expires_at);

CREATE INDEX IF NOT EXISTS idx_recommendations_run_id
    ON recommendations(recommendation_run_id);

CREATE INDEX IF NOT EXISTS idx_recommendation_statuses_recommendation_id
    ON recommendation_statuses(recommendation_id);
