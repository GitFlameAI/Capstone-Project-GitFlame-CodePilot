package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitflame-codepilot/backend/internal/domain"
)

func TestRepositoryFilesForIndexFetchesConcurrentlyAndPreservesOrder(t *testing.T) {
	t.Parallel()

	var active atomic.Int32
	var maximum atomic.Int32
	var requests atomic.Int32
	var releaseOnce sync.Once
	release := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		if requests.Add(1) >= 2 {
			releaseOnce.Do(func() { close(release) })
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = fmt.Fprint(w, strings.TrimPrefix(r.URL.Path, "/api/v1/repos/acme/project/raw/"))
	}))
	defer server.Close()

	client := NewGitFlameClient(server.URL, "token", time.Second)
	requested := []domain.RepositoryFile{
		{Path: "first.go", Type: "file"},
		{Path: "second.go", Type: "file"},
		{Path: "third.go", Type: "file"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, files, err := client.RepositoryFilesForIndex(ctx, "acme/project", "main", defaultIndexConfiguration, requested)
	if err != nil {
		t.Fatalf("RepositoryFilesForIndex returned error: %v", err)
	}
	if maximum.Load() < 2 {
		t.Fatalf("expected concurrent file requests, maximum in flight was %d", maximum.Load())
	}
	for index, expected := range []string{"first.go", "second.go", "third.go"} {
		if files[index].Path != expected || files[index].Content != expected {
			t.Fatalf("file %d = %#v, want path and content %q", index, files[index], expected)
		}
	}
}

func TestRepositoryFilesForIndexSkipsBinaryPathsAndContents(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/bad.dat"):
			_, _ = w.Write([]byte{'t', 'e', 'x', 't', 0, 'd', 'a', 't', 'a'})
		default:
			_, _ = fmt.Fprint(w, "package main")
		}
	}))
	defer server.Close()

	client := NewGitFlameClient(server.URL, "token", time.Second)
	requested := []domain.RepositoryFile{
		{Path: "main.go", Type: "file"},
		{Path: "proof.png", Type: "file"},
		{Path: "bad.dat", Type: "file"},
	}

	_, files, err := client.RepositoryFilesForIndex(
		context.Background(), "acme/project", "main", defaultIndexConfiguration, requested,
	)
	if err != nil {
		t.Fatalf("RepositoryFilesForIndex returned error: %v", err)
	}
	if len(files) != 1 || files[0].Path != "main.go" {
		t.Fatalf("files = %#v, want only main.go", files)
	}
}

func TestDecodeGitFlameCollectionSupportsPaginatedEnvelopes(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"content":      `{"content":[{"id":1}]}`,
		"nestedResult": `{"data":{"results":[{"id":2}]}}`,
		"nestedValues": `{"data":{"content":{"values":[{"id":3}]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var decoded []struct {
				ID int `json:"id"`
			}
			if err := decodeGitFlameCollection([]byte(body), []string{"issues", "items", "data"}, &decoded); err != nil {
				t.Fatalf("decodeGitFlameCollection returned error: %v", err)
			}
			if len(decoded) != 1 || decoded[0].ID == 0 {
				t.Fatalf("decoded = %#v, want one issue", decoded)
			}
		})
	}
}

func TestRepositoryIssuesRequestsJSONAndReadsContentEnvelope(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			t.Fatalf("Accept = %q, want application/json", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":{"content":[{"number":7,"title":"Fix API","body":"Return JSON","user":{"login":"kite"}}]}}`)
	}))
	defer server.Close()

	client := NewGitFlameClient(server.URL, "token", time.Second)
	issues, err := client.RepositoryIssues(context.Background(), "owner/repository")
	if err != nil {
		t.Fatalf("RepositoryIssues returned error: %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "7" || issues[0].Author != "kite" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestRepositoryIssuesFallsBackAfterUnexpectedSuccessfulResponse(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v1/repos/") {
			_, _ = fmt.Fprint(w, `{"unexpected":"shape"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"items":[{"id":"ISSUE-9","title":"Fallback works"}]}`)
	}))
	defer server.Close()

	client := NewGitFlameClient(server.URL, "token", time.Second)
	issues, err := client.RepositoryIssues(context.Background(), "owner/repository")
	if err != nil {
		t.Fatalf("RepositoryIssues returned error: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	if len(issues) != 1 || issues[0].ID != "ISSUE-9" {
		t.Fatalf("issues = %#v", issues)
	}
}
