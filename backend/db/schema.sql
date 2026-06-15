CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS repositories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    owner TEXT NOT NULL,
    default_branch TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT repositories_owner_name_unique UNIQUE (owner, name)
);

CREATE TABLE IF NOT EXISTS ai_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    raw_yml TEXT NOT NULL,
    parsed_config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_valid BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS issue_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    ai_config_id UUID NOT NULL REFERENCES ai_configs(id),
    issue_title TEXT NOT NULL,
    issue_body TEXT NOT NULL DEFAULT '',
    issue_author TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'created',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT issue_sessions_status_check CHECK (
        status IN (
            'created',
            'plan_generated',
            'approved',
            'correction_requested',
            'rejected'
        )
    )
);

CREATE TABLE IF NOT EXISTS generated_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_session_id UUID NOT NULL REFERENCES issue_sessions(id) ON DELETE CASCADE,
    plan_markdown TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT recommendation_runs_status_check CHECK (
        status IN ('pending', 'completed', 'failed')
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
    current_status TEXT NOT NULL DEFAULT 'open',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT recommendations_line_number_check CHECK (
        line_number IS NULL OR line_number > 0
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

CREATE INDEX IF NOT EXISTS idx_ai_configs_repository_id
    ON ai_configs(repository_id);

CREATE INDEX IF NOT EXISTS idx_issue_sessions_repository_id
    ON issue_sessions(repository_id);

CREATE INDEX IF NOT EXISTS idx_issue_sessions_ai_config_id
    ON issue_sessions(ai_config_id);

CREATE INDEX IF NOT EXISTS idx_generated_plans_issue_session_id
    ON generated_plans(issue_session_id);

CREATE INDEX IF NOT EXISTS idx_user_responses_issue_session_id
    ON user_responses(issue_session_id);

CREATE INDEX IF NOT EXISTS idx_recommendation_runs_repository_id
    ON recommendation_runs(repository_id);

CREATE INDEX IF NOT EXISTS idx_recommendation_runs_ai_config_id
    ON recommendation_runs(ai_config_id);

CREATE INDEX IF NOT EXISTS idx_recommendations_run_id
    ON recommendations(recommendation_run_id);

CREATE INDEX IF NOT EXISTS idx_recommendation_statuses_recommendation_id
    ON recommendation_statuses(recommendation_id);
