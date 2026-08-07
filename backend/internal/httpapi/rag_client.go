package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/observability"
)

type RepositoryIndexer interface {
	Ready(context.Context) error
	Status(context.Context, string, string) (RAGIndexResult, error)
	Index(context.Context, RAGIndexRequest) (RAGIndexResult, error)
}

type RAGIndexRequest struct {
	RepositoryID      string                  `json:"repository_id"`
	RepositoryName    string                  `json:"repository_name,omitempty"`
	CommitSHA         string                  `json:"commit_sha"`
	Source            string                  `json:"source,omitempty"`
	ConfigurationYAML string                  `json:"configuration_yaml,omitempty"`
	Files             []domain.RepositoryFile `json:"files"`
	Force             bool                    `json:"force,omitempty"`
}

type RAGIndexResult struct {
	RepositoryID   string `json:"repository_id"`
	CommitSHA      string `json:"commit_sha"`
	Status         string `json:"status"`
	FileCount      int    `json:"file_count"`
	ChunkCount     int    `json:"chunk_count"`
	EmbeddingCount int    `json:"embedding_count"`
}

type RAGClient struct {
	baseURL, apiKey string
	httpClient      *http.Client
}

func NewRAGClient(baseURL, apiKey string, timeout time.Duration) *RAGClient {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	return &RAGClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *RAGClient) Ready(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	observability.PropagateRequestID(ctx, req)
	started := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		observability.ObserveUpstream("rag", "unreachable", started)
		return fmt.Errorf("CodeRAG is unreachable: %w", err)
	}
	defer resp.Body.Close()
	observability.ObserveUpstream("rag", observability.UpstreamOutcome(resp.StatusCode), started)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("CodeRAG health returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *RAGClient) Status(ctx context.Context, repositoryID, commitSHA string) (RAGIndexResult, error) {
	query := url.Values{}
	query.Set("repository_id", repositoryID)
	query.Set("commit_sha", commitSHA)
	var result RAGIndexResult
	err := c.request(ctx, http.MethodGet, "/indexes/status?"+query.Encode(), nil, &result)
	return result, err
}

func (c *RAGClient) Index(ctx context.Context, request RAGIndexRequest) (RAGIndexResult, error) {
	var result RAGIndexResult
	err := c.request(ctx, http.MethodPost, "/indexes", request, &result)
	return result, err
}

func (c *RAGClient) request(ctx context.Context, method, endpoint string, payload, target any) error {
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, &body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	observability.PropagateRequestID(ctx, req)
	started := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		observability.ObserveUpstream("rag", "unreachable", started)
		return &IntegrationError{Status: http.StatusServiceUnavailable, Code: "rag_unreachable", Detail: "CodeRAG service is unreachable"}
	}
	defer resp.Body.Close()
	observability.ObserveUpstream("rag", observability.UpstreamOutcome(resp.StatusCode), started)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var problem struct {
			Detail any `json:"detail"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&problem)
		detail := strings.TrimSpace(fmt.Sprint(problem.Detail))
		if detail == "" || detail == "<nil>" {
			detail = fmt.Sprintf("CodeRAG returned HTTP %d", resp.StatusCode)
		}
		return &IntegrationError{Status: resp.StatusCode, Code: "rag_indexing_failed", Detail: detail}
	}
	if target == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return &IntegrationError{Status: http.StatusBadGateway, Code: "invalid_rag_response", Detail: "CodeRAG returned invalid JSON"}
	}
	return nil
}
