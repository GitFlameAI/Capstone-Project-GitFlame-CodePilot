package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/observability"
	"gitflame-codepilot/backend/internal/repository"
)

type RecommendationClient struct {
	baseURL    string
	httpClient *http.Client
}

type recommendationRepositoryReference struct {
	ID        string `json:"id"`
	CommitSHA string `json:"commit_sha"`
}

type recommendationRepoFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type recommendationAnalyzePayload struct {
	Repository  recommendationRepositoryReference `json:"repository"`
	ConfigYAML  string                            `json:"config_yaml"`
	RepoContext []recommendationRepoFile          `json:"repo_context"`
}

func NewRecommendationClient(baseURL string, timeout time.Duration) *RecommendationClient {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &RecommendationClient{baseURL: strings.TrimRight(baseURL, "/"), httpClient: &http.Client{Timeout: timeout}}
}

func (c *RecommendationClient) AnalyzeRecommendations(ctx context.Context, repositoryMetadata domain.RepositoryMetadata, configYAML string, files []domain.RepositoryFile) (string, []domain.RecommendationCard, error) {
	repoContext := make([]recommendationRepoFile, 0, len(files))
	for _, file := range files {
		repoContext = append(repoContext, recommendationRepoFile{
			Path:    file.Path,
			Content: file.Content,
		})
	}
	payload := recommendationAnalyzePayload{
		Repository: recommendationRepositoryReference{
			ID:        repositoryMetadata.ID,
			CommitSHA: repositoryMetadata.CommitSHA,
		},
		ConfigYAML:  configYAML,
		RepoContext: repoContext,
	}
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/recommendations/analyze", &body)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	observability.PropagateRequestID(ctx, req)
	started := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		observability.ObserveUpstream("recommendation_service", "unreachable", started)
		return "", nil, &IntegrationError{Status: http.StatusBadGateway, Code: "recommendation_service_unreachable", Detail: "recommendation service is unreachable"}
	}
	defer resp.Body.Close()
	observability.ObserveUpstream("recommendation_service", observability.UpstreamOutcome(resp.StatusCode), started)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		code, detail := recommendationProblem(responseBody)
		if detail == "" {
			detail = fmt.Sprintf("recommendation service returned status %d", resp.StatusCode)
		}
		if code == "" {
			code = "recommendation_service_error"
		}
		return "", nil, &IntegrationError{Status: normalizeIntegrationStatus(resp.StatusCode), Code: code, Detail: detail}
	}
	var result struct {
		Summary         string                      `json:"summary"`
		Recommendations []domain.RecommendationCard `json:"recommendations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", nil, &IntegrationError{Status: http.StatusBadGateway, Code: "invalid_recommendation_response", Detail: "recommendation service returned invalid JSON"}
	}
	if strings.TrimSpace(result.Summary) == "" {
		return "", nil, &IntegrationError{Status: http.StatusBadGateway, Code: "invalid_recommendation_response", Detail: "recommendation service returned an empty summary"}
	}
	result.Recommendations = repository.NormalizeRecommendations(result.Recommendations)
	for index := range result.Recommendations {
		if result.Recommendations[index].ID == "" {
			result.Recommendations[index].ID = repository.NewID()
		}
		if result.Recommendations[index].State == "" {
			result.Recommendations[index].State = "open"
		}
	}
	return result.Summary, result.Recommendations, nil
}

func recommendationProblem(body []byte) (string, string) {
	var problem struct {
		Code   string          `json:"code"`
		Detail json.RawMessage `json:"detail"`
	}
	if json.Unmarshal(body, &problem) != nil || len(problem.Detail) == 0 {
		return "", ""
	}
	var detail string
	if json.Unmarshal(problem.Detail, &detail) == nil {
		return problem.Code, detail
	}
	var validationErrors []struct {
		Location []any  `json:"loc"`
		Message  string `json:"msg"`
	}
	if json.Unmarshal(problem.Detail, &validationErrors) != nil {
		return problem.Code, ""
	}
	messages := make([]string, 0, len(validationErrors))
	for _, validationError := range validationErrors {
		location := make([]string, 0, len(validationError.Location))
		for _, segment := range validationError.Location {
			value := fmt.Sprint(segment)
			if value != "body" {
				location = append(location, value)
			}
		}
		message := strings.TrimSpace(validationError.Message)
		if len(location) > 0 {
			message = strings.Join(location, ".") + ": " + message
		}
		if message != "" {
			messages = append(messages, message)
		}
	}
	return problem.Code, strings.Join(messages, "; ")
}
