package app

import (
	"errors"
	"fmt"
	"strings"
)

type AIConfig struct {
	Raw                string
	Version            string
	DefaultBranch      string
	TargetBranchPrefix string
	AnalysisEnabled    bool
	IncludePatterns    []string
	ExcludePatterns    []string
	CodeGeneration     bool
	RequireApproval    bool
	ReviewerPolicy     string
	ApproveCommand     string
	CorrectCommand     string
	RejectCommand      string
}

func ParseAIConfig(raw string) (AIConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return AIConfig{}, errors.New("missing .yml configuration")
	}

	doc := parseSimpleYAML(raw)
	cfg := AIConfig{
		Raw:                raw,
		Version:            scalarOrDefault(doc, "version", "1"),
		DefaultBranch:      scalarOrDefault(doc, "repository.default_branch", "main"),
		TargetBranchPrefix: scalarOrDefault(doc, "repository.target_branch_prefix", "ai/"),
		AnalysisEnabled:    boolOrDefault(doc, "analysis.enabled", true),
		IncludePatterns:    listOrDefault(doc, "analysis.include", []string{"src/**"}),
		ExcludePatterns:    listOrDefault(doc, "analysis.exclude", []string{"node_modules/**", "dist/**", "build/**", ".git/**"}),
		CodeGeneration:     boolOrDefault(doc, "code_generation.enabled", true),
		RequireApproval:    boolOrDefault(doc, "code_generation.require_user_approval", true),
		ReviewerPolicy:     scalarOrDefault(doc, "code_generation.reviewer_policy", "issue_author"),
		ApproveCommand:     scalarOrDefault(doc, "code_generation.allowed_actions.approve_command", "/approve"),
		CorrectCommand:     scalarOrDefault(doc, "code_generation.allowed_actions.correct_command", "/correct"),
		RejectCommand:      scalarOrDefault(doc, "code_generation.allowed_actions.reject_command", "/reject"),
	}

	if err := validateAIConfig(cfg); err != nil {
		return AIConfig{}, err
	}
	return cfg, nil
}

func validateAIConfig(cfg AIConfig) error {
	switch {
	case cfg.Version != "1":
		return fmt.Errorf("unsupported .yml version %q", cfg.Version)
	case !cfg.AnalysisEnabled:
		return errors.New("repository analysis is disabled in .yml configuration")
	case strings.TrimSpace(cfg.DefaultBranch) == "":
		return errors.New("repository.default_branch is required")
	case strings.TrimSpace(cfg.TargetBranchPrefix) == "":
		return errors.New("repository.target_branch_prefix is required")
	case len(cfg.IncludePatterns) == 0:
		return errors.New("analysis.include must contain at least one pattern")
	case !cfg.CodeGeneration:
		return errors.New("code generation is disabled in .yml configuration")
	case cfg.RequireApproval == false:
		return errors.New("code_generation.require_user_approval must be true for Sprint 1")
	case cfg.ReviewerPolicy != "issue_author":
		return fmt.Errorf("unsupported reviewer policy %q", cfg.ReviewerPolicy)
	case !strings.HasPrefix(cfg.ApproveCommand, "/"):
		return errors.New("approve command must start with /")
	case !strings.HasPrefix(cfg.CorrectCommand, "/"):
		return errors.New("correct command must start with /")
	case !strings.HasPrefix(cfg.RejectCommand, "/"):
		return errors.New("reject command must start with /")
	default:
		return nil
	}
}

type simpleYAMLDoc struct {
	scalars map[string]string
	lists   map[string][]string
}

func parseSimpleYAML(raw string) simpleYAMLDoc {
	doc := simpleYAMLDoc{
		scalars: make(map[string]string),
		lists:   make(map[string][]string),
	}
	var stack []string
	var currentList string

	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		indent := leadingSpaces(line) / 2
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			if currentList != "" {
				doc.lists[currentList] = append(doc.lists[currentList], cleanYAMLValue(strings.TrimPrefix(trimmed, "- ")))
			}
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = cleanYAMLValue(value)
		if indent < len(stack) {
			stack = stack[:indent]
		}
		for len(stack) < indent {
			stack = append(stack, "")
		}

		pathParts := append(append([]string{}, stack...), key)
		path := strings.Join(pathParts, ".")
		if value == "" {
			if indent == len(stack) {
				stack = append(stack, key)
			} else {
				stack[indent] = key
			}
			currentList = path
			continue
		}

		doc.scalars[path] = value
		currentList = ""
	}

	return doc
}

func scalarOrDefault(doc simpleYAMLDoc, key, fallback string) string {
	if value, ok := doc.scalars[key]; ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func boolOrDefault(doc simpleYAMLDoc, key string, fallback bool) bool {
	value, ok := doc.scalars[key]
	if !ok {
		return fallback
	}
	return strings.EqualFold(value, "true")
}

func listOrDefault(doc simpleYAMLDoc, key string, fallback []string) []string {
	if values, ok := doc.lists[key]; ok && len(values) > 0 {
		return append([]string(nil), values...)
	}
	return append([]string(nil), fallback...)
}

func leadingSpaces(line string) int {
	count := 0
	for _, r := range line {
		if r != ' ' {
			break
		}
		count++
	}
	return count
}

func cleanYAMLValue(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = value[:idx]
	}
	return strings.Trim(strings.TrimSpace(value), `"'`)
}
