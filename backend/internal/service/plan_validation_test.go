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

func TestNormalizeGeneratedFilesMergesDuplicatePaths(t *testing.T) {
	files := NormalizeGeneratedFiles([]domain.GeneratedFileOperation{
		{
			Action:      "modify",
			Path:        "./app/main.py",
			Content:     "old content",
			Explanation: "First pass.",
		},
		{
			Action:      "modify",
			Path:        "app/main.py",
			Content:     "new content",
			Explanation: "Final pass.",
		},
	})
	if len(files) != 1 {
		t.Fatalf("expected duplicate paths to be merged, got %+v", files)
	}
	if files[0].Path != "app/main.py" || files[0].Content != "new content" || !strings.Contains(files[0].Explanation, "First pass.") || !strings.Contains(files[0].Explanation, "Final pass.") {
		t.Fatalf("unexpected merged operation: %+v", files[0])
	}
	if err := ValidateGeneratedFiles(files, []domain.RepositoryFile{{Path: "app/main.py", Content: "package"}}); err != nil {
		t.Fatalf("merged operation should validate: %v", err)
	}
}

func TestDropNoopGeneratedFilesSkipsUnchangedModifies(t *testing.T) {
	files := DropNoopGeneratedFiles([]domain.GeneratedFileOperation{
		{
			Action:      "modify",
			Path:        "./app/main.py",
			Content:     "package main\n",
			Explanation: "No effective change.",
		},
		{
			Action:      "modify",
			Path:        "app/routes.py",
			Content:     "def route():\n    return 'ok'\n",
			Explanation: "Adds a real change.",
		},
	}, []domain.RepositoryFile{
		{Path: "app/main.py", Content: "package main\r\n"},
		{Path: "app/routes.py", Content: "def route():\n    pass\n"},
	})
	if len(files) != 1 || files[0].Path != "app/routes.py" {
		t.Fatalf("expected only changed file to remain, got %+v", files)
	}
}

func TestDropNoopGeneratedFilesDropsAllNoops(t *testing.T) {
	files := DropNoopGeneratedFiles([]domain.GeneratedFileOperation{{
		Action:      "modify",
		Path:        "app/main.py",
		Content:     "package app\n",
		Explanation: "Safe fallback with complete original content.",
	}}, []domain.RepositoryFile{{Path: "app/main.py", Content: "package app\n"}})
	if len(files) != 0 {
		t.Fatalf("expected all-noop files to be dropped, got %+v", files)
	}
}

func TestDropNoopGeneratedFilesForApplyDropsAllNoops(t *testing.T) {
	files := DropNoopGeneratedFilesForApply([]domain.GeneratedFileOperation{{
		Action:      "modify",
		Path:        "app/main.py",
		Content:     "package app\n",
		Explanation: "Safe fallback with complete original content.",
	}}, []domain.RepositoryFile{{Path: "app/main.py", Content: "package app\n"}})
	if len(files) != 0 {
		t.Fatalf("expected apply filter to drop all no-op files, got %+v", files)
	}
}

func TestDropUnsafePartialModifyFilesSkipsCompressedModifyContent(t *testing.T) {
	originalAuth := strings.Join([]string{
		"from fastapi import Depends, HTTPException",
		"from jose import jwt",
		"",
		"def current_user(token: str):",
		"    payload = jwt.decode(token, 'secret')",
		"    user_id = payload.get('sub')",
		"    if not user_id:",
		"        raise HTTPException(status_code=401)",
		"    return {'id': user_id}",
		"",
		"def require_admin(user = Depends(current_user)):",
		"    if user.get('role') != 'admin':",
		"        raise HTTPException(status_code=403)",
		"    return user",
	}, "\n")
	files := DropUnsafePartialModifyFiles([]domain.GeneratedFileOperation{
		{
			Action:      "modify",
			Path:        "backend-python/app/auth.py",
			Content:     "def current_user(...): validate token and return user",
			Explanation: "Compressed partial change.",
		},
		{
			Action:      "modify",
			Path:        "backend-python/app/main.py",
			Content:     "from fastapi import FastAPI\n\napp = FastAPI()\n\n@app.get('/health')\ndef health():\n    return {'ok': True}\n",
			Explanation: "Keeps a complete file update.",
		},
	}, []domain.RepositoryFile{
		{Path: "backend-python/app/auth.py", Content: originalAuth},
		{Path: "backend-python/app/main.py", Content: "from fastapi import FastAPI\n\napp = FastAPI()\n"},
	})
	if len(files) != 1 || files[0].Path != "backend-python/app/main.py" {
		t.Fatalf("expected only complete modify content to remain, got %+v", files)
	}
}

func TestDropUnsafePartialModifyFilesKeepsSmallFileChanges(t *testing.T) {
	files := DropUnsafePartialModifyFiles([]domain.GeneratedFileOperation{{
		Action:      "modify",
		Path:        "README.md",
		Content:     "# App\n\nUpdated.",
		Explanation: "Updates a small readme.",
	}}, []domain.RepositoryFile{{Path: "README.md", Content: "# App\n"}})
	if len(files) != 1 {
		t.Fatalf("expected small valid change to remain, got %+v", files)
	}
}

func TestNormalizeGitFlameBranchNameRemovesSlashes(t *testing.T) {
	branch := normalizeGitFlameBranchName("ai/ISSUE-170-improve-project")
	if branch != "ai-ISSUE-170-improve-project" {
		t.Fatalf("unexpected branch name: %s", branch)
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
