# Issue
Title: Make ML client resilient to transient failures

The backend currently fails immediately when the ML service is slow or temporarily unavailable. Add configurable request timeout and bounded retries for connection errors, HTTP 429, and HTTP 5xx responses. Do not retry validation errors or cancelled requests. Return useful errors without exposing sensitive response bodies.

# Evaluation Contract
# Implementation Plan Format

Every issue-to-plan response must be valid Markdown and use the sections below in the same order.

```markdown
# Implementation Plan

## Issue Summary
Brief restatement of the requested change.

## Goal
The expected product or technical outcome.

## Relevant Files
- `path/to/existing_file`: why it is relevant.
- `path/to/new_file` (create): why it is needed.

## Proposed Changes
- Concrete behavior or interface changes.

## Implementation Steps
1. Ordered implementation step.
2. Ordered implementation step.

## Expected Files to Change
- `path/to/file`: create or modify.

## Tests and Verification
- Test or verification step.

## Risks and Open Questions
- Risk, dependency, or `TBD` when repository context is insufficient.
```

## Rules

- Return only the implementation plan, without source code or patches.
- Reference only files present in the supplied repository context.
- Mark proposed new files with `(create)`.
- Use `TBD` instead of inventing missing repository details.
- Keep steps concrete, ordered, and testable.



# Repository Context
## File: `backend/internal/app/services.go`
```text
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

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

```

## File: `backend/internal/app/config.go`
```text
package app

import "os"

type Config struct {
	Addr         string
	MLServiceURL string
	DatabaseURL  string
}

func LoadConfig() Config {
	port := envOrDefault("BACKEND_PORT", "8000")
	return Config{
		Addr:         ":" + port,
		MLServiceURL: envOrDefault("ML_SERVICE_URL", "http://localhost:8001"),
		DatabaseURL:  envOrDefault("DATABASE_URL", ""),
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

```

## File: `backend/internal/app/server.go`
```text
package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	cfg         Config
	store       *MemoryStore
	ml          *MLClient
	gitWorkflow GitWorkflowService
	router      *http.ServeMux
}

func NewServer(cfg Config) *Server {
	server := &Server{
		cfg:         cfg,
		store:       NewMemoryStore(),
		ml:          NewMLClient(cfg.MLServiceURL),
		gitWorkflow: NewMockGitWorkflowService(),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.handleHealth)
	mux.HandleFunc("GET /docs", server.handleDocs)
	mux.HandleFunc("GET /swagger/", server.handleDocs)
	mux.HandleFunc("GET /swagger/index.html", server.handleDocs)
	mux.HandleFunc("GET /openapi.json", server.handleOpenAPI)
	mux.HandleFunc("POST /integrations/gitflame/issues/analyze", server.handleAnalyzeIssue)
	mux.HandleFunc("/ai/issues/", server.handleIssueWorkflow)
	mux.HandleFunc("/integrations/gitflame/repositories/", server.handleRecommendationAnalyze)
	mux.HandleFunc("/repositories/", server.handleRepositoryRecommendations)
	mux.HandleFunc("/recommendations/", server.handleRecommendationMutation)
	server.router = mux
	return server
}

func (s *Server) Router() http.Handler {
	return withJSONContentType(s.router)
}

func withJSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/docs" && !strings.HasPrefix(r.URL.Path, "/swagger/") {
			w.Header().Set("Content-Type", "application/json")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "backend",
	})
}

func (s *Server) handleDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>GitFlame CodePilot API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({ url: "/openapi.json", dom_id: "#swagger-ui" });
  </script>
  <noscript>Sprint 1 OpenAPI contract: <a href="/openapi.json">/openapi.json</a></noscript>
</body>
</html>`))
}

func (s *Server) handleAnalyzeIssue(w http.ResponseWriter, r *http.Request) {
	var payload IssueAnalyzeRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateIssueAnalyzeRequest(payload); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	cfg, err := ParseAIConfig(payload.YAMLConfig)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	planMarkdown, err := s.ml.GenerateIssuePlan(r.Context(), payload)
	if err != nil {
		planMarkdown = fallbackIssuePlan(payload)
	}

	session := s.store.SaveIssueSession(payload, cfg, planMarkdown)
	actions := nextActions(cfg)
	writeJSON(w, http.StatusOK, IssueAnalyzeResponse{
		SessionID:    session.SessionID,
		IssueID:      payload.Issue.ID,
		RepositoryID: payload.Repository.ID,
		Status:       session.Status,
		PlanMarkdown: planMarkdown,
		CommentBody:  commentBody(planMarkdown, actions),
		NextActions:  actions,
	})
}

func (s *Server) handleIssueWorkflow(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/ai/issues/")
	issueID, action, ok := strings.Cut(rest, "/")
	if !ok {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/plan") {
			writeError(w, http.StatusNotFound, "issue id is missing")
			return
		}
		writeError(w, http.StatusNotFound, "issue workflow route was not found")
		return
	}

	switch {
	case r.Method == http.MethodGet && action == "plan":
		s.handleGetIssuePlan(w, issueID)
	case r.Method == http.MethodPost && action == "approve":
		s.handleApproveIssue(w, issueID)
	case r.Method == http.MethodPost && action == "correct":
		s.handleCorrectIssue(w, r, issueID)
	case r.Method == http.MethodPost && action == "reject":
		s.handleRejectIssue(w, issueID)
	default:
		writeError(w, http.StatusNotFound, "issue workflow route was not found")
	}
}

func (s *Server) handleGetIssuePlan(w http.ResponseWriter, issueID string) {
	session, ok := s.store.GetIssueSession(issueID)
	if !ok {
		writeError(w, http.StatusNotFound, "issue session was not found")
		return
	}
	actions := nextActions(session.Config)
	writeJSON(w, http.StatusOK, IssuePlanResponse{
		SessionID:    session.SessionID,
		IssueID:      session.Request.Issue.ID,
		RepositoryID: session.Request.Repository.ID,
		Status:       session.Status,
		PlanMarkdown: session.PlanMarkdown,
		CommentBody:  commentBody(session.PlanMarkdown, actions),
		Revision:     session.Revision,
	})
}

func (s *Server) handleApproveIssue(w http.ResponseWriter, issueID string) {
	session, ok := s.store.GetIssueSession(issueID)
	if !ok {
		writeError(w, http.StatusNotFound, "issue session was not found")
		return
	}
	workflowContract, err := s.gitWorkflow.CreatePullRequest(GitWorkflowContractRequest{
		IssueRequest: session.Request,
		Config:       session.Config,
		PlanMarkdown: session.PlanMarkdown,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	workflow := workflowContract.Response
	session.Status = statusApproved
	session.GitWorkflow = &workflow
	s.store.UpdateIssueSession(session)
	writeJSON(w, http.StatusOK, PlanActionResponse{
		SessionID:   session.SessionID,
		IssueID:     session.Request.Issue.ID,
		Status:      session.Status,
		Message:     "Plan approved. Mock Git workflow payload was created.",
		GitWorkflow: &workflow,
	})
}

func (s *Server) handleCorrectIssue(w http.ResponseWriter, r *http.Request, issueID string) {
	session, ok := s.store.GetIssueSession(issueID)
	if !ok {
		writeError(w, http.StatusNotFound, "issue session was not found")
		return
	}
	var payload PlanCorrectionRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(payload.Feedback) == "" {
		writeError(w, http.StatusUnprocessableEntity, "feedback is required")
		return
	}
	session.FeedbackHistory = append(session.FeedbackHistory, payload.Feedback)
	session.Revision++
	session.Status = statusCorrectionRequested
	session.PlanMarkdown = session.PlanMarkdown + "\n\n## Revision feedback\n- " + payload.Feedback + "\n- Update implementation steps before approval.\n"
	s.store.UpdateIssueSession(session)
	writeJSON(w, http.StatusOK, PlanActionResponse{
		SessionID:    session.SessionID,
		IssueID:      session.Request.Issue.ID,
		Status:       session.Status,
		Message:      "Correction request was saved and the plan revision was updated.",
		PlanMarkdown: session.PlanMarkdown,
	})
}

func (s *Server) handleRejectIssue(w http.ResponseWriter, issueID string) {
	session, ok := s.store.GetIssueSession(issueID)
	if !ok {
		writeError(w, http.StatusNotFound, "issue session was not found")
		return
	}
	session.Status = statusRejected
	s.store.UpdateIssueSession(session)
	writeJSON(w, http.StatusOK, PlanActionResponse{
		SessionID: session.SessionID,
		IssueID:   session.Request.Issue.ID,
		Status:    session.Status,
		Message:   "Plan rejected. External GitFlame can close the issue as not planned.",
	})
}

func (s *Server) handleRecommendationAnalyze(w http.ResponseWriter, r *http.Request) {
	const prefix = "/integrations/gitflame/repositories/"
	const suffix = "/recommendations/analyze"
	if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, suffix) {
		writeError(w, http.StatusNotFound, "recommendation analyze route was not found")
		return
	}
	repositoryID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), suffix)
	if repositoryID == "" {
		writeError(w, http.StatusNotFound, "repository id is missing")
		return
	}

	var payload RecommendationAnalyzeRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if payload.Repository.ID != repositoryID {
		writeError(w, http.StatusUnprocessableEntity, "path repository id must match payload repository id")
		return
	}
	if _, err := ParseAIConfig(payload.YAMLConfig); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	summary, cards, err := s.ml.GenerateRecommendations(r.Context(), payload.YAMLConfig, payload.RepositoryContext)
	if err != nil {
		summary, cards = fallbackRecommendations()
	}
	report := s.store.SaveRecommendations(repositoryID, summary, cards)
	writeJSON(w, http.StatusOK, RecommendationAnalyzeResponse{
		RepositoryID:    report.RepositoryID,
		Status:          report.Status,
		Summary:         report.Summary,
		Recommendations: report.Recommendations,
	})
}

func (s *Server) handleRepositoryRecommendations(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/repositories/")
	var repositoryID string
	var action string
	for _, suffix := range []string{"/recommendations/status", "/recommendations/summary", "/recommendations"} {
		if strings.HasSuffix(rest, suffix) {
			repositoryID = strings.TrimSuffix(rest, suffix)
			action = strings.TrimPrefix(suffix, "/recommendations")
			break
		}
	}
	if repositoryID == "" {
		writeError(w, http.StatusNotFound, "repository recommendation route was not found")
		return
	}
	report, ok := s.store.GetRecommendationReport(repositoryID)
	if !ok {
		writeError(w, http.StatusNotFound, "recommendation report was not found for repository")
		return
	}

	switch {
	case r.Method == http.MethodGet && action == "/status":
		closed := 0
		for _, card := range report.Recommendations {
			if card.State == recommendationClosed {
				closed++
			}
		}
		total := len(report.Recommendations)
		writeJSON(w, http.StatusOK, RecommendationStatusResponse{
			RepositoryID: repositoryID,
			Status:       report.Status,
			Total:        total,
			Open:         total - closed,
			Closed:       closed,
		})
	case r.Method == http.MethodGet && action == "/summary":
		writeJSON(w, http.StatusOK, RecommendationSummaryResponse{
			RepositoryID: repositoryID,
			Summary:      report.Summary,
		})
	case r.Method == http.MethodGet && action == "":
		writeJSON(w, http.StatusOK, RecommendationListResponse{
			RepositoryID:    repositoryID,
			Recommendations: report.Recommendations,
		})
	default:
		writeError(w, http.StatusNotFound, "repository recommendation route was not found")
	}
}

func (s *Server) handleRecommendationMutation(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/recommendations/")
	recommendationID, action, ok := strings.Cut(rest, "/")
	if !ok && r.Method == http.MethodDelete {
		if s.store.DeleteRecommendation(rest) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeError(w, http.StatusNotFound, "recommendation was not found")
		return
	}
	if !ok || action != "close" || r.Method != http.MethodPatch {
		writeError(w, http.StatusNotFound, "recommendation route was not found")
		return
	}
	card, found := s.store.CloseRecommendation(recommendationID)
	if !found {
		writeError(w, http.StatusNotFound, "recommendation was not found")
		return
	}
	writeJSON(w, http.StatusOK, card)
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

func validateIssueAnalyzeRequest(payload IssueAnalyzeRequest) error {
	switch {
	case strings.TrimSpace(payload.Repository.ID) == "":
		return errors.New("repository.id is required")
	case strings.TrimSpace(payload.Issue.ID) == "":
		return errors.New("issue.id is required")
	case strings.TrimSpace(payload.Issue.Title) == "":
		return errors.New("issue.title is required")
	case strings.TrimSpace(payload.Issue.Author) == "":
		return errors.New("issue.author is required")
	default:
		return nil
	}
}

func newID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(bytes[:])
}

```
