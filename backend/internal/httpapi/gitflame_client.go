package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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
	yamlConfig, files, err := c.RepositoryFiles(ctx, webhook.Repository.ID, ref, webhook.YAMLConfig, webhook.RepositoryFiles)
	if err != nil {
		return domain.IssueAnalyzeRequest{}, err
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
	endpoints := make([]string, 0, 4)
	if endpoint, ok := giteaRepositoryEndpoint(parsed.Path); ok {
		endpoints = append(endpoints, endpoint)
	}
	endpoints = append(endpoints,
		"/api/v1/repositories/"+url.PathEscape(parsed.Path),
		"/api/v1/projects/"+url.PathEscape(parsed.Path),
		"/api/v1/repos/"+url.PathEscape(parsed.Path),
	)
	var lastErr error
	for _, endpoint := range endpoints {
		var response map[string]any
		if err := c.getJSON(ctx, endpoint, "", &response); err != nil {
			lastErr = err
			var integration *IntegrationError
			if !errors.As(err, &integration) || integration.Status != http.StatusNotFound {
				return resolvedGitFlameRepository{}, err
			}
			continue
		}
		metadata := repositoryMetadataFromGitFlameResponse(response, fallback.Metadata)
		return resolvedGitFlameRepository{Metadata: metadata, RepoURL: firstNonEmpty(metadata.WebURL, fallback.RepoURL)}, nil
	}
	if lastErr != nil {
		return resolvedGitFlameRepository{}, lastErr
	}
	return resolvedGitFlameRepository{}, &IntegrationError{Status: http.StatusNotFound, Code: "gitflame_repository_not_found", Detail: "GitFlame repository was not found"}
}

func (c *GitFlameClient) createBranch(ctx context.Context, repositoryID, branchName, ref string) error {
	payload := map[string]string{
		"new_branch_name": branchName,
		"old_ref_name":    ref,
	}
	err := c.postFirstAvailable(ctx, applyPOSTCandidates(repositoryID, "branches", "branches", payload), nil)
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
	if err := c.postFirstAvailable(ctx, applyPOSTCandidates(repositoryID, "commits", "commits", payload), &response); err != nil {
		var integration *IntegrationError
		if !errors.As(err, &integration) || (integration.Status != http.StatusNotFound && integration.Status != http.StatusMethodNotAllowed) {
			return "", err
		}
		return c.commitGeneratedFilesViaContents(ctx, repositoryID, contract)
	}
	return firstString(response, "sha", "commit_sha", "id", "commit.id", "commit.sha"), nil
}

func (c *GitFlameClient) commitGeneratedFilesViaContents(ctx context.Context, repositoryID string, contract domain.GeneratedFilesContract) (string, error) {
	repositoryEndpoint, ok := giteaRepositoryEndpoint(repositoryID)
	if !ok {
		return "", &IntegrationError{Status: http.StatusUnprocessableEntity, Code: "invalid_repository_id", Detail: "repository id must use owner/repository format"}
	}
	commitSHA := ""
	for index, file := range contract.Files {
		message := contract.CommitMessage
		if len(contract.Files) > 1 {
			message = fmt.Sprintf("%s (%d/%d: %s)", contract.CommitMessage, index+1, len(contract.Files), file.Path)
		}
		sha, err := c.applyGeneratedFileViaContents(ctx, repositoryEndpoint, contract.BranchName, message, file)
		if err != nil {
			return "", err
		}
		if sha != "" {
			commitSHA = sha
		}
	}
	return commitSHA, nil
}

func (c *GitFlameClient) applyGeneratedFileViaContents(ctx context.Context, repositoryEndpoint, branch, message string, file domain.GeneratedFileOperation) (string, error) {
	endpoint := repositoryEndpoint + "/contents/" + escapeRepositoryPath(file.Path)
	var response map[string]any
	switch file.Action {
	case "create":
		if err := c.createGeneratedFileViaContents(ctx, endpoint, branch, message, file.Content, &response); err != nil {
			return "", generatedFileApplyError(file.Path, err)
		}
	case "modify":
		shas, err := c.fetchFileSHACandidates(ctx, endpoint, branch)
		if err != nil {
			return "", generatedFileApplyError(file.Path, err)
		}
		var lastErr error
		for _, sha := range shas {
			for _, payload := range contentsWritePayloads(branch, message, file.Content, sha) {
				if err := c.requestJSON(ctx, http.MethodPut, endpoint, payload, &response); err != nil {
					lastErr = err
					if integrationStatus(err) == http.StatusConflict {
						break
					}
					if retryContentsPayload(err) {
						continue
					}
					return "", generatedFileApplyError(file.Path, err)
				}
				lastErr = nil
				break
			}
			if lastErr == nil {
				break
			}
		}
		if lastErr != nil {
			return "", generatedFileApplyError(file.Path, lastErr)
		}
	case "delete":
		shas, err := c.fetchFileSHACandidates(ctx, endpoint, branch)
		if err != nil {
			return "", generatedFileApplyError(file.Path, err)
		}
		var lastErr error
		for _, sha := range shas {
			payload := map[string]any{
				"branch":         branch,
				"message":        message,
				"commit_message": message,
				"sha":            sha,
			}
			if err := c.requestJSON(ctx, http.MethodDelete, endpoint, payload, &response); err != nil {
				lastErr = err
				if integrationStatus(err) == http.StatusConflict {
					continue
				}
				return "", generatedFileApplyError(file.Path, err)
			}
			lastErr = nil
			break
		}
		if lastErr != nil {
			return "", generatedFileApplyError(file.Path, lastErr)
		}
	default:
		return "", &IntegrationError{Status: http.StatusUnprocessableEntity, Code: "invalid_generated_file_action", Detail: "generated file action is not supported by GitFlame apply"}
	}
	return firstString(response, "commit.sha", "commit.id", "sha", "commit_sha", "id"), nil
}

func (c *GitFlameClient) createGeneratedFileViaContents(ctx context.Context, endpoint, branch, message, content string, response *map[string]any) error {
	var lastErr error
	for _, payload := range contentsWritePayloads(branch, message, content, "") {
		if err := c.postJSON(ctx, endpoint, payload, response); err != nil {
			lastErr = err
			if !retryContentsPayload(err) {
				return err
			}
			continue
		}
		return nil
	}
	for _, payload := range contentsWritePayloads(branch, message, content, "") {
		if err := c.requestJSON(ctx, http.MethodPut, endpoint, payload, response); err != nil {
			lastErr = err
			if !retryContentsPayload(err) {
				return err
			}
			continue
		}
		return nil
	}
	return lastErr
}

func contentsWritePayloads(branch, message, content, sha string) []map[string]any {
	base := map[string]any{
		"branch":         branch,
		"message":        message,
		"commit_message": message,
	}
	if sha != "" {
		base["sha"] = sha
	}
	encoded := clonePayload(base)
	encoded["content"] = base64.StdEncoding.EncodeToString([]byte(content))
	raw := clonePayload(base)
	raw["content"] = content
	encodedWithBranchName := clonePayload(encoded)
	encodedWithBranchName["branch_name"] = branch
	rawWithBranchName := clonePayload(raw)
	rawWithBranchName["branch_name"] = branch
	return []map[string]any{encoded, raw, encodedWithBranchName, rawWithBranchName}
}

func clonePayload(payload map[string]any) map[string]any {
	cloned := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		cloned[key] = value
	}
	return cloned
}

func retryContentsPayload(err error) bool {
	status := integrationStatus(err)
	return status == http.StatusBadRequest || status == http.StatusUnprocessableEntity
}

func generatedFileApplyError(filePath string, err error) error {
	var integration *IntegrationError
	if !errors.As(err, &integration) {
		return fmt.Errorf("%s: %w", filePath, err)
	}
	clone := *integration
	if clone.Detail == "" {
		clone.Detail = filePath
	} else if !strings.Contains(clone.Detail, filePath) {
		clone.Detail = filePath + ": " + clone.Detail
	}
	return &clone
}

func integrationStatus(err error) int {
	var integration *IntegrationError
	if errors.As(err, &integration) {
		return integration.Status
	}
	return 0
}

func (c *GitFlameClient) fetchFileSHACandidates(ctx context.Context, endpoint, ref string) ([]string, error) {
	var response map[string]any
	if err := c.getJSON(ctx, endpoint, ref, &response); err != nil {
		return nil, err
	}
	shas := uniqueNonEmptyStrings(
		firstString(response, "sha"),
		firstString(response, "content.sha"),
		firstString(response, "file.sha"),
		firstString(response, "data.sha"),
		firstString(response, "data.content.sha"),
		firstString(response, "data.file.sha"),
		firstString(response, "last_commit_sha"),
		firstString(response, "commit.sha"),
	)
	if len(shas) == 0 {
		return nil, &IntegrationError{Status: http.StatusBadGateway, Code: "invalid_gitflame_response", Detail: "GitFlame API did not return file sha"}
	}
	return shas, nil
}

func uniqueNonEmptyStrings(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
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
	if err := c.postFirstAvailable(ctx, applyPOSTCandidates(repositoryID, "pulls", "pull-requests", payload), &response); err != nil {
		return "", "", err
	}
	id := firstString(response, "id", "number", "iid", "pull_request.id")
	prURL := firstString(response, "pull_request_url", "html_url", "web_url", "url", "pull_request.url", "pull_request.html_url")
	return id, prURL, nil
}

func applyPOSTCandidates(repositoryID, giteaResource, fallbackResource string, payload any) []gitFlamePOSTCandidate {
	candidates := make([]gitFlamePOSTCandidate, 0, 3)
	if endpoint, ok := giteaRepositoryEndpoint(repositoryID); ok {
		candidates = append(candidates, gitFlamePOSTCandidate{Endpoint: endpoint + "/" + giteaResource, Payload: payload})
		if fallbackResource != giteaResource {
			candidates = append(candidates, gitFlamePOSTCandidate{Endpoint: endpoint + "/" + fallbackResource, Payload: payload})
		}
	}
	candidates = append(candidates, gitFlamePOSTCandidate{
		Endpoint: fmt.Sprintf("/api/v1/repositories/%s/%s", url.PathEscape(repositoryID), fallbackResource),
		Payload:  payload,
	})
	return candidates
}

func (c *GitFlameClient) fetchTree(ctx context.Context, repositoryID, ref string) ([]GitFlameTreeEntry, error) {
	return c.RepositoryTree(ctx, repositoryID, ref)
}

func (c *GitFlameClient) RepositoryFiles(ctx context.Context, repositoryID, ref, yamlConfig string, requested []domain.RepositoryFile) (string, []domain.RepositoryFile, error) {
	if strings.TrimSpace(yamlConfig) == "" {
		content, err := c.fetchFileContent(ctx, repositoryID, ".ai.yml", ref)
		if err != nil {
			return "", nil, err
		}
		yamlConfig = content
	}
	cfg, err := service.ParseAIConfig(yamlConfig)
	if err != nil {
		return "", nil, &IntegrationError{Status: http.StatusUnprocessableEntity, Code: "invalid_ai_config", Detail: err.Error()}
	}
	if len(requested) > 0 {
		files := make([]domain.RepositoryFile, 0, min(len(requested), cfg.MaxFiles))
		for _, file := range requested {
			if len(files) >= cfg.MaxFiles {
				break
			}
			if !repositoryFileIsReadable(file.Type) {
				continue
			}
			file.Path = normalizeRepositoryPath(file.Path)
			if file.Path == "" || !matchesRepositoryRules(file.Path, cfg.IncludePatterns, cfg.ExcludePatterns) {
				continue
			}
			if strings.TrimSpace(file.Content) == "" {
				content, err := c.fetchFileContent(ctx, repositoryID, file.Path, ref)
				if err != nil {
					return "", nil, err
				}
				file.Content = content
			}
			file.Type = ""
			files = append(files, file)
		}
		return yamlConfig, files, nil
	}

	tree, err := c.fetchTree(ctx, repositoryID, ref)
	if err != nil {
		return "", nil, err
	}
	files := make([]domain.RepositoryFile, 0, min(len(tree), cfg.MaxFiles))
	for _, entry := range tree {
		if len(files) >= cfg.MaxFiles {
			break
		}
		if entry.Type != "" && entry.Type != "file" && entry.Type != "blob" {
			continue
		}
		filePath := normalizeRepositoryPath(entry.Path)
		if !matchesRepositoryRules(filePath, cfg.IncludePatterns, cfg.ExcludePatterns) {
			continue
		}
		content, err := c.fetchFileContent(ctx, repositoryID, filePath, ref)
		if err != nil {
			return "", nil, err
		}
		files = append(files, domain.RepositoryFile{Path: filePath, Content: content})
	}
	return yamlConfig, files, nil
}

func repositoryFileIsReadable(fileType string) bool {
	switch strings.ToLower(strings.TrimSpace(fileType)) {
	case "", "file", "blob":
		return true
	default:
		return false
	}
}

func (c *GitFlameClient) RepositoryTree(ctx context.Context, repositoryID, ref string) ([]GitFlameTreeEntry, error) {
	repositoryEndpoint, ok := giteaRepositoryEndpoint(repositoryID)
	if !ok {
		return nil, &IntegrationError{Status: http.StatusUnprocessableEntity, Code: "invalid_repository_id", Detail: "repository id must use owner/repository format"}
	}

	type contentEntry struct {
		Path  string `json:"path"`
		Name  string `json:"name"`
		Title string `json:"title"`
		Type  string `json:"type"`
	}

	const maxTreeEntries = 10_000
	directories := []string{""}
	visitedDirectories := map[string]struct{}{"": {}}
	seenEntries := make(map[string]struct{})
	tree := make([]GitFlameTreeEntry, 0)

	for len(directories) > 0 {
		directory := directories[0]
		directories = directories[1:]
		endpoint := repositoryEndpoint + "/contents"
		if directory != "" {
			endpoint += "/" + escapeRepositoryPath(directory)
		}

		body, err := c.doGET(ctx, endpoint, ref)
		if err != nil {
			return nil, err
		}
		var contents []contentEntry
		if err := decodeGitFlameCollection(body, []string{"contents", "items", "data"}, &contents); err != nil {
			return nil, err
		}

		for _, item := range contents {
			entryPath := strings.Trim(strings.TrimSpace(firstNonEmptyString(item.Path, item.Name, item.Title)), "/")
			if entryPath == "" {
				continue
			}
			if directory != "" && !strings.Contains(entryPath, "/") {
				entryPath = path.Join(directory, entryPath)
			}
			if _, exists := seenEntries[entryPath]; exists {
				continue
			}

			entryType := strings.ToLower(strings.TrimSpace(item.Type))
			switch entryType {
			case "dir", "tree", "directory", "folder":
				entryType = "dir"
				if _, visited := visitedDirectories[entryPath]; !visited {
					visitedDirectories[entryPath] = struct{}{}
					directories = append(directories, entryPath)
				}
			default:
				entryType = "file"
			}

			seenEntries[entryPath] = struct{}{}
			tree = append(tree, GitFlameTreeEntry{Path: entryPath, Type: entryType})
			if len(tree) > maxTreeEntries {
				return nil, &IntegrationError{Status: http.StatusBadGateway, Code: "gitflame_tree_too_large", Detail: "GitFlame repository tree exceeds 10000 entries"}
			}
		}
	}

	return tree, nil
}

func (c *GitFlameClient) RepositoryIssues(ctx context.Context, repositoryID string) ([]domain.IssuePayload, error) {
	candidates := make([]gitFlameGETCandidate, 0, 2)
	if endpoint, ok := giteaRepositoryEndpoint(repositoryID); ok {
		candidates = append(candidates, gitFlameGETCandidate{Endpoint: endpoint + "/issues?state=open&type=issues&limit=100"})
	}
	candidates = append(candidates, gitFlameGETCandidate{
		Endpoint: fmt.Sprintf("/api/v1/repositories/%s/issues?state=open", url.PathEscape(repositoryID)),
	})
	body, err := c.getFirstAvailable(ctx, candidates)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID          any             `json:"id"`
		IID         any             `json:"iid"`
		Number      any             `json:"number"`
		Title       string          `json:"title"`
		Body        string          `json:"body"`
		Description string          `json:"description"`
		Author      json.RawMessage `json:"author"`
		User        json.RawMessage `json:"user"`
	}
	if err := decodeGitFlameCollection(body, []string{"issues", "items", "data"}, &raw); err != nil {
		return nil, err
	}
	issues := make([]domain.IssuePayload, 0, len(raw))
	for _, item := range raw {
		id := firstNonEmptyValue(item.IID, item.Number, item.ID)
		body := item.Body
		if body == "" {
			body = item.Description
		}
		author := gitFlameIssueAuthor(item.Author)
		if author == "" {
			author = gitFlameIssueAuthor(item.User)
		}
		issues = append(issues, domain.IssuePayload{ID: id, Title: item.Title, Body: body, Author: author})
	}
	return issues, nil
}

type gitFlameGETCandidate struct {
	Endpoint string
	Ref      string
}

type gitFlamePOSTCandidate struct {
	Endpoint string
	Payload  any
}

func (c *GitFlameClient) getFirstAvailable(ctx context.Context, candidates []gitFlameGETCandidate) ([]byte, error) {
	var lastErr error
	for _, candidate := range candidates {
		body, err := c.doGET(ctx, candidate.Endpoint, candidate.Ref)
		if err == nil {
			return body, nil
		}
		lastErr = err
		var integration *IntegrationError
		if !errors.As(err, &integration) || integration.Status != http.StatusNotFound {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *GitFlameClient) postFirstAvailable(ctx context.Context, candidates []gitFlamePOSTCandidate, target any) error {
	var lastErr error
	for _, candidate := range candidates {
		err := c.postJSON(ctx, candidate.Endpoint, candidate.Payload, target)
		if err == nil {
			return nil
		}
		lastErr = err
		var integration *IntegrationError
		if !errors.As(err, &integration) || (integration.Status != http.StatusNotFound && integration.Status != http.StatusMethodNotAllowed) {
			return err
		}
	}
	return lastErr
}

func giteaRepositoryEndpoint(repositoryID string) (string, bool) {
	parts := strings.SplitN(strings.Trim(strings.TrimSpace(repositoryID), "/"), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return "/api/v1/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]), true
}

func escapeRepositoryPath(filePath string) string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(filePath), "/"), "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func decodeGitFlameCollection(body []byte, keys []string, target any) error {
	if err := json.Unmarshal(body, target); err == nil {
		return nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return &IntegrationError{Status: http.StatusBadGateway, Code: "invalid_gitflame_response", Detail: "GitFlame API returned invalid JSON"}
	}
	for _, key := range keys {
		if raw, ok := envelope[key]; ok {
			if err := json.Unmarshal(raw, target); err == nil {
				return nil
			}
			var nested map[string]json.RawMessage
			if json.Unmarshal(raw, &nested) == nil {
				for _, nestedKey := range keys {
					if collection, ok := nested[nestedKey]; ok && json.Unmarshal(collection, target) == nil {
						return nil
					}
				}
			}
		}
	}
	return &IntegrationError{Status: http.StatusBadGateway, Code: "invalid_gitflame_response", Detail: "GitFlame API returned an unexpected collection format"}
}

func gitFlameIssueAuthor(raw json.RawMessage) string {
	var author string
	if json.Unmarshal(raw, &author) == nil {
		return author
	}
	var profile struct {
		Username string `json:"username"`
		Login    string `json:"login"`
		Name     string `json:"name"`
	}
	if json.Unmarshal(raw, &profile) != nil {
		return ""
	}
	return firstNonEmptyString(profile.Username, profile.Login, profile.Name)
}

func firstNonEmptyValue(values ...any) string {
	for _, value := range values {
		if value == nil {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (c *GitFlameClient) fetchFileContent(ctx context.Context, repositoryID, filePath, ref string) (string, error) {
	repositoryEndpoint, ok := giteaRepositoryEndpoint(repositoryID)
	if !ok {
		return "", &IntegrationError{Status: http.StatusUnprocessableEntity, Code: "invalid_repository_id", Detail: "repository id must use owner/repository format"}
	}
	return c.getRaw(ctx, repositoryEndpoint+"/raw/"+escapeRepositoryPath(filePath), gitFlameRawRef(ref))
}

func gitFlameRawRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "refs/") {
		return ref
	}
	return "refs/heads/" + ref
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
	return c.requestJSON(ctx, http.MethodPost, endpoint, payload, target)
}

func (c *GitFlameClient) requestJSON(ctx context.Context, method, endpoint string, payload, target any) error {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return err
	}
	requestURL := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, method, requestURL, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("gitflame_http method=%s path=%s error=%q", method, req.URL.EscapedPath(), err)
		return &IntegrationError{Status: http.StatusBadGateway, Code: "gitflame_unreachable", Detail: "GitFlame API is unreachable"}
	}
	defer resp.Body.Close()
	log.Printf("gitflame_http method=%s path=%s status=%d", method, req.URL.EscapedPath(), resp.StatusCode)
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
		separator := "?"
		if strings.Contains(requestURL, "?") {
			separator = "&"
		}
		requestURL += separator + "ref=" + url.QueryEscape(ref)
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
		log.Printf("gitflame_http method=GET path=%s error=%q", req.URL.EscapedPath(), err)
		return nil, &IntegrationError{Status: http.StatusBadGateway, Code: "gitflame_unreachable", Detail: "GitFlame API is unreachable"}
	}
	defer resp.Body.Close()
	log.Printf("gitflame_http method=GET path=%s status=%d", req.URL.EscapedPath(), resp.StatusCode)
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &IntegrationError{Status: normalizeIntegrationStatus(resp.StatusCode), Code: "gitflame_api_error", Detail: fmt.Sprintf("GitFlame API returned status %d", resp.StatusCode)}
	}
	return body, nil
}

func gitFlameHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 200_000))
	var problem struct {
		Detail  any    `json:"detail"`
		Message any    `json:"message"`
		Error   any    `json:"error"`
		Code    string `json:"code"`
	}
	_ = json.Unmarshal(body, &problem)
	detail := gitFlameProblemText(problem.Detail)
	if detail == "" {
		detail = gitFlameProblemText(problem.Message)
	}
	if detail == "" {
		detail = gitFlameProblemText(problem.Error)
	}
	if detail == "" {
		var raw any
		if json.Unmarshal(body, &raw) == nil {
			detail = gitFlameProblemText(raw)
		}
	}
	if detail == "" {
		detail = strings.TrimSpace(string(body))
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

func gitFlameProblemText(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			text := gitFlameProblemText(item)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		for _, key := range []string{"detail", "message", "error", "path"} {
			if text := gitFlameProblemText(typed[key]); text != "" {
				return text
			}
		}
	}
	return ""
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
	if value := firstString(response, "full_name", "fullName", "repository.full_name", "data.full_name", "id", "repository.id", "data.id"); value != "" {
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
	normalized := normalizeRepositoryPath(filePath)
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

func normalizeRepositoryPath(filePath string) string {
	normalized := strings.TrimPrefix(path.Clean(strings.ReplaceAll(strings.TrimSpace(filePath), "\\", "/")), "./")
	if normalized == "." {
		return ""
	}
	return normalized
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
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return status
	default:
		if status >= 500 {
			return http.StatusBadGateway
		}
		return http.StatusUnprocessableEntity
	}
}
