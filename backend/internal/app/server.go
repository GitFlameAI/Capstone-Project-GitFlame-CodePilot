package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	cfg         Config
	store       Store
	ml          *MLClient
	gitWorkflow GitWorkflowService
	router      *http.ServeMux
}

func NewServer(cfg Config) (*Server, error) {
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	store, err := NewPostgresStore(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect PostgreSQL storage: %w", err)
	}
	return NewServerWithStore(cfg, store), nil
}

func NewServerWithStore(cfg Config, store Store) *Server {
	server := &Server{
		cfg:         cfg,
		store:       store,
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

func (s *Server) Close() {
	s.store.Close()
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

	session, err := s.store.SaveIssueSession(r.Context(), payload, cfg, planMarkdown)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
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
		s.handleGetIssuePlan(w, r, issueID)
	case r.Method == http.MethodPost && action == "approve":
		s.handleApproveIssue(w, r, issueID)
	case r.Method == http.MethodPost && action == "correct":
		s.handleCorrectIssue(w, r, issueID)
	case r.Method == http.MethodPost && action == "reject":
		s.handleRejectIssue(w, r, issueID)
	default:
		writeError(w, http.StatusNotFound, "issue workflow route was not found")
	}
}

func (s *Server) handleGetIssuePlan(w http.ResponseWriter, r *http.Request, issueID string) {
	session, ok, err := s.store.GetIssueSession(r.Context(), issueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
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

func (s *Server) handleApproveIssue(w http.ResponseWriter, r *http.Request, issueID string) {
	session, ok, err := s.store.GetIssueSession(r.Context(), issueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
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
	if _, err := s.store.UpdateIssueSession(r.Context(), session); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, PlanActionResponse{
		SessionID:   session.SessionID,
		IssueID:     session.Request.Issue.ID,
		Status:      session.Status,
		Message:     "Plan approved. Mock Git workflow payload was created.",
		GitWorkflow: &workflow,
	})
}

func (s *Server) handleCorrectIssue(w http.ResponseWriter, r *http.Request, issueID string) {
	session, ok, err := s.store.GetIssueSession(r.Context(), issueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
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
	if _, err := s.store.UpdateIssueSession(r.Context(), session); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, PlanActionResponse{
		SessionID:    session.SessionID,
		IssueID:      session.Request.Issue.ID,
		Status:       session.Status,
		Message:      "Correction request was saved and the plan revision was updated.",
		PlanMarkdown: session.PlanMarkdown,
	})
}

func (s *Server) handleRejectIssue(w http.ResponseWriter, r *http.Request, issueID string) {
	session, ok, err := s.store.GetIssueSession(r.Context(), issueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "issue session was not found")
		return
	}
	session.Status = statusRejected
	if _, err := s.store.UpdateIssueSession(r.Context(), session); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
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
	cfg, err := ParseAIConfig(payload.YAMLConfig)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	summary, cards, err := s.ml.GenerateRecommendations(r.Context(), payload.YAMLConfig, payload.RepositoryContext)
	if err != nil {
		summary, cards = fallbackRecommendations()
	}
	report, err := s.store.SaveRecommendations(r.Context(), payload, cfg, summary, cards)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
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
	report, ok, err := s.store.GetRecommendationReport(r.Context(), repositoryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
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
		deleted, err := s.store.DeleteRecommendation(r.Context(), rest)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if deleted {
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
	card, found, err := s.store.CloseRecommendation(r.Context(), recommendationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
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
