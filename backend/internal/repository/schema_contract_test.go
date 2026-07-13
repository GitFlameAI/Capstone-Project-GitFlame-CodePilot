package repository

import (
	"os"
	"strings"
	"testing"
)

func TestDatabaseSchemaContainsBackendWorkerContract(t *testing.T) {
	content, err := os.ReadFile("../../db/migrations/initial_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(content)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS app_users",
		"CREATE TABLE IF NOT EXISTS app_sessions",
		"CREATE TABLE IF NOT EXISTS generated_plans",
		"CREATE TABLE IF NOT EXISTS agent_tasks",
		"CREATE TABLE IF NOT EXISTS plan_revisions",
		"CREATE TABLE IF NOT EXISTS repository_files",
		"CREATE TABLE IF NOT EXISTS gitflame_connections",
		"CREATE TABLE IF NOT EXISTS gitflame_webhooks",
		"CREATE TABLE IF NOT EXISTS gitflame_webhook_events",
		"CREATE TABLE IF NOT EXISTS repository_snapshots",
		"CREATE TABLE IF NOT EXISTS repository_snapshot_files",
		"CREATE TABLE IF NOT EXISTS generated_files",
		"CREATE TABLE IF NOT EXISTS git_workflow_payloads",
		"request_json JSONB",
		"attempt INTEGER",
		"error_json JSONB",
		"relevant_files JSONB",
		"usage_json JSONB",
		"'initial_plan'",
		"'plan_revision'",
		"'code_generation'",
		"'code_generation_queued'",
		"'code_generation_processing'",
		"'code_generated'",
		"'user_edit'",
		"branch_name TEXT",
		"base_branch TEXT",
		"commit_message TEXT",
		"pr_title TEXT",
		"reviewer TEXT",
		"access_token_encrypted TEXT",
		"access_token_ciphertext BYTEA",
		"access_token_nonce BYTEA",
		"encryption_key_version INTEGER",
		"token_hash BYTEA",
		"user_id UUID REFERENCES app_users",
		"scopes JSONB",
		"token_expires_at TIMESTAMPTZ",
		"last_validated_at TIMESTAMPTZ",
		"last_used_at TIMESTAMPTZ",
		"revoked_at TIMESTAMPTZ",
		"gitflame_connections_user_repository_unique",
		"idx_gitflame_webhook_events_delivery_unique",
		"token_last4 TEXT",
		"webhook_secret_hash TEXT",
		"delivery_id TEXT",
		"payload_json JSONB",
		"pull_request_url TEXT",
		"apply_error TEXT",
		"applied_at TIMESTAMPTZ",
		"validation_error TEXT",
		"retention_days INTEGER",
		"expires_at TIMESTAMPTZ",
	} {
		if !strings.Contains(schema, required) {
			t.Errorf("schema misses %q", required)
		}
	}
}
