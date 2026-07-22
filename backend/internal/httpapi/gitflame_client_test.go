package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
		case r.URL.Path == "/api/v1/repos/owner/repo/contents":
			if r.URL.Query().Get("ref") != "main" {
				t.Fatalf("unexpected contents ref: %s", r.URL.RawQuery)
			}
			body = `{"contents":[{"path":"README.md","type":"file"},{"path":"vendor/generated.go","type":"file"}]}`
		case r.URL.Path == "/api/v1/repos/owner/repo/raw/.ai.yml":
			if r.URL.Query().Get("ref") != "refs/heads/main" {
				t.Fatalf("unexpected raw ref: %s", r.URL.RawQuery)
			}
			body = "version: 1\nanalysis:\n  include:\n    - \"**/*\"\n  exclude:\n    - \"vendor/**\"\n  max_files: 5\n"
		case r.URL.Path == "/api/v1/repos/owner/repo/raw/README.md":
			body = "# Project"
		default:
			t.Fatalf("unexpected GitFlame API path: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	req, err := client.BuildAnalyzeRequest(context.Background(), GitFlameIssueWebhook{
		Repository: domain.RepositoryMetadata{ID: "owner/repo", DefaultBranch: "main"},
		Issue:      domain.IssuePayload{ID: "1", Title: "Issue", Body: "Body", Author: "artur"},
		CommitSHA:  "abc",
		Ref:        "main",
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

func TestGitFlameClientHydratesRequestedRepositoryFiles(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.Path {
		case "/api/v1/repos/owner/repo/raw/README.md":
			if r.URL.Query().Get("ref") != "refs/heads/main" {
				t.Fatalf("unexpected raw ref: %s", r.URL.RawQuery)
			}
			body = "# Project"
		default:
			t.Fatalf("unexpected GitFlame API path: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	yamlConfig, files, err := client.RepositoryFiles(context.Background(), "owner/repo", "main", "version: 1", []domain.RepositoryFile{{Path: "backend", Type: "dir"}, {Path: "README.md", Type: "file"}})
	if err != nil {
		t.Fatal(err)
	}
	if yamlConfig != "version: 1" || len(files) != 1 || files[0].Content != "# Project" {
		t.Fatalf("repository files were not hydrated: yaml=%q files=%+v", yamlConfig, files)
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
			body = `{"name":"ai-45-apply-files"}`
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
		BranchName:    "ai-45-apply-files",
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

func TestGitFlameClientAppliesGeneratedFilesToOwnerRepoEndpoint(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	var paths []string
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var body string
		switch r.URL.Path {
		case "/api/v1/repos/owner/repo/branches":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["new_branch_name"] != "ai-45-apply-files" || payload["old_ref_name"] != "main" {
				t.Fatalf("unexpected branch payload: %+v", payload)
			}
			body = `"ai-45-apply-files"`
		case "/api/v1/repos/owner/repo/commits":
			body = `{"sha":"commit-123"}`
		case "/api/v1/repos/owner/repo/pulls":
			body = `{"id":7,"html_url":"https://gitflame.test/repo/pulls/7"}`
		default:
			t.Fatalf("unexpected GitFlame apply path: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	result, err := client.ApplyGeneratedFiles(context.Background(), domain.RepositoryMetadata{ID: "owner/repo", DefaultBranch: "main"}, domain.GeneratedFilesContract{
		BranchName:    "ai-45-apply-files",
		CommitMessage: "Implement apply files",
		PRTitle:       "Apply files",
		Files: []domain.GeneratedFileOperation{{
			Action: "modify", Path: "README.md", Content: "# Updated", Explanation: "Updates docs.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{
		"/api/v1/repos/owner/repo/branches",
		"/api/v1/repos/owner/repo/commits",
		"/api/v1/repos/owner/repo/pulls",
	}
	if strings.Join(paths, ",") != strings.Join(expected, ",") {
		t.Fatalf("unexpected apply sequence: %v", paths)
	}
	if result.CommitSHA != "commit-123" || result.PullRequestID != "7" || result.PullRequestURL != "https://gitflame.test/repo/pulls/7" {
		t.Fatalf("unexpected apply result: %+v", result)
	}
}

func TestGitFlameClientContinuesWhenApplyBranchAlreadyExists(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	var paths []string
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		switch r.URL.Path {
		case "/api/v1/repos/owner/repo/branches":
			return &http.Response{StatusCode: http.StatusConflict, Body: io.NopCloser(strings.NewReader(`{"message":"reference update: reference already exists"}`)), Header: make(http.Header)}, nil
		case "/api/v1/repos/owner/repo/commits":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"sha":"commit-123"}`)), Header: make(http.Header)}, nil
		case "/api/v1/repos/owner/repo/pulls":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":7,"html_url":"https://gitflame.test/repo/pulls/7"}`)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected GitFlame apply path: %s", r.URL.Path)
		}
		return nil, nil
	})

	result, err := client.ApplyGeneratedFiles(context.Background(), domain.RepositoryMetadata{ID: "owner/repo", DefaultBranch: "main"}, domain.GeneratedFilesContract{
		BranchName:    "ai-45-apply-files",
		CommitMessage: "Implement apply files",
		PRTitle:       "Apply files",
		Files: []domain.GeneratedFileOperation{{
			Action: "modify", Path: "README.md", Content: "# Updated", Explanation: "Updates docs.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{
		"/api/v1/repos/owner/repo/branches",
		"/api/v1/repos/owner/repo/commits",
		"/api/v1/repos/owner/repo/pulls",
	}
	if strings.Join(paths, ",") != strings.Join(expected, ",") {
		t.Fatalf("unexpected apply sequence: %v", paths)
	}
	if result.CommitSHA != "commit-123" || result.PullRequestID != "7" {
		t.Fatalf("unexpected apply result: %+v", result)
	}
}

func TestGitFlameClientFallsBackWhenPullsEndpointRejectsPost(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	var paths []string
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		switch r.URL.Path {
		case "/api/v1/repos/owner/repo/branches":
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`"ai-45-apply-files"`)), Header: make(http.Header)}, nil
		case "/api/v1/repos/owner/repo/commits":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"sha":"commit-123"}`)), Header: make(http.Header)}, nil
		case "/api/v1/repos/owner/repo/pulls":
			return &http.Response{StatusCode: http.StatusMethodNotAllowed, Body: io.NopCloser(strings.NewReader(`{"message":"method not allowed"}`)), Header: make(http.Header)}, nil
		case "/api/v1/repos/owner/repo/pull-requests":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":7,"html_url":"https://gitflame.test/repo/pulls/7"}`)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected GitFlame apply path: %s", r.URL.Path)
		}
		return nil, nil
	})

	result, err := client.ApplyGeneratedFiles(context.Background(), domain.RepositoryMetadata{ID: "owner/repo", DefaultBranch: "main"}, domain.GeneratedFilesContract{
		BranchName:    "ai-45-apply-files",
		CommitMessage: "Implement apply files",
		PRTitle:       "Apply files",
		Files: []domain.GeneratedFileOperation{{
			Action: "modify", Path: "README.md", Content: "# Updated", Explanation: "Updates docs.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{
		"/api/v1/repos/owner/repo/branches",
		"/api/v1/repos/owner/repo/commits",
		"/api/v1/repos/owner/repo/pulls",
		"/api/v1/repos/owner/repo/pull-requests",
	}
	if strings.Join(paths, ",") != strings.Join(expected, ",") {
		t.Fatalf("unexpected apply sequence: %v", paths)
	}
	if result.PullRequestID != "7" || result.PullRequestURL != "https://gitflame.test/repo/pulls/7" {
		t.Fatalf("unexpected apply result: %+v", result)
	}
}

func TestGitFlameClientFallsBackToContentsWhenCommitEndpointRejectsPost(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	var paths []string
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/branches":
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`"ai-45-apply-files"`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/commits":
			return &http.Response{StatusCode: http.StatusMethodNotAllowed, Body: io.NopCloser(strings.NewReader(`{"message":"method not allowed"}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/api/v1/repositories/") && strings.HasSuffix(r.URL.Path, "/commits"):
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(`{"message":"not found"}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/owner/repo/contents/README.md":
			if r.URL.Query().Get("ref") != "ai-45-apply-files" {
				t.Fatalf("unexpected contents ref: %s", r.URL.RawQuery)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"sha":"readme-sha","content":{"sha":"commit-sha"}}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/repos/owner/repo/contents/README.md":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			decoded, err := base64.StdEncoding.DecodeString(payload["content"])
			if err != nil {
				t.Fatal(err)
			}
			if payload["branch"] != "ai-45-apply-files" ||
				payload["from_path"] != "README.md" ||
				payload["message"] != "Implement apply files" ||
				payload["sha"] != "readme-sha" ||
				string(decoded) != "# Updated" ||
				len(payload) != 5 {
				t.Fatalf("unexpected contents payload: %+v decoded=%q", payload, string(decoded))
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"commit":{"sha":"commit-123"}}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/pulls":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":7,"html_url":"https://gitflame.test/repo/pulls/7"}`)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected GitFlame apply request: %s %s", r.Method, r.URL.Path)
		}
		return nil, nil
	})

	result, err := client.ApplyGeneratedFiles(context.Background(), domain.RepositoryMetadata{ID: "owner/repo", DefaultBranch: "main"}, domain.GeneratedFilesContract{
		BranchName:    "ai-45-apply-files",
		CommitMessage: "Implement apply files",
		PRTitle:       "Apply files",
		Files: []domain.GeneratedFileOperation{{
			Action: "modify", Path: "README.md", Content: "# Updated", Explanation: "Updates docs.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CommitSHA != "commit-123" || result.PullRequestID != "7" {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if !strings.Contains(strings.Join(paths, ","), "PUT /api/v1/repos/owner/repo/contents/README.md") {
		t.Fatalf("contents fallback was not used: %v", paths)
	}
}

func TestGitFlameClientRetriesContentsUpdateWithAlternateSHA(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	getAttempts := 0
	putAttempts := 0
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/branches":
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`"ai-45-apply-files"`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commits"):
			return &http.Response{StatusCode: http.StatusMethodNotAllowed, Body: io.NopCloser(strings.NewReader(`{"message":"method not allowed"}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/owner/repo/contents/README.md":
			getAttempts++
			sha := "commit-sha"
			if getAttempts > 1 {
				sha = "readme-sha"
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"sha":"` + sha + `"}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/repos/owner/repo/contents/README.md":
			putAttempts++
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if putAttempts == 1 {
				if payload["sha"] != "commit-sha" {
					t.Fatalf("expected first sha candidate, got %+v", payload)
				}
				return &http.Response{StatusCode: http.StatusConflict, Body: io.NopCloser(strings.NewReader(`{"message":"sha conflict"}`)), Header: make(http.Header)}, nil
			}
			if payload["sha"] != "readme-sha" {
				t.Fatalf("expected alternate sha candidate, got %+v", payload)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"commit":{"sha":"commit-123"}}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/pulls":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":7}`)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected GitFlame apply request: %s %s", r.Method, r.URL.Path)
		}
		return nil, nil
	})

	result, err := client.ApplyGeneratedFiles(context.Background(), domain.RepositoryMetadata{ID: "owner/repo", DefaultBranch: "main"}, domain.GeneratedFilesContract{
		BranchName:    "ai-45-apply-files",
		CommitMessage: "Implement apply files",
		PRTitle:       "Apply files",
		Files: []domain.GeneratedFileOperation{{
			Action: "modify", Path: "README.md", Content: "# Updated", Explanation: "Updates docs.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if getAttempts != 2 || putAttempts != 2 || result.CommitSHA != "commit-123" {
		t.Fatalf("expected refreshed sha, gets=%d puts=%d result=%+v", getAttempts, putAttempts, result)
	}
}

func TestGitFlameClientUsesSwaggerUpdateContractForExistingFile(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	putAttempts := 0
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/branches":
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`"ai-45-apply-files"`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commits"):
			return &http.Response{StatusCode: http.StatusMethodNotAllowed, Body: io.NopCloser(strings.NewReader(`{"message":"method not allowed"}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/owner/repo/contents/README.md":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"sha":"readme-sha"}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/repos/owner/repo/contents/README.md":
			putAttempts++
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			decoded, err := base64.StdEncoding.DecodeString(payload["content"])
			if err != nil || string(decoded) != "# Updated" {
				t.Fatalf("expected base64 content payload, got %+v", payload)
			}
			if payload["branch"] != "ai-45-apply-files" ||
				payload["from_path"] != "README.md" ||
				payload["message"] != "Implement apply files" ||
				payload["sha"] != "readme-sha" ||
				len(payload) != 5 {
				t.Fatalf("unexpected Swagger update payload: %+v", payload)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"commit":{"sha":"commit-123"}}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/pulls":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":7}`)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected GitFlame apply request: %s %s", r.Method, r.URL.Path)
		}
		return nil, nil
	})

	result, err := client.ApplyGeneratedFiles(context.Background(), domain.RepositoryMetadata{ID: "owner/repo", DefaultBranch: "main"}, domain.GeneratedFilesContract{
		BranchName:    "ai-45-apply-files",
		CommitMessage: "Implement apply files",
		PRTitle:       "Apply files",
		Files: []domain.GeneratedFileOperation{{
			Action: "modify", Path: "README.md", Content: "# Updated", Explanation: "Updates docs.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if putAttempts != 1 || result.CommitSHA != "commit-123" {
		t.Fatalf("expected one Swagger update request, attempts=%d result=%+v", putAttempts, result)
	}
}

func TestGitFlameClientRefreshesFileSHABeforeFinalConflict(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	getAttempts := 0
	putAttempts := 0
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/branches":
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`"ai-45-apply-files"`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commits"):
			return &http.Response{StatusCode: http.StatusMethodNotAllowed, Body: io.NopCloser(strings.NewReader(`{"message":"method not allowed"}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/owner/repo/contents/README.md":
			getAttempts++
			sha := "old-sha"
			if getAttempts > 1 {
				sha = "fresh-sha"
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"sha":"` + sha + `"}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/repos/owner/repo/contents/README.md":
			putAttempts++
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["sha"] == "fresh-sha" {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"commit":{"sha":"commit-123"}}`)), Header: make(http.Header)}, nil
			}
			return &http.Response{StatusCode: http.StatusConflict, Body: io.NopCloser(strings.NewReader(`{"message":"sha conflict"}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/pulls":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":7}`)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected GitFlame apply request: %s %s", r.Method, r.URL.Path)
		}
		return nil, nil
	})

	result, err := client.ApplyGeneratedFiles(context.Background(), domain.RepositoryMetadata{ID: "owner/repo", DefaultBranch: "main"}, domain.GeneratedFilesContract{
		BranchName:    "ai-45-apply-files",
		CommitMessage: "Implement apply files",
		PRTitle:       "Apply files",
		Files: []domain.GeneratedFileOperation{{
			Action: "modify", Path: "README.md", Content: "# Updated", Explanation: "Updates docs.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if getAttempts != 2 || putAttempts != 2 || result.CommitSHA != "commit-123" {
		t.Fatalf("expected refreshed sha retry, gets=%d puts=%d result=%+v", getAttempts, putAttempts, result)
	}
}

func TestGitFlameClientUsesSwaggerCreateContract(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	createAttempts := 0
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/branches":
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`"ai-45-apply-files"`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commits"):
			return &http.Response{StatusCode: http.StatusMethodNotAllowed, Body: io.NopCloser(strings.NewReader(`{"message":"method not allowed"}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/contents/shared/discount.ts":
			createAttempts++
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			decoded, err := base64.StdEncoding.DecodeString(payload["content"])
			if err != nil || string(decoded) != "export const discount = 10\n" {
				t.Fatalf("expected base64 create content: %+v", payload)
			}
			if payload["branch"] != "ai-45-apply-files" ||
				payload["message"] != "Implement apply files" || len(payload) != 3 {
				t.Fatalf("unexpected Swagger create payload: %+v", payload)
			}
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`{"commit":{"sha":"commit-create"}}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/pulls":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":7}`)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected GitFlame apply request: %s %s", r.Method, r.URL.Path)
		}
		return nil, nil
	})

	result, err := client.ApplyGeneratedFiles(context.Background(), domain.RepositoryMetadata{ID: "owner/repo", DefaultBranch: "main"}, domain.GeneratedFilesContract{
		BranchName:    "ai-45-apply-files",
		CommitMessage: "Implement apply files",
		PRTitle:       "Apply files",
		Files: []domain.GeneratedFileOperation{{
			Action: "create", Path: "shared/discount.ts", Content: "export const discount = 10\n", Explanation: "Creates shared discount helper.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if createAttempts != 1 || result.CommitSHA != "commit-create" {
		t.Fatalf("expected one Swagger create request, attempts=%d result=%+v", createAttempts, result)
	}
}

func TestGitFlameClientUsesSwaggerDeleteContract(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	deleteAttempts := 0
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/branches":
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`"ai-45-apply-files"`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commits"):
			return &http.Response{StatusCode: http.StatusMethodNotAllowed, Body: io.NopCloser(strings.NewReader(`{"message":"method not allowed"}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/owner/repo/contents/README.md":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"sha":"readme-sha"}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/repos/owner/repo/contents/README.md":
			deleteAttempts++
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["branch"] != "ai-45-apply-files" ||
				payload["message"] != "Implement apply files" ||
				payload["sha"] != "readme-sha" || len(payload) != 3 {
				t.Fatalf("unexpected Swagger delete payload: %+v", payload)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"commit":{"sha":"commit-delete"}}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/pulls":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":7}`)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected GitFlame apply request: %s %s", r.Method, r.URL.Path)
		}
		return nil, nil
	})

	result, err := client.ApplyGeneratedFiles(context.Background(), domain.RepositoryMetadata{ID: "owner/repo", DefaultBranch: "main"}, domain.GeneratedFilesContract{
		BranchName:    "ai-45-apply-files",
		CommitMessage: "Implement apply files",
		PRTitle:       "Apply files",
		Files: []domain.GeneratedFileOperation{{
			Action: "delete", Path: "README.md", Explanation: "Removes obsolete docs.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if deleteAttempts != 1 || result.CommitSHA != "commit-delete" {
		t.Fatalf("expected one Swagger delete request, attempts=%d result=%+v", deleteAttempts, result)
	}
}

func TestGitFlameClientAnnotatesContentsApplyErrorsWithPath(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/owner/repo/branches":
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`"ai-45-apply-files"`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commits"):
			return &http.Response{StatusCode: http.StatusMethodNotAllowed, Body: io.NopCloser(strings.NewReader(`{"message":"method not allowed"}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/owner/repo/contents/README.md":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"sha":"readme-sha"}`)), Header: make(http.Header)}, nil
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/repos/owner/repo/contents/README.md":
			return &http.Response{StatusCode: http.StatusConflict, Body: io.NopCloser(strings.NewReader(`{"message":["reference update conflict"]}`)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected GitFlame apply request: %s %s", r.Method, r.URL.Path)
		}
		return nil, nil
	})

	_, err := client.ApplyGeneratedFiles(context.Background(), domain.RepositoryMetadata{ID: "owner/repo", DefaultBranch: "main"}, domain.GeneratedFilesContract{
		BranchName:    "ai-45-apply-files",
		CommitMessage: "Implement apply files",
		PRTitle:       "Apply files",
		Files: []domain.GeneratedFileOperation{{
			Action: "modify", Path: "README.md", Content: "# Updated", Explanation: "Updates docs.",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "README.md: reference update conflict") {
		t.Fatalf("expected path-aware conflict error, got %v", err)
	}
}

func TestGitFlameClientReadsWrappedRepositoryData(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body string
		switch {
		case r.URL.Path == "/api/v1/repos/owner/repo/contents":
			if r.URL.Query().Get("ref") != "main" {
				t.Fatalf("unexpected root contents request: %s", r.URL.String())
			}
			body = `{"contents":[{"path":"backend","type":"dir"},{"path":"README.md","type":"file"}]}`
		case r.URL.Path == "/api/v1/repos/owner/repo/contents/backend":
			if r.URL.Query().Get("ref") != "main" {
				t.Fatalf("unexpected nested contents request: %s", r.URL.String())
			}
			body = `{"contents":[{"path":"backend/main.go","type":"file"}]}`
		case strings.Contains(r.URL.Path, "/issues"):
			if r.URL.Path != "/api/v1/repos/owner/repo/issues" || r.URL.Query().Get("type") != "issues" {
				t.Fatalf("unexpected Gitea issues request: %s", r.URL.String())
			}
			body = `{"issues":[{"iid":42,"title":"Fix API","description":"Return repository data","author":{"username":"artur"}}]}`
		default:
			t.Fatalf("unexpected GitFlame API path: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	tree, err := client.RepositoryTree(context.Background(), "owner/repo", "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 3 || tree[0].Type != "dir" || tree[1].Path != "README.md" || tree[2].Path != "backend/main.go" {
		t.Fatalf("unexpected normalized tree: %+v", tree)
	}
	issues, err := client.RepositoryIssues(context.Background(), "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].ID != "42" || issues[0].Body != "Return repository data" || issues[0].Author != "artur" {
		t.Fatalf("unexpected normalized issues: %+v", issues)
	}
}

func TestGitFlameClientRejectsInaccessibleRepository(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	requests := 0
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(`{"message":"forbidden"}`)), Header: make(http.Header)}, nil
	})

	_, err := client.ResolveRepository(context.Background(), "https://gitflame.test/owner/private-repo")
	var integration *IntegrationError
	if !errors.As(err, &integration) || integration.Status != http.StatusForbidden {
		t.Fatalf("expected forbidden repository error, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected resolver to stop after forbidden response, got %d requests", requests)
	}
}

func TestGitFlameClientResolvesGiteaRepositorySlug(t *testing.T) {
	client := NewGitFlameClient("http://gitflame.test", "token", time.Second)
	client.httpClient.Transport = gitFlameRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/repos/owner/repo" {
			t.Fatalf("unexpected repository request: %s", r.URL.Path)
		}
		body := `{"id":17,"name":"repo","full_name":"owner/repo","default_branch":"develop","html_url":"https://gitflame.test/owner/repo"}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	resolved, err := client.ResolveRepository(context.Background(), "https://gitflame.test/owner/repo/code")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Metadata.ID != "owner/repo" || resolved.Metadata.DefaultBranch != "develop" {
		t.Fatalf("unexpected resolved repository: %+v", resolved)
	}
}
