package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/repository"
)

type RecommendationClient struct {
	baseURL    string
	httpClient *http.Client
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

func (c *RecommendationClient) AnalyzeRecommendations(ctx context.Context, configYAML string, files []domain.RepositoryFile) (string, []domain.RecommendationCard, error) {
	payload := struct {
		ConfigYAML  string                  `json:"config_yaml"`
		RepoContext []domain.RepositoryFile `json:"repo_context"`
	}{ConfigYAML: configYAML, RepoContext: files}
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/recommendations/analyze", &body)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, &IntegrationError{Status: http.StatusBadGateway, Code: "recommendation_service_unreachable", Detail: "recommendation service is unreachable"}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var problem struct {
			Detail string `json:"detail"`
			Code   string `json:"code"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&problem)
		if problem.Detail == "" {
			problem.Detail = fmt.Sprintf("recommendation service returned status %d", resp.StatusCode)
		}
		if problem.Code == "" {
			problem.Code = "recommendation_service_error"
		}
		return "", nil, &IntegrationError{Status: normalizeIntegrationStatus(resp.StatusCode), Code: problem.Code, Detail: problem.Detail}
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
