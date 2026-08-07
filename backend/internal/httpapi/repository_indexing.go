package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/observability"
	"gitflame-codepilot/backend/internal/repository"
	"gitflame-codepilot/backend/internal/service"
)

type gitFlameRevisionReader interface {
	RepositoryRevision(context.Context, string, string) (string, error)
}

type gitFlameIndexReader interface {
	RepositoryFilesForIndex(context.Context, string, string, string, []domain.RepositoryFile) (string, []domain.RepositoryFile, error)
}

type repositoryIndexSync struct {
	Result     RAGIndexResult
	YAMLConfig string
	Files      []domain.RepositoryFile
}

// ensureRequestRepositoryIndex guarantees that the repository is indexed before
// AI work is queued. Indexing usually already runs in the background (started
// when the repository was connected or browsed), so the common path is a wait
// on that job; only when no job exists, or the background job failed, does this
// index the repository inline.
func (s *Server) ensureRequestRepositoryIndex(
	r *http.Request,
	repositoryMetadata *domain.RepositoryMetadata,
) error {
	if s.indexer == nil || repositoryMetadata == nil || strings.TrimSpace(repositoryMetadata.ID) == "" {
		return nil
	}
	reader, connection, err := s.gitFlameReaderForRepository(r, repositoryMetadata.ID)
	if err != nil {
		return err
	}
	if err := s.awaitRepositoryIndexJob(r.Context(), repositoryMetadata.ID); err != nil {
		return err
	}
	result, err := s.synchronizeRepositoryIndex(
		r.Context(), reader, connection, repositoryMetadata.DefaultBranch,
		repositoryMetadata.CommitSHA, false,
	)
	if err != nil {
		return err
	}
	if result.Result.CommitSHA != "" {
		repositoryMetadata.CommitSHA = result.Result.CommitSHA
	}
	return nil
}

// startRepositoryIndexInBackground runs synchronizeRepositoryIndex in its own
// goroutine and returns the job state immediately, so the caller can answer the
// HTTP request without waiting for CodeRAG. A repository never has more than
// one job in flight: concurrent callers observe the running one.
//
// The connection is copied because synchronizeRepositoryIndex updates the
// resolved commit SHA on it, and the caller keeps using its own copy to build
// the response.
func (s *Server) startRepositoryIndexInBackground(
	callerCtx context.Context,
	reader GitFlameRepositoryReader,
	connection *domain.GitFlameConnection,
	ref string,
	force bool,
) indexJobStatus {
	if s.indexer == nil {
		return indexJobStatus{Status: indexStatusDisabled}
	}
	if connection == nil || strings.TrimSpace(connection.Repository.ID) == "" {
		return indexJobStatus{Status: indexStatusDisabled}
	}
	if strings.TrimSpace(ref) == "" {
		ref = connection.DefaultBranch
	}
	repositoryID := connection.Repository.ID
	status, owned := s.indexJobs.begin(repositoryID, connection.ID, ref, force)
	if !owned {
		return status
	}
	snapshot := *connection
	// The job outlives the request, so it gets a fresh context — but it keeps the
	// caller's request id, which is what makes "connect repository" and the index
	// job that it started greppable as one story.
	requestID := observability.RequestIDFromContext(callerCtx)
	go func() {
		ctx, cancel := context.WithTimeout(
			observability.WithRequestID(context.Background(), requestID), s.indexBackgroundTimeout)
		defer cancel()
		result, err := s.synchronizeRepositoryIndex(ctx, reader, &snapshot, ref, "", force)
		finished := s.indexJobs.finish(repositoryID, result.Result, err)
		observability.RepositoryIndexDuration.Observe(float64(finished.DurationMS) / 1000)
		logger := observability.LoggerFromContext(ctx)
		if err != nil {
			observability.RepositoryIndexJobs.Inc("failed")
			logger.Error("rag_index_job",
				slog.String("event", "rag_index_job"),
				slog.String("repository_id", repositoryID), slog.String("ref", ref),
				slog.String("status", indexStatusFailed),
				slog.Int64("duration_ms", finished.DurationMS),
				slog.String("error_code", finished.ErrorCode),
				slog.String("error", finished.Error))
			return
		}
		observability.RepositoryIndexJobs.Inc("completed")
		logger.Info("rag_index_job",
			slog.String("event", "rag_index_job"),
			slog.String("repository_id", repositoryID), slog.String("ref", ref),
			slog.String("status", indexStatusCompleted),
			slog.Int64("duration_ms", finished.DurationMS),
			slog.String("commit_sha", finished.CommitSHA),
			slog.Int("files", finished.FileCount))
	}()
	return status
}

// awaitRepositoryIndexJob blocks while a background job for the repository is
// still running. A failed background job is not an error here: the caller falls
// through to inline indexing, which either succeeds or reports a fresh error.
func (s *Server) awaitRepositoryIndexJob(ctx context.Context, repositoryID string) error {
	status, known, err := s.indexJobs.wait(ctx, repositoryID, s.indexWaitTimeout)
	if err != nil {
		if errors.Is(err, errIndexWaitTimeout) {
			return &IntegrationError{
				Status: http.StatusServiceUnavailable,
				Code:   "rag_indexing_in_progress",
				Detail: "repository indexing is still running, retry in a few moments",
			}
		}
		return err
	}
	if known && status.Status == indexStatusFailed {
		observability.LoggerFromContext(ctx).Warn(
			"rag_index_job_retry_inline",
			slog.String("event", "rag_index_job_retry_inline"),
			slog.String("repository_id", repositoryID),
			slog.String("error_code", status.ErrorCode),
		)
	}
	return nil
}

// synchronizeRepositoryIndex is a synchronous prerequisite. Callers do not
// enqueue recommendation or agent work until it returns successfully.
func (s *Server) synchronizeRepositoryIndex(
	ctx context.Context,
	reader GitFlameRepositoryReader,
	connection *domain.GitFlameConnection,
	ref, explicitCommitSHA string,
	force bool,
) (repositoryIndexSync, error) {
	if s.indexer == nil {
		return repositoryIndexSync{Result: RAGIndexResult{Status: "disabled"}}, nil
	}
	if connection == nil || strings.TrimSpace(connection.Repository.ID) == "" {
		return repositoryIndexSync{}, fmt.Errorf("repository connection is required for indexing")
	}
	if strings.TrimSpace(ref) == "" {
		ref = connection.DefaultBranch
	}
	commitSHA := strings.TrimSpace(explicitCommitSHA)
	if commitSHA == "" {
		if revisionReader, ok := reader.(gitFlameRevisionReader); ok {
			resolved, err := revisionReader.RepositoryRevision(ctx, connection.Repository.ID, ref)
			if err == nil {
				commitSHA = resolved
			}
		}
	}
	if commitSHA == "" {
		commitSHA = strings.TrimSpace(connection.Repository.CommitSHA)
	}
	if commitSHA != "" && !force {
		status, err := s.indexer.Status(ctx, connection.Repository.ID, commitSHA)
		if err != nil {
			return repositoryIndexSync{}, err
		}
		if status.Status == "indexed" {
			return repositoryIndexSync{Result: status}, nil
		}
	}

	tree, err := reader.RepositoryTree(ctx, connection.Repository.ID, ref)
	if err != nil {
		return repositoryIndexSync{}, err
	}
	requested := make([]domain.RepositoryFile, 0, len(tree))
	for _, entry := range tree {
		if entry.Type == "file" || entry.Type == "blob" || entry.Type == "" {
			requested = append(requested, domain.RepositoryFile{Path: entry.Path, Type: entry.Type})
		}
	}
	yamlConfig := ""
	if storedConfig, configErr := s.store.LatestAIConfig(connection.Repository.ID); configErr == nil {
		yamlConfig = storedConfig.Raw
	} else if !errors.Is(configErr, repository.ErrNotFound) {
		return repositoryIndexSync{}, configErr
	}
	var files []domain.RepositoryFile
	if indexReader, ok := reader.(gitFlameIndexReader); ok {
		yamlConfig, files, err = indexReader.RepositoryFilesForIndex(
			ctx, connection.Repository.ID, ref, yamlConfig, requested,
		)
	} else {
		yamlConfig, files, err = reader.RepositoryFiles(
			ctx, connection.Repository.ID, ref, yamlConfig, requested,
		)
	}
	if err != nil {
		return repositoryIndexSync{}, err
	}
	if err := service.ValidateRepositoryFilesForIntegration(files); err != nil {
		return repositoryIndexSync{}, err
	}
	if commitSHA == "" {
		commitSHA = snapshotRevision(files)
	}
	result, err := s.indexer.Index(ctx, RAGIndexRequest{
		RepositoryID:      connection.Repository.ID,
		RepositoryName:    connection.Repository.Name,
		CommitSHA:         commitSHA,
		Source:            connection.RepoURL,
		ConfigurationYAML: yamlConfig,
		Files:             files,
		Force:             force,
	})
	if err != nil {
		return repositoryIndexSync{}, err
	}
	connection.Repository.CommitSHA = commitSHA
	snapshotFiles := make([]domain.RepositorySnapshotFile, 0, len(files))
	for _, file := range files {
		digest := sha256.Sum256([]byte(file.Content))
		snapshotFiles = append(snapshotFiles, domain.RepositorySnapshotFile{
			Path: file.Path, ContentHash: hex.EncodeToString(digest[:]),
		})
	}
	_, _ = s.store.SaveRepositorySnapshot(domain.RepositorySnapshot{
		RepositoryID: connection.Repository.ID,
		ConnectionID: connection.ID,
		Ref:          ref,
		CommitSHA:    commitSHA,
		FileCount:    len(files),
		Status:       "indexed",
	}, snapshotFiles)
	observability.LoggerFromContext(ctx).Info(
		"rag_index",
		slog.String("event", "rag_index"),
		slog.String("repository_id", connection.Repository.ID),
		slog.String("commit_sha", commitSHA),
		slog.Int("files", result.FileCount),
		slog.Int("chunks", result.ChunkCount),
		slog.Int("embeddings", result.EmbeddingCount),
		slog.String("status", result.Status),
	)
	return repositoryIndexSync{Result: result, YAMLConfig: yamlConfig, Files: files}, nil
}

func snapshotRevision(files []domain.RepositoryFile) string {
	ordered := append([]domain.RepositoryFile(nil), files...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	digest := sha256.New()
	for _, file := range ordered {
		_, _ = digest.Write([]byte(file.Path))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write([]byte(file.Content))
		_, _ = digest.Write([]byte{0})
	}
	return "snapshot-" + hex.EncodeToString(digest.Sum(nil))
}
