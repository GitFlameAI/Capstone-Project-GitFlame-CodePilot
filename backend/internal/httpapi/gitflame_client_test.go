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
