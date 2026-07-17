package httpapi

import (
	"context"
	"errors"
	"net/http"
	"path"
	"strings"
	"time"

	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/repository"
	"gitflame-codepilot/backend/internal/security"
)

type gitFlameSessionRequest struct {
	AccessToken string `json:"access_token"`
}

type gitFlameConnectionRequest struct {
	AccessToken    string                    `json:"access_token"`
	Repository     domain.RepositoryMetadata `json:"repository"`
	RepoURL        string                    `json:"repo_url"`
	DefaultBranch  string                    `json:"default_branch"`
	Scopes         []string                  `json:"scopes"`
	TokenExpiresAt *time.Time                `json:"token_expires_at"`
}

func (s *Server) createGitFlameSession(w http.ResponseWriter, r *http.Request) {
	var req gitFlameSessionRequest
	if err := decode(r, &req); err != nil {
		problem(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	session, cookieValue, _, err := s.createSessionFromGitFlameToken(r.Context(), req.AccessToken)
	if err != nil {
		integrationError(w, err, "gitflame_auth_error")
		return
	}
	s.setSessionCookie(w, cookieValue, session.ExpiresAt)
	write(w, http.StatusCreated, map[string]any{"user": session.User, "session_expires_at": session.ExpiresAt})
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	session, err := s.authenticate(r)
	if err != nil {
		authError(w, err)
		return
	}
	if err := s.store.RevokeAppSession(session.ID); err != nil {
		resourceError(w, err, "session_not_found", "application session was not found")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: s.sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.sessionSecure, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) saveGitFlameConnection(w http.ResponseWriter, r *http.Request) {
	var req gitFlameConnectionRequest
	if err := decode(r, &req); err != nil {
		problem(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	session, err := s.authenticate(r)
	if err != nil {
		if !errors.Is(err, repository.ErrNotFound) {
			authError(w, err)
			return
		}
		var created bool
		var cookieValue string
		session, cookieValue, created, err = s.createSessionFromGitFlameToken(r.Context(), req.AccessToken)
		if err != nil {
			integrationError(w, err, "gitflame_auth_error")
			return
		}
		if created {
			s.setSessionCookie(w, cookieValue, session.ExpiresAt)
		}
	}
	connection, err := s.connectionFromRequest(r.Context(), session.User, req, "")
	if err != nil {
		integrationError(w, err, "gitflame_connection_error")
		return
	}
	saved, err := s.store.SaveGitFlameConnection(connection)
	if err != nil {
		problem(w, http.StatusInternalServerError, "storage_error", err.Error())
		return
	}
	write(w, http.StatusCreated, saved)
}

func (s *Server) reconnectGitFlameConnection(w http.ResponseWriter, r *http.Request) {
	session, err := s.authenticate(r)
	if err != nil {
		authError(w, err)
		return
	}
	existing, err := s.store.UserGitFlameConnection(session.User.ID, r.PathValue("id"))
	if err != nil {
		resourceError(w, err, "connection_not_found", "GitFlame connection was not found")
		return
	}
	var req gitFlameConnectionRequest
	if err := decode(r, &req); err != nil {
		problem(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if req.Repository.ID == "" {
		req.Repository = existing.Repository
	}
	if req.RepoURL == "" {
		req.RepoURL = existing.RepoURL
	}
	if req.DefaultBranch == "" {
		req.DefaultBranch = existing.DefaultBranch
	}
	connection, err := s.connectionFromRequest(r.Context(), session.User, req, existing.ID)
	if err != nil {
		integrationError(w, err, "gitflame_connection_error")
		return
	}
	saved, err := s.store.SaveGitFlameConnection(connection)
	if err != nil {
		problem(w, http.StatusInternalServerError, "storage_error", err.Error())
		return
	}
	write(w, http.StatusOK, saved)
}

func (s *Server) revokeGitFlameConnection(w http.ResponseWriter, r *http.Request) {
	session, err := s.authenticate(r)
	if err != nil {
		authError(w, err)
		return
	}
	connection, err := s.store.RevokeGitFlameConnection(session.User.ID, r.PathValue("id"))
	if err != nil {
		resourceError(w, err, "connection_not_found", "GitFlame connection was not found")
		return
	}
	write(w, http.StatusOK, connection)
}

func (s *Server) connectionFromRequest(ctx context.Context, user domain.AppUser, req gitFlameConnectionRequest, existingID string) (domain.GitFlameConnection, error) {
	if s.credentialCipher == nil {
		return domain.GitFlameConnection{}, &IntegrationError{Status: http.StatusServiceUnavailable, Code: "credential_cipher_unconfigured", Detail: "GITFLAME_CREDENTIAL_KEY is required to store GitFlame tokens"}
	}
	if strings.TrimSpace(req.AccessToken) == "" {
		return domain.GitFlameConnection{}, &IntegrationError{Status: http.StatusUnprocessableEntity, Code: "missing_access_token", Detail: "access_token is required"}
	}
	profile, err := s.validateGitFlameToken(ctx, req.AccessToken)
	if err != nil {
		return domain.GitFlameConnection{}, err
	}
	if profile.ID != "" && user.GitFlameUserID != "" && profile.ID != user.GitFlameUserID {
		return domain.GitFlameConnection{}, &IntegrationError{Status: http.StatusForbidden, Code: "gitflame_user_mismatch", Detail: "GitFlame token belongs to another user"}
	}
	if strings.TrimSpace(req.Repository.ID) == "" {
		resolved, err := s.resolveConnectionRepository(ctx, req.AccessToken, req.RepoURL)
		if err != nil {
			return domain.GitFlameConnection{}, err
		}
		req.Repository = resolved.Metadata
		req.RepoURL = resolved.RepoURL
	}
	if strings.TrimSpace(req.Repository.ID) == "" {
		return domain.GitFlameConnection{}, &IntegrationError{Status: http.StatusUnprocessableEntity, Code: "missing_repository_id", Detail: "repository.id or repo_url is required"}
	}
	if strings.TrimSpace(req.RepoURL) == "" {
		req.RepoURL = req.Repository.WebURL
	}
	defaultBranch := req.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = req.Repository.DefaultBranch
	}
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	req.Repository.DefaultBranch = defaultBranch
	ciphertext, nonce, keyVersion, err := s.credentialCipher.Encrypt(req.AccessToken, credentialAAD(user.ID, req.Repository.ID))
	if err != nil {
		return domain.GitFlameConnection{}, err
	}
	now := time.Now().UTC()
	return domain.GitFlameConnection{
		ID: existingID, UserID: user.ID, Repository: req.Repository, RepoURL: req.RepoURL, DefaultBranch: defaultBranch,
		TokenMaterial: domain.GitFlameTokenMaterial{Ciphertext: ciphertext, Nonce: nonce, KeyVersion: keyVersion},
		TokenLast4:    security.TokenLast4(req.AccessToken), TokenStatus: "active", Scopes: req.Scopes,
		TokenExpiresAt: req.TokenExpiresAt, LastValidatedAt: &now,
	}, nil
}

func (s *Server) resolveConnectionRepository(ctx context.Context, accessToken, repoURL string) (resolvedGitFlameRepository, error) {
	client := NewGitFlameClient(s.gitflameBaseURL, accessToken, s.gitflameTimeout)
	if client != nil {
		return client.ResolveRepository(ctx, repoURL)
	}
	parsed, err := parseGitFlameRepositoryURL(repoURL)
	if err != nil {
		return resolvedGitFlameRepository{}, &IntegrationError{Status: http.StatusUnprocessableEntity, Code: "invalid_repo_url", Detail: err.Error()}
	}
	return resolvedGitFlameRepository{
		Metadata: domain.RepositoryMetadata{
			ID:            parsed.Path,
			Name:          path.Base(parsed.Path),
			DefaultBranch: "main",
			WebURL:        parsed.WebURL,
		},
		RepoURL: parsed.WebURL,
	}, nil
}

func (s *Server) createSessionFromGitFlameToken(ctx context.Context, accessToken string) (*domain.AppSession, string, bool, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, "", false, &IntegrationError{Status: http.StatusUnprocessableEntity, Code: "missing_access_token", Detail: "access_token is required"}
	}
	profile, err := s.validateGitFlameToken(ctx, accessToken)
	if err != nil {
		return nil, "", false, err
	}
	user, err := s.store.UpsertAppUser(domain.AppUser{GitFlameUserID: profile.ID, Username: profile.Username})
	if err != nil {
		return nil, "", false, err
	}
	cookieValue, tokenHash, err := security.GenerateSessionToken()
	if err != nil {
		return nil, "", false, err
	}
	expiresAt := time.Now().UTC().Add(s.sessionTTL)
	session, err := s.store.CreateAppSession(user.ID, tokenHash, expiresAt)
	if err != nil {
		return nil, "", false, err
	}
	return session, cookieValue, true, nil
}

func (s *Server) validateGitFlameToken(ctx context.Context, accessToken string) (GitFlameUserProfile, error) {
	client := NewGitFlameClient(s.gitflameBaseURL, accessToken, s.gitflameTimeout)
	if client == nil {
		if validator, ok := s.gitflame.(interface {
			CurrentUser(context.Context) (GitFlameUserProfile, error)
		}); ok {
			return validator.CurrentUser(ctx)
		}
		return GitFlameUserProfile{}, &IntegrationError{Status: http.StatusServiceUnavailable, Code: "gitflame_client_unavailable", Detail: "GITFLAME_BASE_URL is required to validate GitFlame tokens"}
	}
	return client.CurrentUser(ctx)
}

func (s *Server) gitFlameSourceForRepository(r *http.Request, repositoryID string) (GitFlameSource, *domain.GitFlameConnection, error) {
	if s.credentialCipher == nil || strings.TrimSpace(s.gitflameBaseURL) == "" {
		return s.gitflame, nil, nil
	}
	session, err := s.authenticate(r)
	if err != nil {
		return nil, nil, &IntegrationError{Status: http.StatusUnauthorized, Code: "unauthorized", Detail: "valid application session cookie is required"}
	}
	connection, err := s.store.UserGitFlameConnectionByRepository(session.User.ID, repositoryID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, &IntegrationError{Status: http.StatusUnauthorized, Code: "gitflame_connection_required", Detail: "GitFlame connection for this repository is required"}
		}
		return nil, nil, err
	}
	if connection.TokenStatus != "" && connection.TokenStatus != "active" {
		return nil, nil, &IntegrationError{Status: http.StatusUnauthorized, Code: "gitflame_token_inactive", Detail: "GitFlame connection token is not active"}
	}
	if connection.TokenExpiresAt != nil && !connection.TokenExpiresAt.After(time.Now().UTC()) {
		return nil, nil, &IntegrationError{Status: http.StatusUnauthorized, Code: "gitflame_token_expired", Detail: "GitFlame connection token is expired"}
	}
	if len(connection.TokenMaterial.Ciphertext) == 0 || len(connection.TokenMaterial.Nonce) == 0 || connection.TokenMaterial.KeyVersion == 0 {
		return nil, nil, &IntegrationError{Status: http.StatusUnauthorized, Code: "gitflame_reauth_required", Detail: "GitFlame connection must be reconnected before use"}
	}
	accessToken, err := s.credentialCipher.Decrypt(
		connection.TokenMaterial.Ciphertext,
		connection.TokenMaterial.Nonce,
		connection.TokenMaterial.KeyVersion,
		credentialAAD(session.User.ID, connection.Repository.ID),
	)
	if err != nil {
		return nil, nil, &IntegrationError{Status: http.StatusUnauthorized, Code: "gitflame_reauth_required", Detail: "GitFlame connection token could not be decrypted"}
	}
	return NewGitFlameClient(s.gitflameBaseURL, accessToken, s.gitflameTimeout), connection, nil
}

func (s *Server) gitFlameReaderForRepository(r *http.Request, repositoryID string) (GitFlameRepositoryReader, *domain.GitFlameConnection, error) {
	source, connection, err := s.gitFlameSourceForRepository(r, repositoryID)
	if err != nil {
		return nil, nil, err
	}
	reader, ok := source.(GitFlameRepositoryReader)
	if !ok {
		return nil, nil, &IntegrationError{Status: http.StatusServiceUnavailable, Code: "gitflame_client_unavailable", Detail: "GitFlame repository client is not configured"}
	}
	return reader, connection, nil
}

func (s *Server) gitFlameReaderForConnection(r *http.Request, connectionID string) (GitFlameRepositoryReader, *domain.GitFlameConnection, error) {
	session, err := s.authenticate(r)
	if err != nil {
		return nil, nil, &IntegrationError{Status: http.StatusUnauthorized, Code: "unauthorized", Detail: "valid application session cookie is required"}
	}
	connection, err := s.store.UserGitFlameConnection(session.User.ID, connectionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, &IntegrationError{Status: http.StatusNotFound, Code: "connection_not_found", Detail: "GitFlame connection was not found"}
		}
		return nil, nil, err
	}
	if connection.RevokedAt != nil || (connection.TokenStatus != "" && connection.TokenStatus != "active") {
		return nil, nil, &IntegrationError{Status: http.StatusUnauthorized, Code: "gitflame_token_inactive", Detail: "GitFlame connection token is not active"}
	}
	if connection.TokenExpiresAt != nil && !connection.TokenExpiresAt.After(time.Now().UTC()) {
		return nil, nil, &IntegrationError{Status: http.StatusUnauthorized, Code: "gitflame_token_expired", Detail: "GitFlame connection token is expired"}
	}

	if s.credentialCipher == nil || strings.TrimSpace(s.gitflameBaseURL) == "" {
		reader, ok := s.gitflame.(GitFlameRepositoryReader)
		if !ok {
			return nil, nil, &IntegrationError{Status: http.StatusServiceUnavailable, Code: "gitflame_client_unavailable", Detail: "GitFlame repository client is not configured"}
		}
		return reader, connection, nil
	}
	if len(connection.TokenMaterial.Ciphertext) == 0 || len(connection.TokenMaterial.Nonce) == 0 || connection.TokenMaterial.KeyVersion == 0 {
		return nil, nil, &IntegrationError{Status: http.StatusUnauthorized, Code: "gitflame_reauth_required", Detail: "GitFlame connection must be reconnected before use"}
	}
	accessToken, err := s.credentialCipher.Decrypt(
		connection.TokenMaterial.Ciphertext,
		connection.TokenMaterial.Nonce,
		connection.TokenMaterial.KeyVersion,
		credentialAAD(session.User.ID, connection.Repository.ID),
	)
	if err != nil {
		return nil, nil, &IntegrationError{Status: http.StatusUnauthorized, Code: "gitflame_reauth_required", Detail: "GitFlame connection token could not be decrypted"}
	}
	return NewGitFlameClient(s.gitflameBaseURL, accessToken, s.gitflameTimeout), connection, nil
}

func (s *Server) authenticate(r *http.Request) (*domain.AppSession, error) {
	cookie, err := r.Cookie(s.sessionCookie)
	if err != nil {
		return nil, repository.ErrNotFound
	}
	tokenHash, err := security.SessionTokenHashFromCookie(cookie.Value)
	if err != nil {
		return nil, repository.ErrNotFound
	}
	return s.store.AppSessionByTokenHash(tokenHash)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, value string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: s.sessionCookie, Value: value, Path: "/", Expires: expiresAt,
		HttpOnly: true, Secure: s.sessionSecure, SameSite: http.SameSiteLaxMode,
	})
}

func authError(w http.ResponseWriter, err error) {
	if errors.Is(err, repository.ErrNotFound) {
		problem(w, http.StatusUnauthorized, "unauthorized", "valid application session cookie is required")
		return
	}
	problem(w, http.StatusInternalServerError, "auth_error", err.Error())
}

func credentialAAD(userID, repositoryID string) string {
	return userID + ":" + repositoryID
}
