package httpapi

import (
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
