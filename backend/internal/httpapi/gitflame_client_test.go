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
