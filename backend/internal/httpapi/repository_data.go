package httpapi

import (
	"context"
	"net/http"
	"strings"

	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/service"
)

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
	if _, err := s.synchronizeRepositoryIndex(r.Context(), reader, connection, connection.DefaultBranch, "", true); err != nil {
		integrationError(w, err, "rag_indexing_failed")
		return
	}
	write(w, http.StatusOK, map[string]any{"repository_id": connection.Repository.ID, "yaml_config": cfg.Raw, "status": "saved"})
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
	syncResult, err := s.synchronizeRepositoryIndex(r.Context(), reader, connection, ref, "", false)
	if err != nil {
		integrationError(w, err, "rag_indexing_failed")
		return
	}
	tree := syncResult.Tree
	if tree == nil {
		tree, err = reader.RepositoryTree(r.Context(), connection.Repository.ID, ref)
		if err != nil {
			integrationError(w, err, "gitflame_tree_error")
			return
		}
	}
	_ = s.store.TouchGitFlameConnection(connection.UserID, connection.ID)
	write(w, http.StatusOK, map[string]any{
		"repository_id": connection.Repository.ID,
		"ref":           ref,
		"tree":          tree,
		"index":         syncResult.Result,
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
	syncResult, err := s.synchronizeRepositoryIndex(r.Context(), reader, connection, ref, "", false)
	if err != nil {
		integrationError(w, err, "rag_indexing_failed")
		return
	}
	yamlConfig, files := syncResult.YAMLConfig, syncResult.Files
	if files == nil {
		yamlConfig, files, err = reader.RepositoryFiles(r.Context(), connection.Repository.ID, ref, "", nil)
		if err != nil {
			integrationError(w, err, "gitflame_files_error")
			return
		}
	}
	_ = s.store.TouchGitFlameConnection(connection.UserID, connection.ID)
	write(w, http.StatusOK, map[string]any{
		"repository_id":    connection.Repository.ID,
		"ref":              ref,
		"yaml_config":      yamlConfig,
		"repository_files": files,
		"index":            syncResult.Result,
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
