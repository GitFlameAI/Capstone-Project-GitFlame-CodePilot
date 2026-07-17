package httpapi

import (
	"net/http"
	"strings"

	"gitflame-codepilot/backend/internal/domain"
)

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
	yamlConfig, files, err := reader.RepositoryFiles(r.Context(), connection.Repository.ID, ref, "", nil)
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
	if !repositoryFilesNeedContent(req.RepositoryFiles, req.RepositoryContext) {
		return req, nil
	}
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
	yamlConfig, hydrated, err := reader.RepositoryFiles(r.Context(), req.Repository.ID, ref, req.YAMLConfig, files)
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

func repositoryFilesNeedContent(files []domain.RepositoryFile, legacyPaths []string) bool {
	if len(files) == 0 {
		return len(legacyPaths) > 0
	}
	for _, file := range files {
		if strings.TrimSpace(file.Content) == "" {
			return true
		}
	}
	return false
}
