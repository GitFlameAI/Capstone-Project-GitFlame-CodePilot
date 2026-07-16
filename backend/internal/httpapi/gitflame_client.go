package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/service"
)

type GitFlameClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type gitFlameTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type gitFlameCommitAction struct {
	Action   string `json:"action"`
	FilePath string `json:"file_path"`
	Content  string `json:"content,omitempty"`
}

type GitFlameUserProfile struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Login    string `json:"login"`
	Name     string `json:"name"`
	Email    string `json:"email"`
}

type resolvedGitFlameRepository struct {
	Metadata domain.RepositoryMetadata
	RepoURL  string
}

func NewGitFlameClient(baseURL, apiKey string, timeout time.Duration) *GitFlameClient {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &GitFlameClient{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, httpClient: &http.Client{Timeout: timeout}}
}

func (c *GitFlameClient) BuildAnalyzeRequest(ctx context.Context, webhook GitFlameIssueWebhook) (domain.IssueAnalyzeRequest, error) {
	ref := webhook.Ref
	if ref == "" {
		ref = webhook.Repository.CommitSHA
	}
	if ref == "" {
		ref = webhook.CommitSHA
	}
	if ref == "" {
		ref = webhook.Repository.DefaultBranch
	}
	yamlConfig := webhook.YAMLConfig
	if strings.TrimSpace(yamlConfig) == "" {
		content, err := c.fetchFileContent(ctx, webhook.Repository.ID, ".ai.yml", ref)
		if err != nil {
			return domain.IssueAnalyzeRequest{}, err
		}
		yamlConfig = content
	}
	cfg, err := service.ParseAIConfig(yamlConfig)
	if err != nil {
		return domain.IssueAnalyzeRequest{}, &IntegrationError{Status: http.StatusUnprocessableEntity, Code: "invalid_ai_config", Detail: err.Error()}
	}
	files := append([]domain.RepositoryFile(nil), webhook.RepositoryFiles...)
	if len(files) == 0 {
		tree, err := c.fetchTree(ctx, webhook.Repository.ID, ref)
		if err != nil {
			return domain.IssueAnalyzeRequest{}, err
		}
		for _, entry := range tree {
			if len(files) >= cfg.MaxFiles {
				break
			}
			if entry.Type != "" && entry.Type != "file" && entry.Type != "blob" {
				continue
			}
			if !matchesRepositoryRules(entry.Path, cfg.IncludePatterns, cfg.ExcludePatterns) {
				continue
			}
			content, err := c.fetchFileContent(ctx, webhook.Repository.ID, entry.Path, ref)
			if err != nil {
				return domain.IssueAnalyzeRequest{}, err
			}
			files = append(files, domain.RepositoryFile{Path: entry.Path, Content: content})
		}
	}
	if len(files) == 0 {
		return domain.IssueAnalyzeRequest{}, &IntegrationError{Status: http.StatusUnprocessableEntity, Code: "empty_repository_context", Detail: "GitFlame API returned no repository files after applying .ai.yml analysis rules"}
	}
	repository := webhook.Repository
	if repository.CommitSHA == "" {
		repository.CommitSHA = webhook.CommitSHA
	}
	return domain.IssueAnalyzeRequest{Repository: repository, Issue: webhook.Issue, YAMLConfig: yamlConfig, RepositoryFiles: files, Metadata: webhook.Metadata}, nil
}

func (c *GitFlameClient) ApplyGeneratedFiles(ctx context.Context, repository domain.RepositoryMetadata, contract domain.GeneratedFilesContract) (domain.GitFlameApplyResult, error) {
	if strings.TrimSpace(repository.ID) == "" {
		return domain.GitFlameApplyResult{}, &IntegrationError{Status: http.StatusUnprocessableEntity, Code: "missing_repository_id", Detail: "repository.id is required to apply generated files"}
	}
	baseBranch := contract.BaseBranch
	if strings.TrimSpace(baseBranch) == "" {
		baseBranch = repository.DefaultBranch
	}
	if strings.TrimSpace(baseBranch) == "" {
		baseBranch = "main"
	}
	if err := c.createBranch(ctx, repository.ID, contract.BranchName, baseBranch); err != nil {
		return domain.GitFlameApplyResult{}, err
	}
	commitSHA, err := c.commitGeneratedFiles(ctx, repository.ID, contract)
	if err != nil {
		return domain.GitFlameApplyResult{}, err
	}
	prID, prURL, err := c.createPullRequest(ctx, repository.ID, contract, baseBranch)
	if err != nil {
		return domain.GitFlameApplyResult{}, err
	}
	return domain.GitFlameApplyResult{BranchName: contract.BranchName, CommitSHA: commitSHA, PullRequestID: prID, PullRequestURL: prURL}, nil
}

func (c *GitFlameClient) CurrentUser(ctx context.Context) (GitFlameUserProfile, error) {
	var lastErr error
	for _, endpoint := range []string{"/api/v1/user", "/api/v1/users/me", "/api/v1/profile"} {
		var response map[string]any
		err := c.getJSON(ctx, endpoint, "", &response)
		if err != nil {
			lastErr = err
			continue
		}
		profile := GitFlameUserProfile{
			ID:       firstString(response, "id", "user.id", "data.id"),
			Username: firstString(response, "username", "login", "user.username", "user.login", "data.username", "data.login"),
			Login:    firstString(response, "login", "username", "user.login", "user.username", "data.login", "data.username"),
			Name:     firstString(response, "name", "user.name", "data.name"),
			Email:    firstString(response, "email", "user.email", "data.email"),
		}
		if profile.ID == "" && profile.Username != "" {
			profile.ID = profile.Username
		}
		if profile.Username == "" {
			profile.Username = profile.Login
		}
		if profile.Username == "" {
			profile.Username = profile.Name
		}
		if profile.ID != "" && profile.Username != "" {
			return profile, nil
		}
		lastErr = &IntegrationError{Status: http.StatusBadGateway, Code: "invalid_gitflame_user_response", Detail: "GitFlame API did not return user id and username"}
	}
	if lastErr == nil {
		lastErr = &IntegrationError{Status: http.StatusUnauthorized, Code: "invalid_gitflame_token", Detail: "GitFlame token could not be validated"}
	}
	return GitFlameUserProfile{}, lastErr
}

func (c *GitFlameClient) ResolveRepository(ctx context.Context, rawURL string) (resolvedGitFlameRepository, error) {
	parsed, err := parseGitFlameRepositoryURL(rawURL)
	if err != nil {
		return resolvedGitFlameRepository{}, &IntegrationError{Status: http.StatusUnprocessableEntity, Code: "invalid_repo_url", Detail: err.Error()}
	}
	fallback := resolvedGitFlameRepository{
		Metadata: domain.RepositoryMetadata{
			ID:            parsed.Path,
			Name:          path.Base(parsed.Path),
			DefaultBranch: "main",
			WebURL:        parsed.WebURL,
		},
		RepoURL: parsed.WebURL,
	}
	for _, endpoint := range []string{
		"/api/v1/repositories/" + url.PathEscape(parsed.Path),
		"/api/v1/projects/" + url.PathEscape(parsed.Path),
		"/api/v1/repos/" + url.PathEscape(parsed.Path),
	} {
		var response map[string]any
		if err := c.getJSON(ctx, endpoint, "", &response); err != nil {
			continue
		}
		metadata := repositoryMetadataFromGitFlameResponse(response, fallback.Metadata)
		return resolvedGitFlameRepository{Metadata: metadata, RepoURL: firstNonEmpty(metadata.WebURL, fallback.RepoURL)}, nil
	}
	return fallback, nil
}

func (c *GitFlameClient) createBranch(ctx context.Context, repositoryID, branchName, ref string) error {
	payload := map[string]string{"branch_name": branchName, "name": branchName, "ref": ref}
	var response map[string]any
	err := c.postJSON(ctx, fmt.Sprintf("/api/v1/repositories/%s/branches", url.PathEscape(repositoryID)), payload, &response)
	var integration *IntegrationError
	if errors.As(err, &integration) && integration.Status == http.StatusConflict {
		return nil
	}
	return err
}

func (c *GitFlameClient) commitGeneratedFiles(ctx context.Context, repositoryID string, contract domain.GeneratedFilesContract) (string, error) {
	actions := make([]gitFlameCommitAction, 0, len(contract.Files))
	for _, file := range contract.Files {
		actions = append(actions, gitFlameCommitAction{Action: file.Action, FilePath: file.Path, Content: file.Content})
	}
	payload := map[string]any{
		"branch":         contract.BranchName,
		"branch_name":    contract.BranchName,
		"commit_message": contract.CommitMessage,
		"message":        contract.CommitMessage,
		"actions":        actions,
		"files":          actions,
	}
	var response map[string]any
	if err := c.postJSON(ctx, fmt.Sprintf("/api/v1/repositories/%s/commits", url.PathEscape(repositoryID)), payload, &response); err != nil {
		return "", err
	}
	return firstString(response, "sha", "commit_sha", "id", "commit.id", "commit.sha"), nil
}

func (c *GitFlameClient) createPullRequest(ctx context.Context, repositoryID string, contract domain.GeneratedFilesContract, baseBranch string) (string, string, error) {
	payload := map[string]any{
		"title":         contract.PRTitle,
		"body":          contract.Summary,
		"description":   contract.Summary,
		"source_branch": contract.BranchName,
		"head":          contract.BranchName,
		"target_branch": baseBranch,
		"base":          baseBranch,
		"reviewer":      contract.Reviewer,
	}
	var response map[string]any
	if err := c.postJSON(ctx, fmt.Sprintf("/api/v1/repositories/%s/pull-requests", url.PathEscape(repositoryID)), payload, &response); err != nil {
		return "", "", err
	}
	id := firstString(response, "id", "number", "iid", "pull_request.id")
	prURL := firstString(response, "pull_request_url", "html_url", "web_url", "url", "pull_request.url", "pull_request.html_url")
	return id, prURL, nil
}

func (c *GitFlameClient) fetchTree(ctx context.Context, repositoryID, ref string) ([]gitFlameTreeEntry, error) {
	var tree []gitFlameTreeEntry
	if err := c.getJSON(ctx, fmt.Sprintf("/api/v1/repositories/%s/tree", url.PathEscape(repositoryID)), ref, &tree); err != nil {
		return nil, err
	}
	return tree, nil
}

func (c *GitFlameClient) fetchFileContent(ctx context.Context, repositoryID, filePath, ref string) (string, error) {
	var response struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	err := c.getJSON(ctx, fmt.Sprintf("/api/v1/repositories/%s/files/%s", url.PathEscape(repositoryID), url.PathEscape(filePath)), ref, &response)
	if err == nil {
		return response.Content, nil
	}
	var integration *IntegrationError
	if !errors.As(err, &integration) || integration.Code != "invalid_gitflame_response" {
		return "", err
	}
	content, rawErr := c.getRaw(ctx, fmt.Sprintf("/api/v1/repositories/%s/raw/%s", url.PathEscape(repositoryID), url.PathEscape(filePath)), ref)
	if rawErr != nil {
		return "", rawErr
	}
	return content, nil
}

func (c *GitFlameClient) getJSON(ctx context.Context, endpoint, ref string, target any) error {
	body, err := c.doGET(ctx, endpoint, ref)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, target); err != nil {
		return &IntegrationError{Status: http.StatusBadGateway, Code: "invalid_gitflame_response", Detail: "GitFlame API returned invalid JSON"}
	}
	return nil
}

func (c *GitFlameClient) postJSON(ctx context.Context, endpoint string, payload, target any) error {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return err
	}
	requestURL := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &IntegrationError{Status: http.StatusBadGateway, Code: "gitflame_unreachable", Detail: "GitFlame API is unreachable"}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return gitFlameHTTPError(resp)
	}
	if target == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return &IntegrationError{Status: http.StatusBadGateway, Code: "invalid_gitflame_response", Detail: "GitFlame API returned invalid JSON"}
	}
	return nil
}

func (c *GitFlameClient) getRaw(ctx context.Context, endpoint, ref string) (string, error) {
	body, err := c.doGET(ctx, endpoint, ref)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *GitFlameClient) doGET(ctx context.Context, endpoint, ref string) ([]byte, error) {
	requestURL := c.baseURL + endpoint
	if ref != "" {
		requestURL += "?ref=" + url.QueryEscape(ref)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &IntegrationError{Status: http.StatusBadGateway, Code: "gitflame_unreachable", Detail: "GitFlame API is unreachable"}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &IntegrationError{Status: normalizeIntegrationStatus(resp.StatusCode), Code: "gitflame_api_error", Detail: fmt.Sprintf("GitFlame API returned status %d", resp.StatusCode)}
	}
	return body, nil
}

func gitFlameHTTPError(resp *http.Response) error {
	var problem struct {
		Detail  string `json:"detail"`
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 200_000)).Decode(&problem)
	detail := problem.Detail
	if detail == "" {
		detail = problem.Message
	}
	if detail == "" {
		detail = fmt.Sprintf("GitFlame API returned status %d", resp.StatusCode)
	}
	code := problem.Code
	if code == "" {
		code = "gitflame_api_error"
	}
	return &IntegrationError{Status: normalizeIntegrationStatus(resp.StatusCode), Code: code, Detail: detail}
}

func firstString(document map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := nestedValue(document, strings.Split(key, ".")); ok {
			switch typed := value.(type) {
			case string:
				return typed
			case float64:
				if typed == float64(int64(typed)) {
					return fmt.Sprintf("%d", int64(typed))
				}
				return fmt.Sprint(typed)
			default:
				if typed != nil {
					return fmt.Sprint(typed)
				}
			}
		}
	}
	return ""
}

func repositoryMetadataFromGitFlameResponse(response map[string]any, fallback domain.RepositoryMetadata) domain.RepositoryMetadata {
	metadata := fallback
	if value := firstString(response, "id", "repository.id", "data.id"); value != "" {
		metadata.ID = value
	}
	if value := firstString(response, "name", "path", "repository.name", "repository.path", "data.name", "data.path"); value != "" {
		metadata.Name = value
	}
	if value := firstString(response, "default_branch", "defaultBranch", "repository.default_branch", "data.default_branch"); value != "" {
		metadata.DefaultBranch = value
	}
	if value := firstString(response, "web_url", "html_url", "url", "repository.web_url", "data.web_url"); value != "" {
		metadata.WebURL = value
	}
	if value := firstString(response, "commit_sha", "sha", "default_branch_sha", "repository.commit_sha", "data.commit_sha"); value != "" {
		metadata.CommitSHA = value
	}
	if metadata.DefaultBranch == "" {
		metadata.DefaultBranch = "main"
	}
	return metadata
}

type parsedGitFlameRepositoryURL struct {
	Path   string
	WebURL string
}

func parseGitFlameRepositoryURL(rawURL string) (parsedGitFlameRepositoryURL, error) {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return parsedGitFlameRepositoryURL{}, errors.New("repo_url is required when repository.id is empty")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		trimmed := strings.Trim(value, "/")
		trimmed = strings.TrimSuffix(trimmed, ".git")
		trimmed = stripRepositoryViewSuffix(trimmed)
		if strings.Count(trimmed, "/") < 1 {
			return parsedGitFlameRepositoryURL{}, errors.New("repo_url must be a GitFlame repository URL or owner/repository path")
		}
		return parsedGitFlameRepositoryURL{Path: trimmed, WebURL: ""}, nil
	}
	repoPath := strings.Trim(parsed.EscapedPath(), "/")
	unescaped, err := url.PathUnescape(repoPath)
	if err != nil {
		return parsedGitFlameRepositoryURL{}, errors.New("repo_url contains invalid escaping")
	}
	unescaped = strings.TrimSuffix(unescaped, ".git")
	unescaped = stripRepositoryViewSuffix(unescaped)
	if strings.Count(unescaped, "/") < 1 {
		return parsedGitFlameRepositoryURL{}, errors.New("repo_url does not include owner and repository")
	}
	webURL := parsed.Scheme + "://" + parsed.Host + "/" + unescaped
	return parsedGitFlameRepositoryURL{Path: unescaped, WebURL: webURL}, nil
}

func stripRepositoryViewSuffix(repoPath string) string {
	parts := strings.Split(strings.Trim(repoPath, "/"), "/")
	for index, part := range parts {
		switch part {
		case "code", "issues", "pulls", "pull-requests", "merge_requests", "branches", "commits", "settings":
			if index >= 2 {
				return strings.Join(parts[:index], "/")
			}
		}
	}
	return strings.Join(parts, "/")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func nestedValue(value any, path []string) (any, bool) {
	if len(path) == 0 {
		return value, true
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	next, ok := object[path[0]]
	if !ok {
		return nil, false
	}
	return nestedValue(next, path[1:])
}

func matchesRepositoryRules(filePath string, include, exclude []string) bool {
	normalized := strings.TrimPrefix(path.Clean(strings.ReplaceAll(filePath, "\\", "/")), "./")
	if normalized == "." || strings.HasPrefix(normalized, "../") || strings.HasPrefix(normalized, "/") {
		return false
	}
	for _, pattern := range exclude {
		if matchRepositoryPattern(pattern, normalized) {
			return false
		}
	}
	for _, pattern := range include {
		if matchRepositoryPattern(pattern, normalized) {
			return true
		}
	}
	return false
}

func matchRepositoryPattern(pattern, value string) bool {
	pattern = strings.TrimPrefix(path.Clean(strings.ReplaceAll(pattern, "\\", "/")), "./")
	if pattern == "**/*" || pattern == "**" {
		return true
	}
	if strings.HasSuffix(pattern, "/**") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "/**")+"/")
	}
	if strings.HasPrefix(pattern, "**/") {
		suffix := strings.TrimPrefix(pattern, "**/")
		return strings.HasSuffix(value, suffix)
	}
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		return strings.HasPrefix(value, parts[0]) && strings.HasSuffix(value, parts[len(parts)-1])
	}
	matched, _ := filepath.Match(pattern, value)
	if matched {
		return true
	}
	return strings.TrimSuffix(pattern, "/") == value
}

func normalizeIntegrationStatus(status int) int {
	switch status {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return status
	default:
		if status >= 500 {
			return http.StatusBadGateway
		}
		return http.StatusUnprocessableEntity
	}
}
