package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"gitflame-codepilot/backend/internal/domain"
)

type GitFlameSource interface {
	BuildAnalyzeRequest(context.Context, GitFlameIssueWebhook) (domain.IssueAnalyzeRequest, error)
	ApplyGeneratedFiles(context.Context, domain.RepositoryMetadata, domain.GeneratedFilesContract) (domain.GitFlameApplyResult, error)
}

type GitFlameRepositoryReader interface {
	RepositoryTree(context.Context, string, string) ([]GitFlameTreeEntry, error)
	RepositoryIssues(context.Context, string) ([]domain.IssuePayload, error)
}

type GitFlameTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type RecommendationAnalyzer interface {
	AnalyzeRecommendations(context.Context, string, []domain.RepositoryFile) (string, []domain.RecommendationCard, error)
}

type IntegrationError struct {
	Status       int
	Code, Detail string
}

func (e *IntegrationError) Error() string { return e.Detail }

type GitFlameIssueWebhook struct {
	Event           string                    `json:"event"`
	Action          string                    `json:"action"`
	Repository      domain.RepositoryMetadata `json:"repository"`
	Issue           domain.IssuePayload       `json:"issue"`
	CommitSHA       string                    `json:"commit_sha"`
	Ref             string                    `json:"ref"`
	YAMLConfig      string                    `json:"yaml_config"`
	RepositoryFiles []domain.RepositoryFile   `json:"repository_files"`
	Metadata        map[string]any            `json:"metadata,omitempty"`
}

func (s *Server) gitflameIssueWebhook(w http.ResponseWriter, r *http.Request) {
	var req GitFlameIssueWebhook
	if err := decode(r, &req); err != nil {
		problem(w, 400, "invalid_json", err.Error())
		return
	}
	analyzeReq, err := s.analyzeRequestFromWebhook(r.Context(), req)
	if err != nil {
		integrationError(w, err, "gitflame_integration_error")
		return
	}
	session, task, err := s.workflow.Analyze(analyzeReq)
	if err != nil {
		workflowError(w, err)
		return
	}
	write(w, http.StatusAccepted, analyzeResponse{session.ID, task.ID, analyzeReq.Issue.ID, analyzeReq.Repository.ID, task.Status, "/ai/tasks/" + task.ID})
}

func (s *Server) analyzeRequestFromWebhook(ctx context.Context, req GitFlameIssueWebhook) (domain.IssueAnalyzeRequest, error) {
	if strings.TrimSpace(req.Repository.CommitSHA) == "" {
		req.Repository.CommitSHA = req.CommitSHA
	}
	if req.YAMLConfig != "" && len(req.RepositoryFiles) > 0 {
		return domain.IssueAnalyzeRequest{
			Repository:      req.Repository,
			Issue:           req.Issue,
			YAMLConfig:      req.YAMLConfig,
			RepositoryFiles: req.RepositoryFiles,
			Metadata:        req.Metadata,
		}, nil
	}
	if s.gitflame == nil {
		return domain.IssueAnalyzeRequest{}, &IntegrationError{Status: http.StatusServiceUnavailable, Code: "gitflame_client_unavailable", Detail: "GitFlame API client is not configured and webhook payload did not include yaml_config plus repository_files"}
	}
	analyzeReq, err := s.gitflame.BuildAnalyzeRequest(ctx, req)
	if err != nil {
		return domain.IssueAnalyzeRequest{}, err
	}
	if analyzeReq.Metadata == nil {
		analyzeReq.Metadata = req.Metadata
	}
	return analyzeReq, nil
}

func integrationError(w http.ResponseWriter, err error, fallbackCode string) {
	var integration *IntegrationError
	if errors.As(err, &integration) {
		status := integration.Status
		if status == 0 {
			status = http.StatusBadGateway
		}
		code := integration.Code
		if code == "" {
			code = fallbackCode
		}
		problem(w, status, code, integration.Detail)
		return
	}
	problem(w, http.StatusBadGateway, fallbackCode, err.Error())
}
