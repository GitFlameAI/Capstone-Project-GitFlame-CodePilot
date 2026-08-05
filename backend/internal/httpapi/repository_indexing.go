package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"gitflame-codepilot/backend/internal/domain"
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
	Tree       []GitFlameTreeEntry
	YAMLConfig string
	Files      []domain.RepositoryFile
}

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
	log.Printf(
		"rag_index repository_id=%s commit_sha=%s files=%d chunks=%d embeddings=%d status=%s",
		connection.Repository.ID, commitSHA, result.FileCount, result.ChunkCount,
		result.EmbeddingCount, result.Status,
	)
	return repositoryIndexSync{Result: result, Tree: tree, YAMLConfig: yamlConfig, Files: files}, nil
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
