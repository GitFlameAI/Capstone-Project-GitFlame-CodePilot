package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"gitflame-codepilot/backend/internal/domain"
)

func ParseAIConfig(raw string) (domain.AIConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return domain.AIConfig{}, errors.New("missing .yml configuration")
	}
	var document any
	if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
		return domain.AIConfig{}, fmt.Errorf("invalid YAML configuration: %w", err)
	}
	if _, ok := document.(map[string]any); !ok {
		return domain.AIConfig{}, errors.New("YAML configuration must contain a mapping")
	}
	if containsModelSelection(document) {
		return domain.AIConfig{}, errors.New("model selection is operator-controlled")
	}
	doc := parseSimpleYAML(raw)
	retentionDays, err := strictInteger(doc, "storage.recommendation_ttl_days", 30)
	if err != nil {
		return domain.AIConfig{}, err
	}
	cfg := domain.AIConfig{
		Raw:                raw,
		Version:            scalar(doc, "version", "1"),
		DefaultBranch:      scalar(doc, "repository.default_branch", "main"),
		TargetBranchPrefix: scalar(doc, "repository.target_branch_prefix", "ai/"),
		AnalysisEnabled:    boolean(doc, "analysis.enabled", true),
		RequireApproval:    boolean(doc, "code_generation.require_user_approval", true),
		IncludePatterns:    list(doc, "analysis.include", []string{"**/*"}),
		ExcludePatterns:    list(doc, "analysis.exclude", []string{".git/**", "node_modules/**", "dist/**", "build/**"}),
		MaxFiles:           integer(doc, "analysis.max_files", 20),
		MaxSnippetsPerFile: integer(doc, "analysis.max_snippets_per_file", 3),
		RetentionDays:      retentionDays,
		ReviewerPolicy:     scalar(doc, "code_generation.reviewer_policy", "issue_author"),
		ApproveCommand:     scalar(doc, "code_generation.allowed_actions.approve_command", "/approve"),
		CorrectCommand:     scalar(doc, "code_generation.allowed_actions.correct_command", "/correct"),
		RejectCommand:      scalar(doc, "code_generation.allowed_actions.reject_command", "/reject"),
	}
	if !cfg.AnalysisEnabled {
		return cfg, errors.New("repository analysis is disabled in .yml configuration")
	}
	if strings.TrimSpace(cfg.DefaultBranch) == "" || strings.TrimSpace(cfg.TargetBranchPrefix) == "" {
		return cfg, errors.New("repository branch configuration is required")
	}
	if cfg.RetentionDays < 1 || cfg.RetentionDays > 365 {
		return cfg, errors.New("storage.recommendation_ttl_days must be between 1 and 365")
	}
	return cfg, nil
}

type yamlDoc struct {
	scalars map[string]string
	lists   map[string][]string
}

func parseSimpleYAML(raw string) yamlDoc {
	d := yamlDoc{map[string]string{}, map[string][]string{}}
	var root map[string]any
	if err := yaml.Unmarshal([]byte(raw), &root); err == nil {
		flattenYAML(d, "", root)
	}
	return d
}

func flattenYAML(document yamlDoc, prefix string, value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			flattenYAML(document, path, nested)
		}
	case []any:
		values := make([]string, 0, len(typed))
		for _, nested := range typed {
			values = append(values, fmt.Sprint(nested))
		}
		document.lists[prefix] = values
	case nil:
		return
	default:
		document.scalars[prefix] = fmt.Sprint(typed)
	}
}
func scalar(d yamlDoc, k, f string) string {
	if v, ok := d.scalars[k]; ok && v != "" {
		return v
	}
	return f
}
func boolean(d yamlDoc, k string, f bool) bool {
	v, ok := d.scalars[k]
	if !ok {
		return f
	}
	return strings.EqualFold(v, "true")
}
func list(d yamlDoc, k string, f []string) []string {
	if v, ok := d.lists[k]; ok && len(v) > 0 {
		return append([]string(nil), v...)
	}
	return append([]string(nil), f...)
}

func integer(d yamlDoc, key string, fallback int) int {
	value, ok := d.scalars[key]
	if !ok {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func strictInteger(d yamlDoc, key string, fallback int) (int, error) {
	value, ok := d.scalars[key]
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return parsed, nil
}

func containsModelSelection(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			normalized := strings.ReplaceAll(strings.ToLower(key), "-", "_")
			switch normalized {
			case "model", "model_id", "agent_model", "llm_model":
				return true
			}
			if containsModelSelection(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if containsModelSelection(nested) {
				return true
			}
		}
	}
	return false
}
