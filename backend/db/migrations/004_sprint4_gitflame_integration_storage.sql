CREATE TABLE IF NOT EXISTS gitflame_connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    repo_url TEXT NOT NULL,
    default_branch TEXT NOT NULL DEFAULT 'main',
    access_token_encrypted TEXT NOT NULL,
    token_last4 TEXT NOT NULL DEFAULT '',
    token_status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT gitflame_connections_repository_unique UNIQUE (repository_id),
    CONSTRAINT gitflame_connections_token_status_check CHECK (
        token_status IN ('active', 'invalid', 'revoked')
    )
);

CREATE TABLE IF NOT EXISTS gitflame_webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    connection_id UUID NOT NULL REFERENCES gitflame_connections(id) ON DELETE CASCADE,
    webhook_url TEXT NOT NULL,
    webhook_secret_hash TEXT NOT NULL DEFAULT '',
    events JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL DEFAULT 'pending',
    external_webhook_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT gitflame_webhooks_connection_url_unique UNIQUE (
        connection_id,
        webhook_url
    ),
    CONSTRAINT gitflame_webhooks_status_check CHECK (
        status IN ('pending', 'active', 'disabled', 'failed')
    )
);

CREATE TABLE IF NOT EXISTS gitflame_webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL REFERENCES gitflame_webhooks(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    action TEXT NOT NULL DEFAULT '',
    delivery_id TEXT NOT NULL DEFAULT '',
    repository_external_id TEXT NOT NULL DEFAULT '',
    issue_session_id UUID REFERENCES issue_sessions(id) ON DELETE SET NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'received',
    error_json JSONB,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ,
    CONSTRAINT gitflame_webhook_events_status_check CHECK (
        status IN ('received', 'processed', 'ignored', 'failed')
    )
);

CREATE TABLE IF NOT EXISTS repository_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    connection_id UUID REFERENCES gitflame_connections(id) ON DELETE SET NULL,
    ref TEXT NOT NULL DEFAULT '',
    commit_sha TEXT NOT NULL DEFAULT '',
    ai_config_id UUID REFERENCES ai_configs(id) ON DELETE SET NULL,
    file_count INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'fetched',
    error_json JSONB,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT repository_snapshots_file_count_check CHECK (file_count >= 0),
    CONSTRAINT repository_snapshots_status_check CHECK (
        status IN ('fetched', 'failed')
    )
);

CREATE TABLE IF NOT EXISTS repository_snapshot_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_snapshot_id UUID NOT NULL REFERENCES repository_snapshots(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    content_hash TEXT NOT NULL DEFAULT '',
    commit_sha TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT repository_snapshot_files_snapshot_path_unique UNIQUE (
        repository_snapshot_id,
        file_path
    )
);

ALTER TABLE git_workflow_payloads
    ADD COLUMN IF NOT EXISTS commit_sha TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pull_request_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pull_request_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS apply_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS applied_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_gitflame_connections_repository_id
    ON gitflame_connections(repository_id);

CREATE INDEX IF NOT EXISTS idx_gitflame_webhooks_connection_id
    ON gitflame_webhooks(connection_id);

CREATE INDEX IF NOT EXISTS idx_gitflame_webhook_events_webhook_id
    ON gitflame_webhook_events(webhook_id);

CREATE INDEX IF NOT EXISTS idx_gitflame_webhook_events_delivery_id
    ON gitflame_webhook_events(delivery_id);

CREATE INDEX IF NOT EXISTS idx_gitflame_webhook_events_issue_session_id
    ON gitflame_webhook_events(issue_session_id);

CREATE INDEX IF NOT EXISTS idx_repository_snapshots_repository_id
    ON repository_snapshots(repository_id);

CREATE INDEX IF NOT EXISTS idx_repository_snapshots_connection_id
    ON repository_snapshots(connection_id);

CREATE INDEX IF NOT EXISTS idx_repository_snapshot_files_snapshot_id
    ON repository_snapshot_files(repository_snapshot_id);
