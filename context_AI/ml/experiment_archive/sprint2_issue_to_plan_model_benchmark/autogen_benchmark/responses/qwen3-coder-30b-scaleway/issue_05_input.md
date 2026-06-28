# Issue
Title: Expire recommendation results using repository retention settings

Recommendation results must be retained for the number of days configured by the repository owner and then removed automatically. Add an explicit expiration timestamp, ensure expired results are not returned by read APIs, and provide a safe cleanup operation that can run repeatedly. Existing close and delete behavior must remain unchanged.

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
## File: `backend/db/schema.sql`
```text
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS repositories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    owner TEXT NOT NULL,
    default_branch TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT repositories_owner_name_unique UNIQUE (owner, name)
);

CREATE TABLE IF NOT EXISTS ai_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    raw_yml TEXT NOT NULL,
    parsed_config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_valid BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS issue_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    ai_config_id UUID NOT NULL REFERENCES ai_configs(id),
    issue_title TEXT NOT NULL,
    issue_body TEXT NOT NULL DEFAULT '',
    issue_author TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'created',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT issue_sessions_status_check CHECK (
        status IN (
            'created',
            'plan_generated',
            'approved',
            'correction_requested',
            'rejected'
        )
    )
);

CREATE TABLE IF NOT EXISTS generated_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_session_id UUID NOT NULL REFERENCES issue_sessions(id) ON DELETE CASCADE,
    plan_markdown TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_responses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_session_id UUID NOT NULL REFERENCES issue_sessions(id) ON DELETE CASCADE,
    response_type TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT user_responses_type_check CHECK (
        response_type IN ('approve', 'correct', 'reject')
    )
);

CREATE TABLE IF NOT EXISTS recommendation_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    ai_config_id UUID NOT NULL REFERENCES ai_configs(id),
    summary TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT recommendation_runs_status_check CHECK (
        status IN ('pending', 'completed', 'failed')
    )
);

CREATE TABLE IF NOT EXISTS recommendations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recommendation_run_id UUID NOT NULL REFERENCES recommendation_runs(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    line_number INTEGER,
    category TEXT NOT NULL,
    severity TEXT NOT NULL,
    problem TEXT NOT NULL,
    suggestion TEXT NOT NULL,
    current_status TEXT NOT NULL DEFAULT 'open',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT recommendations_line_number_check CHECK (
        line_number IS NULL OR line_number > 0
    ),
    CONSTRAINT recommendations_current_status_check CHECK (
        current_status IN ('open', 'closed', 'deleted')
    )
);

CREATE TABLE IF NOT EXISTS recommendation_statuses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recommendation_id UUID NOT NULL REFERENCES recommendations(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    changed_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT recommendation_statuses_status_check CHECK (
        status IN ('open', 'closed', 'deleted')
    )
);

CREATE INDEX IF NOT EXISTS idx_ai_configs_repository_id
    ON ai_configs(repository_id);

CREATE INDEX IF NOT EXISTS idx_issue_sessions_repository_id
    ON issue_sessions(repository_id);

CREATE INDEX IF NOT EXISTS idx_issue_sessions_ai_config_id
    ON issue_sessions(ai_config_id);

CREATE INDEX IF NOT EXISTS idx_generated_plans_issue_session_id
    ON generated_plans(issue_session_id);

CREATE INDEX IF NOT EXISTS idx_user_responses_issue_session_id
    ON user_responses(issue_session_id);

CREATE INDEX IF NOT EXISTS idx_recommendation_runs_repository_id
    ON recommendation_runs(repository_id);

CREATE INDEX IF NOT EXISTS idx_recommendation_runs_ai_config_id
    ON recommendation_runs(ai_config_id);

CREATE INDEX IF NOT EXISTS idx_recommendations_run_id
    ON recommendations(recommendation_run_id);

CREATE INDEX IF NOT EXISTS idx_recommendation_statuses_recommendation_id
    ON recommendation_statuses(recommendation_id);

```

## File: `backend/internal/app/storage.go`
```text
package app

import (
	"sync"
	"time"
)

const (
	statusPlanGenerated       = "plan_generated"
	statusApproved            = "approved"
	statusCorrectionRequested = "correction_requested"
	statusRejected            = "rejected"

	recommendationOpen   = "open"
	recommendationClosed = "closed"
)

type IssueSession struct {
	SessionID       string
	Request         IssueAnalyzeRequest
	Config          AIConfig
	PlanMarkdown    string
	Status          string
	Revision        int
	GitWorkflow     *GitWorkflowResponse
	FeedbackHistory []string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type RecommendationReport struct {
	RepositoryID    string
	Summary         string
	Recommendations []RecommendationCard
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type MemoryStore struct {
	mu                    sync.RWMutex
	issueSessions         map[string]*IssueSession
	issueIndex            map[string]string
	recommendationReports map[string]*RecommendationReport
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		issueSessions:         make(map[string]*IssueSession),
		issueIndex:            make(map[string]string),
		recommendationReports: make(map[string]*RecommendationReport),
	}
}

func (s *MemoryStore) SaveIssueSession(request IssueAnalyzeRequest, cfg AIConfig, planMarkdown string) *IssueSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	session := &IssueSession{
		SessionID:    newID(),
		Request:      request,
		Config:       cfg,
		PlanMarkdown: planMarkdown,
		Status:       statusPlanGenerated,
		Revision:     1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.issueSessions[session.SessionID] = session
	s.issueIndex[request.Issue.ID] = session.SessionID
	return cloneIssueSession(session)
}

func (s *MemoryStore) GetIssueSession(issueOrSessionID string) (*IssueSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if session, ok := s.issueSessions[issueOrSessionID]; ok {
		return cloneIssueSession(session), true
	}
	sessionID, ok := s.issueIndex[issueOrSessionID]
	if !ok {
		return nil, false
	}
	session, ok := s.issueSessions[sessionID]
	if !ok {
		return nil, false
	}
	return cloneIssueSession(session), true
}

func (s *MemoryStore) UpdateIssueSession(session *IssueSession) *IssueSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	session.UpdatedAt = time.Now().UTC()
	s.issueSessions[session.SessionID] = cloneIssueSession(session)
	s.issueIndex[session.Request.Issue.ID] = session.SessionID
	return cloneIssueSession(session)
}

func (s *MemoryStore) SaveRecommendations(repositoryID, summary string, cards []RecommendationCard) *RecommendationReport {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	report := &RecommendationReport{
		RepositoryID:    repositoryID,
		Summary:         summary,
		Recommendations: append([]RecommendationCard(nil), cards...),
		Status:          "ready",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.recommendationReports[repositoryID] = report
	return cloneRecommendationReport(report)
}

func (s *MemoryStore) GetRecommendationReport(repositoryID string) (*RecommendationReport, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	report, ok := s.recommendationReports[repositoryID]
	if !ok {
		return nil, false
	}
	return cloneRecommendationReport(report), true
}

func (s *MemoryStore) CloseRecommendation(recommendationID string) (RecommendationCard, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, report := range s.recommendationReports {
		for i := range report.Recommendations {
			if report.Recommendations[i].ID == recommendationID {
				report.Recommendations[i].State = recommendationClosed
				report.UpdatedAt = time.Now().UTC()
				return report.Recommendations[i], true
			}
		}
	}
	return RecommendationCard{}, false
}

func (s *MemoryStore) DeleteRecommendation(recommendationID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, report := range s.recommendationReports {
		next := report.Recommendations[:0]
		deleted := false
		for _, card := range report.Recommendations {
			if card.ID == recommendationID {
				deleted = true
				continue
			}
			next = append(next, card)
		}
		if deleted {
			report.Recommendations = next
			report.UpdatedAt = time.Now().UTC()
			return true
		}
	}
	return false
}

func cloneIssueSession(session *IssueSession) *IssueSession {
	clone := *session
	clone.Request.RepositoryContext = append([]string(nil), session.Request.RepositoryContext...)
	clone.FeedbackHistory = append([]string(nil), session.FeedbackHistory...)
	return &clone
}

func cloneRecommendationReport(report *RecommendationReport) *RecommendationReport {
	clone := *report
	clone.Recommendations = append([]RecommendationCard(nil), report.Recommendations...)
	return &clone
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

## File: `backend/models/recommendation_run.go`
```text
package models

type RecommendationRun struct {
	ID           UUID                    `db:"id" json:"id"`
	RepositoryID UUID                    `db:"repository_id" json:"repository_id"`
	AIConfigID   UUID                    `db:"ai_config_id" json:"ai_config_id"`
	Summary      string                  `db:"summary" json:"summary"`
	Status       RecommendationRunStatus `db:"status" json:"status"`
	Timestamps
}

```

## File: `backend/models/recommendation.go`
```text
package models

type Recommendation struct {
	ID                  UUID                      `db:"id" json:"id"`
	RecommendationRunID UUID                      `db:"recommendation_run_id" json:"recommendation_run_id"`
	FilePath            string                    `db:"file_path" json:"file_path"`
	LineNumber          *int                      `db:"line_number" json:"line_number,omitempty"`
	Category            string                    `db:"category" json:"category"`
	Severity            string                    `db:"severity" json:"severity"`
	Problem             string                    `db:"problem" json:"problem"`
	Suggestion          string                    `db:"suggestion" json:"suggestion"`
	CurrentStatus       RecommendationStatusValue `db:"current_status" json:"current_status"`
	Timestamps
}

```

## File: `docs/config/ai_config.example.yml`
```text
version: 1

repository:
  default_branch: main
  target_branch_prefix: ai/

analysis:
  enabled: true
  include:
    - src/**
    - internal/**
  exclude:
    - node_modules/**
    - dist/**
    - build/**
    - .git/**

code_generation:
  enabled: true
  require_user_approval: true
  reviewer_policy: issue_author
  allowed_actions:
    approve_command: "/approve"
    correct_command: "/correct"
    reject_command: "/reject"

recommendations:
  enabled: true
  severity_threshold: low
  categories:
    - code_duplication
    - security
    - maintainability
    - performance
    - architecture

rag:
  max_files: 20
  max_file_size_kb: 120
  context_strategy: issue_relevant_files

storage:
  recommendation_ttl_days: 30


```
