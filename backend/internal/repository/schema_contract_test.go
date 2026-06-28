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
		"retention_days INTEGER",
		"expires_at TIMESTAMPTZ",
	} {
		if !strings.Contains(schema, required) {
			t.Errorf("schema misses %q", required)
		}
	}
}
