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
		"CREATE TABLE IF NOT EXISTS generated_plans",
		"CREATE TABLE IF NOT EXISTS agent_tasks",
		"CREATE TABLE IF NOT EXISTS plan_revisions",
		"CREATE TABLE IF NOT EXISTS repository_files",
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
		"validation_error TEXT",
		"retention_days INTEGER",
		"expires_at TIMESTAMPTZ",
	} {
		if !strings.Contains(schema, required) {
			t.Errorf("schema misses %q", required)
		}
	}
}
