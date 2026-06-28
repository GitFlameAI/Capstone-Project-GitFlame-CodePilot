package service

import (
	"strings"
	"testing"

	"gitflame-codepilot/backend/internal/domain"
)

func TestValidatePlan(t *testing.T) {
	plan := validPlan("internal/httpapi/server.go")
	if err := ValidatePlan(plan, []domain.RepositoryFile{{Path: "internal/httpapi/server.go"}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePlan(strings.Replace(plan, "## Goal", "## Objective", 1), []domain.RepositoryFile{{Path: "internal/httpapi/server.go"}}); err == nil {
		t.Fatal("expected missing section error")
	}
	if err := ValidatePlan(validPlan("invented/file.go"), []domain.RepositoryFile{{Path: "internal/httpapi/server.go"}}); err == nil {
		t.Fatal("expected hallucinated file error")
	}
}

func TestValidateGeneratedFiles(t *testing.T) {
	repositoryFiles := []domain.RepositoryFile{{Path: "internal/httpapi/server.go", Content: "package httpapi"}}
	valid := []domain.GeneratedFileOperation{{
		Action:      "modify",
		Path:        "internal/httpapi/server.go",
		Content:     "package httpapi\n",
		Explanation: "Updates the endpoint.",
	}}
	if err := ValidateGeneratedFiles(valid, repositoryFiles); err != nil {
		t.Fatal(err)
	}
	invalidCases := map[string][]domain.GeneratedFileOperation{
		"unsafe path": {{
			Action:      "modify",
			Path:        "../server.go",
			Content:     "package main",
			Explanation: "Bad path.",
		}},
		"missing content": {{
			Action:      "modify",
			Path:        "internal/httpapi/server.go",
			Explanation: "No content.",
		}},
		"unknown modify target": {{
			Action:      "modify",
			Path:        "internal/httpapi/missing.go",
			Content:     "package httpapi",
			Explanation: "Unknown target.",
		}},
		"delete with diff": {{
			Action:      "delete",
			Path:        "internal/httpapi/server.go",
			Diff:        "@@",
			Explanation: "Invalid delete.",
		}},
	}
	for name, files := range invalidCases {
		if err := ValidateGeneratedFiles(files, repositoryFiles); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}

func validPlan(path string) string {
	return `# Implementation Plan

## Issue Summary
Summary.

## Goal
Goal.

## Relevant Files
- ` + "`" + path + "`" + `: relevant.

## Proposed Changes
- Change behavior.

## Implementation Steps
1. Implement.

## Expected Files to Change
- ` + "`" + path + "`" + `: modify.

## Tests and Verification
- Verify.

## Risks and Open Questions
- TBD.
`
}
