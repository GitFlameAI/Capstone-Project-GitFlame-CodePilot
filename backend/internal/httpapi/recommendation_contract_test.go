package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"gitflame-codepilot/backend/internal/domain"
)

func TestRecommendationClientSendsRepositoryRevision(t *testing.T) {
	var payload struct {
		Repository  domain.RepositoryMetadata `json:"repository"`
		ConfigYAML  string                    `json:"config_yaml"`
		RepoContext []domain.RepositoryFile   `json:"repo_context"`
	}
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"summary":"complete","recommendations":[]}`)),
			Request:    r,
		}, nil
	})

	client := NewRecommendationClient("http://recommendation-service", time.Second)
	client.httpClient.Transport = transport
	repository := domain.RepositoryMetadata{ID: "owner/repository", CommitSHA: "abc123"}
	_, _, err := client.AnalyzeRecommendations(
		context.Background(),
		repository,
		"version: 1",
		[]domain.RepositoryFile{{Path: "src/app.py", Content: "package app\n"}},
	)
	if err != nil {
		t.Fatalf("AnalyzeRecommendations returned error: %v", err)
	}
	if payload.Repository.ID != repository.ID || payload.Repository.CommitSHA != repository.CommitSHA {
		t.Fatalf("unexpected repository payload: %+v", payload.Repository)
	}
	if len(payload.RepoContext) != 1 || payload.RepoContext[0].Path != "src/app.py" {
		t.Fatalf("unexpected repository files: %+v", payload.RepoContext)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type completeAnalysisReader struct {
	indexRequested []domain.RepositoryFile
}

func (r *completeAnalysisReader) RepositoryTree(context.Context, string, string) ([]GitFlameTreeEntry, error) {
	return nil, nil
}

func (r *completeAnalysisReader) RepositoryFiles(context.Context, string, string, string, []domain.RepositoryFile) (string, []domain.RepositoryFile, error) {
	return "", nil, nil
}

func (r *completeAnalysisReader) RepositoryFilesForIndex(_ context.Context, _, _, yaml string, requested []domain.RepositoryFile) (string, []domain.RepositoryFile, error) {
	r.indexRequested = requested
	return yaml, []domain.RepositoryFile{
		{Path: "src/a.go", Content: "package a\n"},
		{Path: "src/b.go", Content: "package b\n"},
	}, nil
}

func (r *completeAnalysisReader) RepositoryIssues(context.Context, string) ([]domain.IssuePayload, error) {
	return nil, nil
}

func TestRepositoryFilesForAnalysisRequestsCompleteSnapshot(t *testing.T) {
	reader := &completeAnalysisReader{}
	_, files, err := repositoryFilesForAnalysis(
		context.Background(),
		reader,
		"owner/repository",
		"main",
		"version: 1",
		[]domain.RepositoryFile{{Path: "src/a.go"}},
	)
	if err != nil {
		t.Fatalf("repositoryFilesForAnalysis returned error: %v", err)
	}
	if reader.indexRequested != nil {
		t.Fatalf("complete reader must receive nil requested files, got: %+v", reader.indexRequested)
	}
	if len(files) != 2 {
		t.Fatalf("expected complete snapshot, got %d files", len(files))
	}
}
