BEGIN;

CREATE TABLE IF NOT EXISTS app_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gitflame_user_id TEXT NOT NULL UNIQUE,
    username TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS app_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT app_sessions_token_hash_length_check CHECK (
        octet_length(token_hash) = 32
    ),
    CONSTRAINT app_sessions_expiration_check CHECK (
        expires_at > created_at
    )
);

ALTER TABLE gitflame_connections
    ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES app_users(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS access_token_ciphertext BYTEA,
    ADD COLUMN IF NOT EXISTS access_token_nonce BYTEA,
    ADD COLUMN IF NOT EXISTS encryption_key_version INTEGER,
    ADD COLUMN IF NOT EXISTS scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS token_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_validated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ;

-- Legacy rows keep access_token_encrypted until the backend re-encrypts them.
ALTER TABLE gitflame_connections
    ALTER COLUMN access_token_encrypted DROP NOT NULL,
    DROP CONSTRAINT IF EXISTS gitflame_connections_repository_unique,
    DROP CONSTRAINT IF EXISTS gitflame_connections_token_status_check,
    DROP CONSTRAINT IF EXISTS gitflame_connections_user_repository_unique,
    DROP CONSTRAINT IF EXISTS gitflame_connections_token_material_check,
    DROP CONSTRAINT IF EXISTS gitflame_connections_key_version_check;

ALTER TABLE gitflame_connections
    ADD CONSTRAINT gitflame_connections_user_repository_unique
        UNIQUE (user_id, repository_id),
    ADD CONSTRAINT gitflame_connections_token_status_check CHECK (
        token_status IN ('active', 'invalid', 'expired', 'revoked', 'reauth_required')
    ),
    ADD CONSTRAINT gitflame_connections_token_material_check CHECK (
        access_token_encrypted IS NOT NULL
        OR (
            access_token_ciphertext IS NOT NULL
            AND access_token_nonce IS NOT NULL
            AND encryption_key_version IS NOT NULL
        )
    ),
    ADD CONSTRAINT gitflame_connections_key_version_check CHECK (
        encryption_key_version IS NULL OR encryption_key_version > 0
    );

CREATE INDEX IF NOT EXISTS idx_app_sessions_user_id
    ON app_sessions(user_id);

CREATE INDEX IF NOT EXISTS idx_app_sessions_expires_at
    ON app_sessions(expires_at);

CREATE INDEX IF NOT EXISTS idx_gitflame_connections_user_id
    ON gitflame_connections(user_id);

CREATE INDEX IF NOT EXISTS idx_gitflame_connections_token_status
    ON gitflame_connections(token_status);

CREATE INDEX IF NOT EXISTS idx_gitflame_connections_token_expires_at
    ON gitflame_connections(token_expires_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_gitflame_webhook_events_delivery_unique
    ON gitflame_webhook_events(webhook_id, delivery_id)
    WHERE delivery_id <> '';

COMMIT;
