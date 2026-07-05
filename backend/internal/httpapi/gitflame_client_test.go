package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"gitflame-codepilot/backend/internal/domain"
)

type gitFlameRoundTripFunc func(*http.Request) (*http.Response, error)

func (f gitFlameRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGitFlameClientBuildsAnalyzeRequestFromAPI(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("missing GitFlame API token")
		}
		var body string
		switch {
		case strings.Contains(r.URL.Path, "/tree"):
			body = `[{"path":"README.md","type":"file"},{"path":"vendor/generated.go","type":"file"}]`
		case strings.Contains(r.URL.Path, "/files/.ai.yml"):
			body = fmt.Sprintf(`{"path":".ai.yml","content":%q}`, "version: 1\nanalysis:\n  include:\n    - \"**/*\"\n  exclude:\n    - \"vendor/**\"\n  max_files: 5\n")
		case strings.Contains(r.URL.Path, "/files/README.md"):
			body = `{"path":"README.md","content":"# Project"}`
		default:
			t.Fatalf("unexpected GitFlame API path: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	req, err := client.BuildAnalyzeRequest(context.Background(), GitFlameIssueWebhook{
		Repository: domain.RepositoryMetadata{ID: "repo", DefaultBranch: "main"},
		Issue:      domain.IssuePayload{ID: "1", Title: "Issue", Body: "Body", Author: "artur"},
		CommitSHA:  "abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.YAMLConfig == "" || req.Repository.CommitSHA != "abc" {
		t.Fatalf("missing config or commit sha: %+v", req)
	}
	if len(req.RepositoryFiles) != 1 || req.RepositoryFiles[0].Path != "README.md" || req.RepositoryFiles[0].Content != "# Project" {
		t.Fatalf("repository files were not filtered/fetched correctly: %+v", req.RepositoryFiles)
	}
}

func TestGitFlameClientAppliesGeneratedFiles(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	var paths []string
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var body string
		switch {
		case strings.Contains(r.URL.Path, "/branches"):
			body = `{"name":"ai/45-apply-files"}`
		case strings.Contains(r.URL.Path, "/commits"):
			body = `{"sha":"commit-123"}`
		case strings.Contains(r.URL.Path, "/pull-requests"):
			body = `{"id":7,"html_url":"https://gitflame.test/repo/pulls/7"}`
		default:
			t.Fatalf("unexpected GitFlame apply path: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	result, err := client.ApplyGeneratedFiles(context.Background(), domain.RepositoryMetadata{ID: "repo", DefaultBranch: "main"}, domain.GeneratedFilesContract{
		BranchName:    "ai/45-apply-files",
		CommitMessage: "Implement apply files",
		PRTitle:       "Apply files",
		Files: []domain.GeneratedFileOperation{{
			Action: "modify", Path: "README.md", Content: "# Updated", Explanation: "Updates docs.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 || !strings.Contains(paths[0], "/branches") || !strings.Contains(paths[1], "/commits") || !strings.Contains(paths[2], "/pull-requests") {
		t.Fatalf("unexpected apply sequence: %v", paths)
	}
	if result.CommitSHA != "commit-123" || result.PullRequestID != "7" || result.PullRequestURL != "https://gitflame.test/repo/pulls/7" {
		t.Fatalf("unexpected apply result: %+v", result)
	}
}
