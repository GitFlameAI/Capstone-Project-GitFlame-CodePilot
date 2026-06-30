package service

import "testing"

func TestParseAIConfigRejectsInvalidYAMLAndModelSelection(t *testing.T) {
	if _, err := ParseAIConfig("analysis: ["); err == nil {
		t.Fatal("expected malformed YAML to be rejected")
	}
	if _, err := ParseAIConfig("version: 1\nagent_model: attacker/model\n"); err == nil {
		t.Fatal("expected repository-controlled model selection to be rejected")
	}
	if _, err := ParseAIConfig("storage:\n  recommendation_ttl_days: nope\n"); err == nil {
		t.Fatal("expected invalid retention period to be rejected")
	}
}

func TestParseAIConfigSupportsSprint3Spec(t *testing.T) {
	cfg, err := ParseAIConfig(`repository:
  default_branch: develop
analysis:
  enabled: true
  exclude:
    - node_modules/**
    - dist/**
recommendations:
  enabled: true
  categories:
    - security
storage:
  recommendation_ttl_days: 14
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultBranch != "develop" {
		t.Fatalf("unexpected default branch: %s", cfg.DefaultBranch)
	}
	if cfg.RetentionDays != 14 {
		t.Fatalf("unexpected retention days: %d", cfg.RetentionDays)
	}
	if len(cfg.ExcludePatterns) != 2 || cfg.ExcludePatterns[0] != "node_modules/**" || cfg.ExcludePatterns[1] != "dist/**" {
		t.Fatalf("unexpected exclude patterns: %#v", cfg.ExcludePatterns)
	}
}
