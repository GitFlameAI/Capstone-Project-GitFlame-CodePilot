package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"gitflame-codepilot/backend/internal/agent"
	"gitflame-codepilot/backend/internal/config"
	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/observability"
	"gitflame-codepilot/backend/internal/queue"
	"gitflame-codepilot/backend/internal/repository"
	"gitflame-codepilot/backend/internal/security"
	"gitflame-codepilot/backend/internal/service"
)

type Server struct {
	workflow          *service.Workflow
	store             repository.Store
	gitflame          GitFlameSource
	recommender       RecommendationAnalyzer
	indexer           RepositoryIndexer
	router            *http.ServeMux
	checks            map[string]func(context.Context) error
	indexJobs         *indexJobRegistry
	broker            queue.Broker
	credentialCipher  *security.CredentialCipher
	gitflameBaseURL   string
	gitflameTimeout   time.Duration
	sessionCookie     string
	sessionTTL        time.Duration
	sessionSecure     bool
	publicBaseURL     string
	webhookSigningKey [32]byte

	// indexBackgroundTimeout bounds one background indexing job, and
	// indexWaitTimeout bounds how long a request waits for that job.
	indexBackgroundTimeout time.Duration
	indexWaitTimeout       time.Duration
}

func New(cfg config.Config) (*Server, error) {
	var store repository.Store = repository.NewMemoryStore()
	if cfg.DatabaseURL != "" {
		postgres, err := repository.NewPostgresStore(context.Background(), cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}
		store = postgres
	}
	engine := agent.NewClient(cfg.AgentEngineURL, cfg.AgentTimeout)
	var gitflame GitFlameSource
	var credentialCipher *security.CredentialCipher
	if strings.TrimSpace(cfg.GitFlameCredentialKey) != "" {
		var err error
		credentialCipher, err = security.NewCredentialCipher(cfg.GitFlameCredentialKey, cfg.GitFlameCredentialKeyVersion)
		if err != nil {
			return nil, err
		}
	}
	recommender := NewRecommendationClient(cfg.RecommendationServiceURL, cfg.RecommendationTimeout)
	indexer := NewRAGClient(cfg.RAGBaseURL, cfg.RAGAPIKey, cfg.RAGIndexTimeout)
	checks := map[string]func(context.Context) error{"storage": store.Ping, "agent_engine": engine.Ready}
	if indexer != nil {
		checks["rag"] = indexer.Ready
	}
	if cfg.DispatchMode == "redis" {
		if cfg.DatabaseURL == "" {
			return nil, errors.New("TASK_DISPATCH_MODE=redis requires DATABASE_URL")
		}
		broker, err := queue.NewRedisBroker(cfg.RedisURL, cfg.AgentQueueName, cfg.AgentConsumerGroup, cfg.QueueMaxLength)
		if err != nil {
			return nil, err
		}
		checks["redis"] = broker.Ping
		server := newServer(service.NewQueuedWorkflow(store, broker), store, gitflame, recommender, indexer, checks, cfg, credentialCipher)
		server.broker = broker
		return server, nil
	}
	return newServer(service.NewWorkflow(store, engine), store, gitflame, recommender, indexer, checks, cfg, credentialCipher), nil
}
func NewWithDependencies(store repository.Store, generator agent.Generator) *Server {
	return NewWithDependenciesAndIntegrations(store, generator, nil, nil)
}

func NewWithDependenciesAndIntegrations(store repository.Store, generator agent.Generator, gitflame GitFlameSource, recommender RecommendationAnalyzer) *Server {
	return newServer(service.NewWorkflow(store, generator), store, gitflame, recommender, nil, map[string]func(context.Context) error{"storage": store.Ping}, config.Config{SessionCookieName: "codepilot_session", SessionTTL: 168 * time.Hour}, nil)
}

func newServer(workflow *service.Workflow, store repository.Store, gitflame GitFlameSource, recommender RecommendationAnalyzer, indexer RepositoryIndexer, checks map[string]func(context.Context) error, cfg config.Config, credentialCipher *security.CredentialCipher) *Server {
	if cfg.SessionCookieName == "" {
		cfg.SessionCookieName = "codepilot_session"
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 168 * time.Hour
	}
	if cfg.RAGIndexTimeout <= 0 {
		cfg.RAGIndexTimeout = defaultIndexWaitTimeout
	}
	if cfg.RAGIndexWaitTimeout <= 0 {
		cfg.RAGIndexWaitTimeout = defaultIndexWaitTimeout
	}
	s := &Server{
		workflow: workflow, store: store, gitflame: gitflame, recommender: recommender, indexer: indexer, checks: checks,
		credentialCipher: credentialCipher, gitflameBaseURL: cfg.GitFlameBaseURL, gitflameTimeout: cfg.GitFlameTimeout,
		sessionCookie: cfg.SessionCookieName, sessionTTL: cfg.SessionTTL, sessionSecure: cfg.SessionCookieSecure,
		publicBaseURL:     strings.TrimRight(cfg.PublicBaseURL, "/"),
		webhookSigningKey: sha256.Sum256([]byte(cfg.GitFlameCredentialKey)),
		indexJobs:         newIndexJobRegistry(),

		indexBackgroundTimeout: cfg.RAGIndexTimeout,
		indexWaitTimeout:       cfg.RAGIndexWaitTimeout,
	}
	mux := http.NewServeMux()
	route(mux, "GET /health", s.health)
	route(mux, "GET /version", s.version)
	route(mux, "GET /ready", s.ready)
	route(mux, "GET /docs", s.docs)
	route(mux, "GET /swagger/", s.docs)
	route(mux, "GET /swagger/index.html", s.docs)
	route(mux, "GET /openapi.json", s.openAPI)
	// Registered directly: a scrape must not measure itself, and nginx blocks
	// this path from the outside so only Prometheus on the Docker network sees it.
	mux.Handle("GET /metrics", observability.MetricsHandler())
	route(mux, "GET /ops/status", s.opsStatus)
	route(mux, "GET /ops/tasks", s.opsTasks)
	route(mux, "GET /ops/dead-letter", s.opsDeadLetter)
	route(mux, "POST /auth/gitflame/session", s.createGitFlameSession)
	route(mux, "DELETE /auth/session", s.revokeSession)
	route(mux, "POST /integrations/gitflame/connections", s.saveGitFlameConnection)
	route(mux, "PUT /integrations/gitflame/connections/{id}", s.reconnectGitFlameConnection)
	route(mux, "DELETE /integrations/gitflame/connections/{id}", s.revokeGitFlameConnection)
	route(mux, "GET /integrations/gitflame/connections/{id}/tree", s.repositoryTree)
	route(mux, "GET /integrations/gitflame/connections/{id}/files", s.repositoryFiles)
	route(mux, "GET /integrations/gitflame/connections/{id}/issues", s.repositoryIssues)
	route(mux, "GET /integrations/gitflame/connections/{id}/index", s.repositoryIndexStatus)
	route(mux, "PUT /integrations/gitflame/connections/{id}/config", s.saveRepositoryConfig)
	route(mux, "POST /integrations/gitflame/connections/{id}/webhook", s.createGitFlameWebhook)
	route(mux, "GET /integrations/gitflame/connections/{id}/webhook", s.getGitFlameWebhook)
	route(mux, "DELETE /integrations/gitflame/connections/{id}/webhook", s.disableGitFlameWebhook)
	route(mux, "GET /integrations/gitflame/connections/{id}/webhook/events", s.listGitFlameWebhookEvents)
	route(mux, "POST /integrations/gitflame/webhooks/{id}", s.receiveGitFlameWebhook)
	route(mux, "POST /integrations/gitflame/issues/analyze", s.analyze)
	route(mux, "GET /ai/tasks/{taskId}", s.task)
	route(mux, "POST /ai/tasks/{taskId}/retry", s.retryTask)
	route(mux, "GET /ai/issues/{id}/plan", s.plan)
	route(mux, "POST /ai/issues/{id}/approve", s.approve)
	route(mux, "GET /ai/issues/{id}/code-generation", s.codeGenerationStatus)
	route(mux, "POST /ai/issues/{id}/gitflame/apply", s.applyGeneratedFiles)
	route(mux, "POST /ai/issues/{id}/correct", s.correct)
	route(mux, "POST /ai/issues/{id}/reject", s.reject)
	route(mux, "POST /integrations/gitflame/recommendations/analyze", s.analyzeRecommendations)
	route(mux, "POST /integrations/gitflame/repositories/{id}/recommendations/analyze", s.analyzeRecommendations)
	route(mux, "GET /repositories/recommendations/status", s.recommendationStatus)
	route(mux, "GET /repositories/recommendations/summary", s.recommendationSummary)
	route(mux, "GET /repositories/recommendations", s.recommendations)
	route(mux, "GET /repositories/{id}/recommendations/status", s.recommendationStatus)
	route(mux, "GET /repositories/{id}/recommendations/summary", s.recommendationSummary)
	route(mux, "GET /repositories/{id}/recommendations", s.recommendations)
	route(mux, "PATCH /recommendations/{id}/close", s.closeRecommendation)
	route(mux, "DELETE /recommendations/{id}", s.deleteRecommendation)
	s.router = mux
	return s
}

// Router builds the middleware chain. Order matters: the request identifier has
// to exist before anything logs, and panic recovery sits innermost so that the
// recovered 500 is still counted and logged as a normal response.
func (s *Server) Router() http.Handler {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/docs" && !strings.HasPrefix(r.URL.Path, "/swagger/") {
			w.Header().Set("Content-Type", "application/json")
		}
		s.router.ServeHTTP(w, r)
	})
	return observability.RequestIDMiddleware(requestLogger(recoverMiddleware(handler)))
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	write(w, 200, map[string]string{"status": "ok", "service": "backend"})
}

// version answers "what exactly is running here", which is the first question of
// every incident and the only reliable way to tell a redeploy from a rollback.
func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, observability.Build("backend"))
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	components := make(map[string]string, len(s.checks))
	status := http.StatusOK
	for name, check := range s.checks {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		err := check(ctx)
		cancel()
		if err != nil {
			components[name] = "unavailable"
			status = http.StatusServiceUnavailable
		} else {
			components[name] = "ready"
		}
	}
	state := "ready"
	if status != http.StatusOK {
		state = "not_ready"
	}
	write(w, status, map[string]any{"status": state, "service": "backend", "components": components})
}
func (s *Server) docs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><head><title>GitFlame CodePilot API</title><link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"></head><body><div id="swagger-ui"></div><script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script><script>SwaggerUIBundle({url:"/openapi.json",dom_id:"#swagger-ui"})</script></body></html>`))
}

type analyzeResponse struct {
	SessionID    string `json:"session_id"`
	TaskID       string `json:"task_id"`
	IssueID      string `json:"issue_id"`
	RepositoryID string `json:"repository_id"`
	Status       string `json:"status"`
	StatusURL    string `json:"status_url"`
}

func (s *Server) analyze(w http.ResponseWriter, r *http.Request) {
	var req domain.IssueAnalyzeRequest
	if err := decode(r, &req); err != nil {
		problem(w, 400, "invalid_json", err.Error())
		return
	}
	var err error
	req, err = s.hydrateAnalyzeRequest(r, req)
	if err != nil {
		integrationError(w, err, "gitflame_repository_error")
		return
	}
	if err := s.ensureRequestRepositoryIndex(r, &req.Repository); err != nil {
		integrationError(w, err, "rag_indexing_failed")
		return
	}
	session, task, err := s.workflow.Analyze(req)
	if err != nil {
		workflowError(w, err)
		return
	}
	write(w, 202, analyzeResponse{session.ID, task.ID, req.Issue.ID, req.Repository.ID, task.Status, "/ai/tasks/" + task.ID})
}

type taskResponse struct {
	TaskID               string                         `json:"task_id"`
	SessionID            string                         `json:"session_id"`
	IssueID              string                         `json:"issue_id"`
	Type                 string                         `json:"type"`
	Status               string                         `json:"status"`
	Attempt              int                            `json:"attempt"`
	PlanMarkdown         string                         `json:"plan_markdown,omitempty"`
	ToolExecutionSummary string                         `json:"tool_execution_summary,omitempty"`
	RelevantFiles        []domain.RelevantFile          `json:"relevant_files,omitempty"`
	Model                string                         `json:"model,omitempty"`
	Usage                domain.AgentUsage              `json:"usage,omitempty"`
	Error                *domain.TaskError              `json:"error,omitempty"`
	GeneratedFiles       *domain.GeneratedFilesContract `json:"generated_files_contract,omitempty"`
}

func (s *Server) task(w http.ResponseWriter, r *http.Request) {
	v, err := s.workflow.Task(r.PathValue("taskId"))
	if err != nil {
		resourceError(w, err, "task_not_found", "agent task was not found")
		return
	}
	response := taskResponse{
		TaskID: v.ID, SessionID: v.SessionID, IssueID: v.IssueID, Type: v.Type,
		Status: v.Status, Attempt: v.Attempt, PlanMarkdown: v.PlanMarkdown, ToolExecutionSummary: v.ToolExecutionSummary,
		RelevantFiles: v.RelevantFiles, Model: v.Model, Usage: v.Usage, Error: v.Error,
	}
	if v.Type == domain.TaskCodeGeneration {
		if session, err := s.workflow.Session(v.SessionID); err == nil {
			response.GeneratedFiles = session.GeneratedFiles
		}
	}
	write(w, 200, response)
}

func (s *Server) retryTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.workflow.Retry(r.PathValue("taskId"))
	if err != nil {
		workflowError(w, err)
		return
	}
	write(w, http.StatusAccepted, actionResponse{SessionID: task.SessionID, IssueID: task.IssueID, Status: task.Status, Message: "Retry task queued.", TaskID: task.ID, StatusURL: "/ai/tasks/" + task.ID})
}

type planResponse struct {
	SessionID    string `json:"session_id"`
	IssueID      string `json:"issue_id"`
	RepositoryID string `json:"repository_id"`
	Status       string `json:"status"`
	PlanMarkdown string `json:"plan_markdown"`
	Revision     int    `json:"revision"`
}

func (s *Server) plan(w http.ResponseWriter, r *http.Request) {
	v, err := s.workflow.Session(r.PathValue("id"))
	if err != nil {
		resourceError(w, err, "session_not_found", "issue session was not found")
		return
	}
	if strings.TrimSpace(v.PlanMarkdown) == "" {
		problem(w, 409, "plan_not_ready", "plan generation has not completed")
		return
	}
	write(w, 200, planResponse{v.ID, v.Request.Issue.ID, v.Request.Repository.ID, v.Status, v.PlanMarkdown, v.Revision})
}

type actionResponse struct {
	SessionID      string                         `json:"session_id"`
	IssueID        string                         `json:"issue_id"`
	Status         string                         `json:"status"`
	Message        string                         `json:"message"`
	TaskID         string                         `json:"task_id,omitempty"`
	StatusURL      string                         `json:"status_url,omitempty"`
	GeneratedFiles *domain.GeneratedFilesContract `json:"generated_files_contract,omitempty"`
}

func (s *Server) approve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlanMarkdown *string `json:"plan_markdown"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &req); err != nil {
			problem(w, 400, "invalid_json", err.Error())
			return
		}
	}
	v, task, err := s.workflow.Approve(r.PathValue("id"), req.PlanMarkdown)
	if err != nil {
		workflowError(w, err)
		return
	}
	write(w, http.StatusAccepted, actionResponse{SessionID: v.ID, IssueID: v.Request.Issue.ID, Status: v.Status, Message: "Plan approved. Code generation task queued.", TaskID: task.ID, StatusURL: "/ai/issues/" + v.Request.Issue.ID + "/code-generation", GeneratedFiles: v.GeneratedFiles})
}

func (s *Server) codeGenerationStatus(w http.ResponseWriter, r *http.Request) {
	session, err := s.workflow.Session(r.PathValue("id"))
	if err != nil {
		resourceError(w, err, "session_not_found", "issue session was not found")
		return
	}
	task, err := s.store.LatestTask(session.ID)
	if err != nil {
		resourceError(w, err, "task_not_found", "code generation task was not found")
		return
	}
	if task.Type != domain.TaskCodeGeneration {
		problem(w, http.StatusConflict, "code_generation_not_started", "code generation has not been queued for this issue")
		return
	}
	write(w, 200, taskResponse{
		TaskID: task.ID, SessionID: task.SessionID, IssueID: task.IssueID, Type: task.Type,
		Status: task.Status, Attempt: task.Attempt, ToolExecutionSummary: task.ToolExecutionSummary,
		Model: task.Model, Usage: task.Usage, Error: task.Error, GeneratedFiles: session.GeneratedFiles,
	})
}

func (s *Server) applyGeneratedFiles(w http.ResponseWriter, r *http.Request) {
	session, err := s.workflow.Session(r.PathValue("id"))
	if err != nil {
		resourceError(w, err, "session_not_found", "issue session was not found")
		return
	}
	if session.GeneratedFiles == nil || len(session.GeneratedFiles.Files) == 0 {
		problem(w, http.StatusConflict, "generated_files_not_ready", "code generation has not produced files to apply")
		return
	}
	if session.GeneratedFiles.ApplyStatus == "applied" {
		write(w, http.StatusOK, actionResponse{SessionID: session.ID, IssueID: session.Request.Issue.ID, Status: session.Status, Message: "Generated files already applied to GitFlame.", GeneratedFiles: session.GeneratedFiles})
		return
	}
	applyContract := *session.GeneratedFiles
	applyContract.Files = service.DropNoopGeneratedFilesForApply(session.GeneratedFiles.Files, session.Request.RepositoryFiles)
	now := time.Now().UTC()
	if len(applyContract.Files) == 0 {
		session.GeneratedFiles.ApplyStatus = "failed"
		session.GeneratedFiles.ApplyError = "generated files contain no effective changes to apply"
		session.GeneratedFiles.AppliedAt = &now
		for index := range session.GeneratedFiles.Files {
			session.GeneratedFiles.Files[index].Status = "skipped"
		}
		if err := s.store.UpdateSession(session); err != nil {
			problem(w, http.StatusInternalServerError, "storage_error", err.Error())
			return
		}
		problem(w, http.StatusConflict, "no_effective_generated_changes", "generated files contain no effective changes to apply")
		return
	}
	gitflame, connection, err := s.gitFlameSourceForRepository(r, session.Request.Repository.ID)
	if err != nil {
		integrationError(w, err, "gitflame_client_unavailable")
		return
	}
	if gitflame == nil {
		problem(w, http.StatusServiceUnavailable, "gitflame_client_unavailable", "GitFlame API client is not configured")
		return
	}
	result, err := gitflame.ApplyGeneratedFiles(r.Context(), session.Request.Repository, applyContract)
	if err != nil {
		session.GeneratedFiles.ApplyStatus = "failed"
		session.GeneratedFiles.ApplyError = err.Error()
		session.GeneratedFiles.AppliedAt = &now
		_ = s.store.UpdateSession(session)
		integrationError(w, err, "gitflame_apply_error")
		return
	}
	session.GeneratedFiles.ApplyStatus = "applied"
	session.GeneratedFiles.ApplyError = ""
	session.GeneratedFiles.CommitSHA = result.CommitSHA
	session.GeneratedFiles.PullRequestID = result.PullRequestID
	session.GeneratedFiles.PullRequestURL = result.PullRequestURL
	session.GeneratedFiles.AppliedAt = &now
	for index := range session.GeneratedFiles.Files {
		session.GeneratedFiles.Files[index].Status = "applied"
	}
	if err := s.store.UpdateSession(session); err != nil {
		problem(w, http.StatusInternalServerError, "storage_error", err.Error())
		return
	}
	if connection != nil {
		_ = s.store.TouchGitFlameConnection(connection.UserID, connection.ID)
	}
	write(w, http.StatusOK, actionResponse{SessionID: session.ID, IssueID: session.Request.Issue.ID, Status: session.Status, Message: "Generated files applied to GitFlame.", GeneratedFiles: session.GeneratedFiles})
}

func (s *Server) correct(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Feedback string `json:"feedback"`
	}
	if err := decode(r, &req); err != nil {
		problem(w, 400, "invalid_json", err.Error())
		return
	}
	task, err := s.workflow.Correct(r.PathValue("id"), req.Feedback)
	if err != nil {
		workflowError(w, err)
		return
	}
	write(w, 202, actionResponse{SessionID: task.SessionID, IssueID: task.IssueID, Status: task.Status, Message: "Correction task queued.", TaskID: task.ID, StatusURL: "/ai/tasks/" + task.ID})
}
func (s *Server) reject(w http.ResponseWriter, r *http.Request) {
	v, err := s.workflow.Reject(r.PathValue("id"))
	if err != nil {
		workflowError(w, err)
		return
	}
	write(w, 200, actionResponse{SessionID: v.ID, IssueID: v.Request.Issue.ID, Status: v.Status, Message: "Plan rejected."})
}

func (s *Server) analyzeRecommendations(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Repository        domain.RepositoryMetadata `json:"repository"`
		YAMLConfig        string                    `json:"yaml_config"`
		Categories        []string                  `json:"categories"`
		RepositoryFiles   []domain.RepositoryFile   `json:"repository_files"`
		RepositoryContext []string                  `json:"repository_context"`
	}
	if err := decode(r, &req); err != nil {
		problem(w, 400, "invalid_json", err.Error())
		return
	}
	repositoryID := repositoryIDFromRequest(r)
	if repositoryID != "" && req.Repository.ID != repositoryID {
		problem(w, 422, "validation_error", "path repository id must match payload repository id")
		return
	}
	cfg, err := service.ParseAIConfig(req.YAMLConfig)
	if err != nil {
		problem(w, 422, "validation_error", err.Error())
		return
	}
	if !cfg.RecommendationsEnabled {
		report, err := s.store.SaveRecommendations(req.Repository, cfg, "Recommendation analysis is disabled by .ai.yml categories.", []domain.RecommendationCard{})
		if err != nil {
			problem(w, 500, "storage_error", err.Error())
			return
		}
		write(w, 200, recommendationReportResponse(report))
		return
	}
	files := append([]domain.RepositoryFile(nil), req.RepositoryFiles...)
	if len(files) == 0 {
		for _, path := range req.RepositoryContext {
			files = append(files, domain.RepositoryFile{Path: path})
		}
	}
	reader, connection, err := s.gitFlameReaderForRepository(r, req.Repository.ID)
	if err != nil {
		integrationError(w, err, "gitflame_repository_error")
		return
	}
	req.YAMLConfig, files, err = repositoryFilesForAnalysis(
		r.Context(), reader, req.Repository.ID, req.Repository.DefaultBranch,
		req.YAMLConfig, files,
	)
	if err != nil {
		integrationError(w, err, "gitflame_files_error")
		return
	}
	if connection != nil {
		_ = s.store.TouchGitFlameConnection(connection.UserID, connection.ID)
	}
	if err := service.ValidateRepositoryFilesForIntegration(files); err != nil {
		problem(w, 422, "validation_error", err.Error())
		return
	}
	if err := s.ensureRequestRepositoryIndex(r, &req.Repository); err != nil {
		integrationError(w, err, "rag_indexing_failed")
		return
	}
	if s.recommender == nil {
		problem(w, http.StatusServiceUnavailable, "recommendation_service_unavailable", "recommendation service client is not configured")
		return
	}
	summary, cards, err := s.recommender.AnalyzeRecommendations(
		r.Context(), req.Repository, req.YAMLConfig, files,
	)
	if err != nil {
		integrationError(w, err, "recommendation_service_error")
		return
	}
	for index := range cards {
		if cards[index].ID == "" {
			cards[index].ID = repository.NewID()
		}
		if cards[index].State == "" {
			cards[index].State = "open"
		}
		if cards[index].Severity == "" {
			cards[index].Severity = "medium"
		}
	}
	report, err := s.store.SaveRecommendations(req.Repository, cfg, summary, cards)
	if err != nil {
		problem(w, 500, "storage_error", err.Error())
		return
	}
	write(w, 200, recommendationReportResponse(report))
}

func recommendationReportResponse(report *domain.RecommendationReport) map[string]any {
	recommendations := report.Recommendations
	if recommendations == nil {
		recommendations = []domain.RecommendationCard{}
	}
	return map[string]any{"repository_id": report.RepositoryID, "status": report.Status, "summary": report.Summary, "recommendations": recommendations}
}
func (s *Server) recommendationStatus(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.Recommendations(repositoryIDFromRequest(r))
	if err != nil {
		resourceError(w, err, "recommendations_not_found", "recommendation report was not found")
		return
	}
	closed := 0
	for _, c := range v.Recommendations {
		if c.State == "closed" {
			closed++
		}
	}
	write(w, 200, map[string]any{"repository_id": v.RepositoryID, "status": v.Status, "total": len(v.Recommendations), "open": len(v.Recommendations) - closed, "closed": closed})
}
func (s *Server) recommendationSummary(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.Recommendations(repositoryIDFromRequest(r))
	if err != nil {
		resourceError(w, err, "recommendations_not_found", "recommendation report was not found")
		return
	}
	write(w, 200, map[string]string{"repository_id": v.RepositoryID, "summary": v.Summary})
}
func (s *Server) recommendations(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.Recommendations(repositoryIDFromRequest(r))
	if err != nil {
		resourceError(w, err, "recommendations_not_found", "recommendation report was not found")
		return
	}
	recommendations := v.Recommendations
	if recommendations == nil {
		recommendations = []domain.RecommendationCard{}
	}
	write(w, 200, map[string]any{"repository_id": v.RepositoryID, "recommendations": recommendations})
}

func repositoryIDFromRequest(r *http.Request) string {
	if id := strings.TrimSpace(r.PathValue("id")); id != "" {
		return id
	}
	return strings.TrimSpace(r.URL.Query().Get("repository_id"))
}
func (s *Server) closeRecommendation(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.CloseRecommendation(r.PathValue("id"))
	if err != nil {
		resourceError(w, err, "recommendation_not_found", "recommendation was not found")
		return
	}
	write(w, 200, v)
}
func (s *Server) deleteRecommendation(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteRecommendation(r.PathValue("id")); err != nil {
		resourceError(w, err, "recommendation_not_found", "recommendation was not found")
		return
	}
	w.WriteHeader(204)
}

func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if d.Decode(&struct{}{}) == nil {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}
func write(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, code, detail string) {
	write(w, status, map[string]any{"status": status, "code": code, "detail": detail})
}
func workflowError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrDispatch) {
		problem(w, http.StatusServiceUnavailable, "queue_unavailable", err.Error())
		return
	}
	if errors.Is(err, repository.ErrNotFound) {
		problem(w, 404, "session_not_found", err.Error())
		return
	}
	problem(w, 422, "invalid_workflow_state", err.Error())
}

func resourceError(w http.ResponseWriter, err error, notFoundCode, notFoundDetail string) {
	if errors.Is(err, repository.ErrNotFound) {
		problem(w, 404, notFoundCode, notFoundDetail)
		return
	}
	problem(w, 500, "storage_error", err.Error())
}
