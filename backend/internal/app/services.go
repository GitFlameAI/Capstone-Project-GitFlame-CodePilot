package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type AIConfig struct {
	Raw                string
	TargetBranchPrefix string
	ApproveCommand     string
	CorrectCommand     string
	RejectCommand      string
}

func ParseAIConfig(raw string) (AIConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return AIConfig{}, errors.New("missing .yml configuration")
	}
	if hasDisabledAnalysis(raw) {
		return AIConfig{}, errors.New("repository analysis is disabled in .yml configuration")
	}
	return AIConfig{
		Raw:                raw,
		TargetBranchPrefix: valueAfterKey(raw, "target_branch_prefix", "ai/"),
		ApproveCommand:     valueAfterKey(raw, "approve_command", "/approve"),
		CorrectCommand:     valueAfterKey(raw, "correct_command", "/correct"),
		RejectCommand:      valueAfterKey(raw, "reject_command", "/reject"),
	}, nil
}

func hasDisabledAnalysis(raw string) bool {
	inAnalysis := false
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inAnalysis = strings.HasPrefix(trimmed, "analysis:")
			continue
		}
		if inAnalysis && strings.HasPrefix(trimmed, "enabled:") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "enabled:"))
			return strings.EqualFold(value, "false")
		}
	}
	return false
}

func valueAfterKey(raw, key, fallback string) string {
	prefix := key + ":"
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		value = strings.Trim(value, `"'`)
		if value != "" {
			return value
		}
	}
	return fallback
}

type MLClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewMLClient(baseURL string) *MLClient {
	return &MLClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 4 * time.Second,
		},
	}
}

func (c *MLClient) GenerateIssuePlan(ctx context.Context, payload IssueAnalyzeRequest) (string, error) {
	body := map[string]any{
		"issue_title":        payload.Issue.Title,
		"issue_body":         payload.Issue.Body,
		"yaml_config":        payload.YAMLConfig,
		"repository_context": payload.RepositoryContext,
	}
	var response struct {
		PlanMarkdown string `json:"plan_markdown"`
	}
	if err := c.postJSON(ctx, "/issue-plan", body, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.PlanMarkdown) == "" {
		return "", errors.New("ML service returned an empty plan")
	}
	return response.PlanMarkdown, nil
}

func (c *MLClient) GenerateRecommendations(ctx context.Context, yamlConfig string, repositoryContext []string) (string, []RecommendationCard, error) {
	body := map[string]any{
		"yaml_config":        yamlConfig,
		"repository_context": repositoryContext,
	}
	var response struct {
		Summary         string               `json:"summary"`
		Recommendations []RecommendationCard `json:"recommendations"`
	}
	if err := c.postJSON(ctx, "/recommendations", body, &response); err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(response.Summary) == "" {
		return "", nil, errors.New("ML service returned an empty summary")
	}
	for i := range response.Recommendations {
		if response.Recommendations[i].ID == "" {
			response.Recommendations[i].ID = newID()
		}
		if response.Recommendations[i].State == "" {
			response.Recommendations[i].State = recommendationOpen
		}
	}
	return response.Summary, response.Recommendations, nil
}

func (c *MLClient) postJSON(ctx context.Context, path string, payload any, target any) error {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ML service returned status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func fallbackIssuePlan(payload IssueAnalyzeRequest) string {
	return fmt.Sprintf(`# Implementation Plan

## Issue
%s

## Steps
1. Validate the repository .yml configuration and branch rules.
2. Review the issue body and repository context supplied by GitFlame.
3. Identify files likely affected by the requested change.
4. Implement the change in an AI-generated branch after user approval.
5. Create a pull request and assign the issue author as reviewer.
`, payload.Issue.Title)
}

func fallbackRecommendations() (string, []RecommendationCard) {
	confidence := 0.72
	return "Sprint 1 mock analysis completed. No critical issues were detected.", []RecommendationCard{
		{
			ID:         newID(),
			Severity:   "low",
			File:       "README.md",
			Problem:    "Project setup documentation is still minimal.",
			Suggestion: "Add run instructions and API documentation links after endpoints are merged.",
			Confidence: &confidence,
			State:      recommendationOpen,
		},
	}
}

func createGitWorkflow(request IssueAnalyzeRequest, branchPrefix string) GitWorkflowResponse {
	slug := regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(request.Issue.Title, "-")
	slug = strings.Trim(strings.ToLower(slug), "-")
	if len(slug) > 48 {
		slug = slug[:48]
	}
	if slug == "" {
		slug = strings.ToLower(request.Issue.ID)
	}
	baseURL := request.Repository.WebURL
	if baseURL == "" {
		baseURL = "https://gitflame.local/" + request.Repository.ID
	}
	return GitWorkflowResponse{
		BranchName:     branchPrefix + request.Issue.ID + "-" + slug,
		PullRequestURL: strings.TrimRight(baseURL, "/") + "/-/merge_requests/mock-" + request.Issue.ID,
		Reviewer:       request.Issue.Author,
		Provider:       "mock",
	}
}

func nextActions(cfg AIConfig) map[string]string {
	return map[string]string{
		"approve": cfg.ApproveCommand,
		"correct": cfg.CorrectCommand,
		"reject":  cfg.RejectCommand,
	}
}

func commentBody(planMarkdown string, actions map[string]string) string {
	return fmt.Sprintf(`%s

---
Reply with one of the configured commands:
- %s to approve and create a mock PR
- %s <feedback> to regenerate the plan
- %s to reject and close as not planned
`, planMarkdown, actions["approve"], actions["correct"], actions["reject"])
}
