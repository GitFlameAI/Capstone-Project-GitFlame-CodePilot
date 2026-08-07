package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/repository"
	"gitflame-codepilot/backend/internal/service"
)

// repositoryIndexStatus exposes the background indexing job for a connected
// repository so the UI (and an operator) can tell whether the repository is
// still being prepared, and why it failed if it did.
func (s *Server) repositoryIndexStatus(w http.ResponseWriter, r *http.Request) {
	connection, err := s.authenticatedConnection(r, r.PathValue("id"))
	if err != nil {
		integrationError(w, err, "gitflame_repository_error")
		return
	}
	if s.indexer == nil {
		write(w, http.StatusOK, indexJobStatus{
			RepositoryID: connection.Repository.ID,
			ConnectionID: connection.ID,
			Status:       indexStatusDisabled,
		})
		return
	}
	status, known := s.indexJobs.status(connection.Repository.ID)
	if !known {
		// No job has run in this process. The index may still exist in CodeRAG;
		// the next analysis verifies it and rebuilds it when needed.
		status = indexJobStatus{
			RepositoryID: connection.Repository.ID,
			ConnectionID: connection.ID,
			Ref:          connection.DefaultBranch,
			Status:       indexStatusIdle,
		}
	}
	write(w, http.StatusOK, status)
}

func (s *Server) saveRepositoryConfig(w http.ResponseWriter, r *http.Request) {
	connection, err := s.authenticatedConnection(r, r.PathValue("id"))
	if err != nil {
		integrationError(w, err, "gitflame_repository_error")
		return
	}
	var req struct {
		YAMLConfig string `json:"yaml_config"`
	}
	if err := decode(r, &req); err != nil {
		problem(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	cfg, err := service.ParseAIConfig(req.YAMLConfig)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	if err := s.store.SaveAIConfig(connection.Repository, cfg); err != nil {
		problem(w, http.StatusInternalServerError, "storage_error", err.Error())
		return
	}
	reader, err := s.gitFlameReaderFromStoredConnection(connection)
	if err != nil {
		integrationError(w, err, "gitflame_repository_error")
		return
	}
	// The .ai.yml controls which files are indexed, so the index is rebuilt —
	// in the background, so that saving the configuration stays instant.
	indexJob := s.startRepositoryIndexInBackground(r.Context(), reader, connection, connection.DefaultBranch, true)
	write(w, http.StatusOK, map[string]any{
		"repository_id": connection.Repository.ID,
		"yaml_config":   cfg.Raw,
		"status":        "saved",
		"index":         indexJob,
	})
}

func (s *Server) repositoryTree(w http.ResponseWriter, r *http.Request) {
	reader, connection, err := s.gitFlameReaderForConnection(r, r.PathValue("id"))
	if err != nil {
		integrationError(w, err, "gitflame_repository_error")
		return
	}
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if ref == "" {
		ref = connection.DefaultBranch
	}
	// Browsing the repository must stay fast, so the tree is read directly from
	// GitFlame and indexing only gets nudged in the background. The reported
	// `index` state lets the caller show progress without blocking on it.
	indexJob := s.startRepositoryIndexInBackground(r.Context(), reader, connection, ref, false)
	tree, err := reader.RepositoryTree(r.Context(), connection.Repository.ID, ref)
	if err != nil {
		integrationError(w, err, "gitflame_tree_error")
		return
	}
	_ = s.store.TouchGitFlameConnection(connection.UserID, connection.ID)
	write(w, http.StatusOK, map[string]any{
		"repository_id": connection.Repository.ID,
		"ref":           ref,
		"tree":          tree,
		"index":         indexJob,
	})
}

func (s *Server) repositoryFiles(w http.ResponseWriter, r *http.Request) {
	reader, connection, err := s.gitFlameReaderForConnection(r, r.PathValue("id"))
	if err != nil {
		integrationError(w, err, "gitflame_repository_error")
		return
	}
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if ref == "" {
		ref = connection.DefaultBranch
	}
	storedConfig := ""
	if stored, configErr := s.store.LatestAIConfig(connection.Repository.ID); configErr == nil {
		storedConfig = stored.Raw
	} else if !errors.Is(configErr, repository.ErrNotFound) {
		problem(w, http.StatusInternalServerError, "storage_error", configErr.Error())
		return
	}
	// Same reasoning as repositoryTree: file contents come straight from
	// GitFlame and indexing runs in the background.
	indexJob := s.startRepositoryIndexInBackground(r.Context(), reader, connection, ref, false)
	yamlConfig, files, err := repositoryFilesForAnalysis(
		r.Context(), reader, connection.Repository.ID, ref, storedConfig, nil,
	)
	if err != nil {
		integrationError(w, err, "gitflame_files_error")
		return
	}
	_ = s.store.TouchGitFlameConnection(connection.UserID, connection.ID)
	write(w, http.StatusOK, map[string]any{
		"repository_id":    connection.Repository.ID,
		"ref":              ref,
		"yaml_config":      yamlConfig,
		"repository_files": files,
		"index":            indexJob,
	})
}

func (s *Server) repositoryIssues(w http.ResponseWriter, r *http.Request) {
	reader, connection, err := s.gitFlameReaderForConnection(r, r.PathValue("id"))
	if err != nil {
		integrationError(w, err, "gitflame_repository_error")
		return
	}
	issues, err := reader.RepositoryIssues(r.Context(), connection.Repository.ID)
	if err != nil {
		integrationError(w, err, "gitflame_issues_error")
		return
	}
	_ = s.store.TouchGitFlameConnection(connection.UserID, connection.ID)
	write(w, http.StatusOK, map[string]any{
		"repository_id": connection.Repository.ID,
		"issues":        issues,
	})
}

func (s *Server) hydrateAnalyzeRequest(r *http.Request, req domain.IssueAnalyzeRequest) (domain.IssueAnalyzeRequest, error) {
	if strings.TrimSpace(req.Repository.ID) == "" {
		return req, nil
	}
	files := append([]domain.RepositoryFile(nil), req.RepositoryFiles...)
	if len(files) == 0 {
		for _, filePath := range req.RepositoryContext {
			files = append(files, domain.RepositoryFile{Path: filePath})
		}
	}
	reader, connection, err := s.gitFlameReaderForRepository(r, req.Repository.ID)
	if err != nil {
		return req, err
	}
	ref := req.Repository.DefaultBranch
	yamlConfig, hydrated, err := repositoryFilesForAnalysis(
		r.Context(), reader, req.Repository.ID, ref, req.YAMLConfig, files,
	)
	if err != nil {
		return req, err
	}
	req.YAMLConfig = yamlConfig
	req.RepositoryFiles = hydrated
	req.RepositoryContext = nil
	if connection != nil {
		_ = s.store.TouchGitFlameConnection(connection.UserID, connection.ID)
	}
	return req, nil
}

func repositoryFilesForAnalysis(
	ctx context.Context,
	reader GitFlameRepositoryReader,
	repositoryID, ref, yamlConfig string,
	requested []domain.RepositoryFile,
) (string, []domain.RepositoryFile, error) {
	if completeReader, ok := reader.(gitFlameIndexReader); ok {
		return completeReader.RepositoryFilesForIndex(
			ctx, repositoryID, ref, yamlConfig, nil,
		)
	}
	return reader.RepositoryFiles(ctx, repositoryID, ref, yamlConfig, requested)
}

func repositoryFilesNeedContent(files []domain.RepositoryFile, legacyPaths []string) bool {
	if len(files) == 0 {
		return true
	}
	for _, file := range files {
		if strings.TrimSpace(file.Content) == "" {
			return true
		}
	}
	return false
}
