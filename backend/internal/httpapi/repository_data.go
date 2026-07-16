package httpapi

import (
	"net/http"
	"strings"
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
