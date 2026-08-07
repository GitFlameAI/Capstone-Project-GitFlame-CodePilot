package httpapi

import (
	"context"
	"errors"
	"net/http"

	"gitflame-codepilot/backend/internal/domain"
)

type GitFlameSource interface {
	ApplyGeneratedFiles(context.Context, domain.RepositoryMetadata, domain.GeneratedFilesContract) (domain.GitFlameApplyResult, error)
}

type GitFlameRepositoryReader interface {
	RepositoryTree(context.Context, string, string) ([]GitFlameTreeEntry, error)
	RepositoryFiles(context.Context, string, string, string, []domain.RepositoryFile) (string, []domain.RepositoryFile, error)
	RepositoryIssues(context.Context, string) ([]domain.IssuePayload, error)
}

type GitFlameTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type RecommendationAnalyzer interface {
	AnalyzeRecommendations(context.Context, domain.RepositoryMetadata, string, []domain.RepositoryFile) (string, []domain.RecommendationCard, error)
}

type IntegrationError struct {
	Status       int
	Code, Detail string
}

func (e *IntegrationError) Error() string { return e.Detail }

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
