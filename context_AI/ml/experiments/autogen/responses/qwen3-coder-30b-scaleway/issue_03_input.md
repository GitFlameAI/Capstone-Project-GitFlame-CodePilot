# Issue
Title: Add snippet limits to the repository AI configuration

Extend the repository .yml configuration with RAG limits for maximum retrieved files and maximum snippets per file. Validate positive bounds and expose the parsed values to the issue analysis flow. The user must not choose a model or plan format, and allowed_actions must not be part of the public configuration.

# Evaluation Contract
# Implementation Plan Format

Every issue-to-plan response must be valid Markdown and use the sections below in the same order.

```markdown
# Implementation Plan

## Issue Summary
Brief restatement of the requested change.

## Goal
The expected product or technical outcome.

## Relevant Files
- `path/to/existing_file`: why it is relevant.
- `path/to/new_file` (create): why it is needed.

## Proposed Changes
- Concrete behavior or interface changes.

## Implementation Steps
1. Ordered implementation step.
2. Ordered implementation step.

## Expected Files to Change
- `path/to/file`: create or modify.

## Tests and Verification
- Test or verification step.

## Risks and Open Questions
- Risk, dependency, or `TBD` when repository context is insufficient.
```

## Rules

- Return only the implementation plan, without source code or patches.
- Reference only files present in the supplied repository context.
- Mark proposed new files with `(create)`.
- Use `TBD` instead of inventing missing repository details.
- Keep steps concrete, ordered, and testable.



# Repository Context
## File: `backend/internal/app/config_service.go`
```text
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

```

## File: `backend/internal/app/models.go`
```text
package app

type RepositoryMetadata struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	DefaultBranch string `json:"default_branch"`
	WebURL        string `json:"web_url,omitempty"`
}

type IssuePayload struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Author string `json:"author"`
}

type IssueAnalyzeRequest struct {
	Repository        RepositoryMetadata `json:"repository"`
	Issue             IssuePayload       `json:"issue"`
	YAMLConfig        string             `json:"yaml_config"`
	RepositoryContext []string           `json:"repository_context"`
	Metadata          map[string]any     `json:"metadata,omitempty"`
}

type GitWorkflowResponse struct {
	BranchName     string `json:"branch_name"`
	PullRequestURL string `json:"pull_request_url"`
	Reviewer       string `json:"reviewer"`
	Provider       string `json:"provider"`
}

type IssueAnalyzeResponse struct {
	SessionID    string            `json:"session_id"`
	IssueID      string            `json:"issue_id"`
	RepositoryID string            `json:"repository_id"`
	Status       string            `json:"status"`
	PlanMarkdown string            `json:"plan_markdown"`
	CommentBody  string            `json:"comment_body"`
	NextActions  map[string]string `json:"next_actions"`
}

type IssuePlanResponse struct {
	SessionID    string `json:"session_id"`
	IssueID      string `json:"issue_id"`
	RepositoryID string `json:"repository_id"`
	Status       string `json:"status"`
	PlanMarkdown string `json:"plan_markdown"`
	CommentBody  string `json:"comment_body"`
	Revision     int    `json:"revision"`
}

type PlanCorrectionRequest struct {
	Feedback string `json:"feedback"`
}

type PlanActionResponse struct {
	SessionID    string               `json:"session_id"`
	IssueID      string               `json:"issue_id"`
	Status       string               `json:"status"`
	Message      string               `json:"message"`
	PlanMarkdown string               `json:"plan_markdown,omitempty"`
	GitWorkflow  *GitWorkflowResponse `json:"git_workflow,omitempty"`
}

type RecommendationAnalyzeRequest struct {
	Repository        RepositoryMetadata `json:"repository"`
	YAMLConfig        string             `json:"yaml_config"`
	RepositoryContext []string           `json:"repository_context"`
}

type RecommendationCard struct {
	ID         string   `json:"id"`
	Severity   string   `json:"severity"`
	File       string   `json:"file"`
	Line       *int     `json:"line,omitempty"`
	Problem    string   `json:"problem"`
	Suggestion string   `json:"suggestion"`
	Confidence *float64 `json:"confidence,omitempty"`
	State      string   `json:"state"`
}

type RecommendationAnalyzeResponse struct {
	RepositoryID    string               `json:"repository_id"`
	Status          string               `json:"status"`
	Summary         string               `json:"summary"`
	Recommendations []RecommendationCard `json:"recommendations"`
}

type RecommendationStatusResponse struct {
	RepositoryID string `json:"repository_id"`
	Status       string `json:"status"`
	Total        int    `json:"total"`
	Open         int    `json:"open"`
	Closed       int    `json:"closed"`
}

type RecommendationSummaryResponse struct {
	RepositoryID string `json:"repository_id"`
	Summary      string `json:"summary"`
}

type RecommendationListResponse struct {
	RepositoryID    string               `json:"repository_id"`
	Recommendations []RecommendationCard `json:"recommendations"`
}

```

## File: `backend/models/ai_config.go`
```text
package models

import "encoding/json"

type AIConfig struct {
	ID               UUID            `db:"id" json:"id"`
	RepositoryID     UUID            `db:"repository_id" json:"repository_id"`
	RawYML           string          `db:"raw_yml" json:"raw_yml"`
	ParsedConfigJSON json.RawMessage `db:"parsed_config_json" json:"parsed_config_json"`
	IsValid          bool            `db:"is_valid" json:"is_valid"`
	Timestamps
}

```

## File: `docs/config/ai_config.example.yml`
```text
version: 1

repository:
  default_branch: main
  target_branch_prefix: ai/

analysis:
  enabled: true
  include:
    - src/**
    - internal/**
  exclude:
    - node_modules/**
    - dist/**
    - build/**
    - .git/**

code_generation:
  enabled: true
  require_user_approval: true
  reviewer_policy: issue_author
  allowed_actions:
    approve_command: "/approve"
    correct_command: "/correct"
    reject_command: "/reject"

recommendations:
  enabled: true
  severity_threshold: low
  categories:
    - code_duplication
    - security
    - maintainability
    - performance
    - architecture

rag:
  max_files: 20
  max_file_size_kb: 120
  context_strategy: issue_relevant_files

storage:
  recommendation_ttl_days: 30


```
