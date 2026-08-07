package httpapi

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/repository"
)

// blockingIndexReader is a GitFlameRepositoryReader whose tree read blocks until
// the test releases it, which is how a slow repository is simulated.
type blockingIndexReader struct {
	release chan struct{}
	tree    []GitFlameTreeEntry
}

func (r *blockingIndexReader) RepositoryTree(ctx context.Context, _, _ string) ([]GitFlameTreeEntry, error) {
	select {
	case <-r.release:
		return r.tree, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *blockingIndexReader) RepositoryFiles(_ context.Context, _, _, yamlConfig string, requested []domain.RepositoryFile) (string, []domain.RepositoryFile, error) {
	files := make([]domain.RepositoryFile, 0, len(requested))
	for _, file := range requested {
		files = append(files, domain.RepositoryFile{Path: file.Path, Content: "package main\n"})
	}
	return yamlConfig, files, nil
}

func (r *blockingIndexReader) RepositoryIssues(context.Context, string) ([]domain.IssuePayload, error) {
	return nil, nil
}

type stubIndexer struct {
	indexed chan RAGIndexRequest
	failure error
}

func (s *stubIndexer) Ready(context.Context) error { return nil }

func (s *stubIndexer) Status(context.Context, string, string) (RAGIndexResult, error) {
	return RAGIndexResult{Status: "missing"}, nil
}

func (s *stubIndexer) Index(_ context.Context, request RAGIndexRequest) (RAGIndexResult, error) {
	if s.failure != nil {
		return RAGIndexResult{}, s.failure
	}
	select {
	case s.indexed <- request:
	default:
	}
	return RAGIndexResult{
		RepositoryID: request.RepositoryID, CommitSHA: request.CommitSHA,
		Status: "indexed", FileCount: len(request.Files), ChunkCount: len(request.Files), EmbeddingCount: len(request.Files),
	}, nil
}

func testIndexServer(indexer RepositoryIndexer) *Server {
	return &Server{
		store:                  repository.NewMemoryStore(),
		indexer:                indexer,
		indexJobs:              newIndexJobRegistry(),
		indexBackgroundTimeout: 10 * time.Second,
		indexWaitTimeout:       5 * time.Second,
	}
}

func testConnection() *domain.GitFlameConnection {
	return &domain.GitFlameConnection{
		ID:            "connection-1",
		DefaultBranch: "main",
		Repository:    domain.RepositoryMetadata{ID: "acme/project", Name: "project", DefaultBranch: "main"},
	}
}

func TestStartRepositoryIndexInBackgroundReturnsBeforeIndexingFinishes(t *testing.T) {
	t.Parallel()

	reader := &blockingIndexReader{release: make(chan struct{}), tree: []GitFlameTreeEntry{{Path: "main.go", Type: "file"}}}
	indexer := &stubIndexer{indexed: make(chan RAGIndexRequest, 1)}
	server := testIndexServer(indexer)
	connection := testConnection()

	started := time.Now()
	status := server.startRepositoryIndexInBackground(context.Background(), reader, connection, "main", false)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("start blocked for %s, expected an immediate return", elapsed)
	}
	if status.Status != indexStatusRunning {
		t.Fatalf("status = %q, want %q", status.Status, indexStatusRunning)
	}

	// While the job runs, a second caller must observe the same job.
	if again := server.startRepositoryIndexInBackground(context.Background(), reader, connection, "main", false); again.Status != indexStatusRunning {
		t.Fatalf("second start status = %q, want %q", again.Status, indexStatusRunning)
	}

	close(reader.release)
	select {
	case request := <-indexer.indexed:
		if request.RepositoryID != connection.Repository.ID {
			t.Fatalf("indexed repository = %q, want %q", request.RepositoryID, connection.Repository.ID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("background indexing did not reach the indexer")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.awaitRepositoryIndexJob(ctx, connection.Repository.ID); err != nil {
		t.Fatalf("awaitRepositoryIndexJob returned error: %v", err)
	}
	final, known := server.indexJobs.status(connection.Repository.ID)
	if !known || final.Status != indexStatusCompleted {
		t.Fatalf("final status = %#v, want completed", final)
	}
	if final.FileCount != 1 {
		t.Fatalf("final file count = %d, want 1", final.FileCount)
	}
}

func TestStartRepositoryIndexInBackgroundRecordsFailure(t *testing.T) {
	t.Parallel()

	reader := &blockingIndexReader{release: make(chan struct{}), tree: []GitFlameTreeEntry{{Path: "main.go", Type: "file"}}}
	close(reader.release)
	indexer := &stubIndexer{failure: &IntegrationError{Status: http.StatusServiceUnavailable, Code: "rag_unreachable", Detail: "CodeRAG service is unreachable"}}
	server := testIndexServer(indexer)
	connection := testConnection()

	server.startRepositoryIndexInBackground(context.Background(), reader, connection, "main", false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.awaitRepositoryIndexJob(ctx, connection.Repository.ID); err != nil {
		t.Fatalf("awaitRepositoryIndexJob must not fail on a failed job, got: %v", err)
	}
	status, known := server.indexJobs.status(connection.Repository.ID)
	if !known || status.Status != indexStatusFailed {
		t.Fatalf("status = %#v, want failed", status)
	}
	if status.ErrorCode != "rag_unreachable" {
		t.Fatalf("error code = %q, want %q", status.ErrorCode, "rag_unreachable")
	}
}

func TestIndexJobRegistryReusesRecentSuccessUnlessForced(t *testing.T) {
	t.Parallel()

	registry := newIndexJobRegistry()
	if _, owned := registry.begin("acme/project", "connection-1", "main", false); !owned {
		t.Fatal("first begin must own the job")
	}
	registry.finish("acme/project", RAGIndexResult{Status: "indexed", CommitSHA: "abc123"}, nil)

	if _, owned := registry.begin("acme/project", "connection-1", "main", false); owned {
		t.Fatal("a job that finished moments ago must be reused, not restarted")
	}
	if _, owned := registry.begin("acme/project", "connection-1", "main", true); !owned {
		t.Fatal("a forced begin must start a new job")
	}
}

func TestIndexJobRegistryWaitTimesOutWithoutCancellingTheJob(t *testing.T) {
	t.Parallel()

	registry := newIndexJobRegistry()
	registry.begin("acme/project", "connection-1", "main", false)

	_, known, err := registry.wait(context.Background(), "acme/project", 20*time.Millisecond)
	if !known {
		t.Fatal("wait must report the job as known")
	}
	if !errors.Is(err, errIndexWaitTimeout) {
		t.Fatalf("wait error = %v, want errIndexWaitTimeout", err)
	}
	if status, _ := registry.status("acme/project"); status.Status != indexStatusRunning {
		t.Fatalf("status after a timed-out wait = %q, want %q", status.Status, indexStatusRunning)
	}

	registry.finish("acme/project", RAGIndexResult{Status: "indexed"}, nil)
	status, known, err := registry.wait(context.Background(), "acme/project", time.Second)
	if err != nil || !known || status.Status != indexStatusCompleted {
		t.Fatalf("wait after completion = (%#v, %v, %v)", status, known, err)
	}
}

func TestIndexJobRegistryWaitIsNoopForUnknownRepository(t *testing.T) {
	t.Parallel()

	registry := newIndexJobRegistry()
	status, known, err := registry.wait(context.Background(), "acme/project", time.Second)
	if known || err != nil || status.Status != "" {
		t.Fatalf("wait for an unknown repository = (%#v, %v, %v)", status, known, err)
	}
}
