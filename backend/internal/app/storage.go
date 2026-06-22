package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	statusPlanGenerated       = "plan_generated"
	statusApproved            = "approved"
	statusCorrectionRequested = "correction_requested"
	statusRejected            = "rejected"

	recommendationOpen   = "open"
	recommendationClosed = "closed"
)

type Store interface {
	Close()
	SaveIssueSession(context.Context, IssueAnalyzeRequest, AIConfig, string) (*IssueSession, error)
	GetIssueSession(context.Context, string) (*IssueSession, bool, error)
	UpdateIssueSession(context.Context, *IssueSession) (*IssueSession, error)
	SaveRecommendations(context.Context, RecommendationAnalyzeRequest, AIConfig, string, []RecommendationCard) (*RecommendationReport, error)
	GetRecommendationReport(context.Context, string) (*RecommendationReport, bool, error)
	CloseRecommendation(context.Context, string) (RecommendationCard, bool, error)
	DeleteRecommendation(context.Context, string) (bool, error)
}

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
	RetentionDays   int
	ExpiresAt       time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(databaseURL string) (*PostgresStore, error) {
	const attempts = 20
	const delay = time.Second

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		pool, err := pgxpool.New(ctx, databaseURL)
		if err == nil {
			err = pool.Ping(ctx)
		}
		cancel()

		if err == nil {
			return &PostgresStore{pool: pool}, nil
		}
		if pool != nil {
			pool.Close()
		}

		lastErr = err
		if attempt < attempts {
			time.Sleep(delay)
		}
	}
	return nil, fmt.Errorf("connect PostgreSQL after %d attempts: %w", attempts, lastErr)
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

func (s *PostgresStore) SaveIssueSession(ctx context.Context, request IssueAnalyzeRequest, cfg AIConfig, planMarkdown string) (*IssueSession, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollbackTx(ctx, tx)

	repositoryID, err := upsertRepository(ctx, tx, request.Repository)
	if err != nil {
		return nil, err
	}
	configID, err := insertAIConfig(ctx, tx, repositoryID, cfg)
	if err != nil {
		return nil, err
	}

	var sessionID string
	err = tx.QueryRow(ctx, `
		INSERT INTO issue_sessions (
			repository_id,
			ai_config_id,
			external_issue_id,
			issue_title,
			issue_body,
			issue_author,
			status,
			current_revision,
			updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 1, now())
		ON CONFLICT (repository_id, external_issue_id) DO UPDATE SET
			ai_config_id = EXCLUDED.ai_config_id,
			issue_title = EXCLUDED.issue_title,
			issue_body = EXCLUDED.issue_body,
			issue_author = EXCLUDED.issue_author,
			status = EXCLUDED.status,
			current_revision = EXCLUDED.current_revision,
			git_workflow_json = NULL,
			updated_at = now()
		RETURNING id::text
	`, repositoryID, configID, request.Issue.ID, request.Issue.Title, request.Issue.Body, request.Issue.Author, statusPlanGenerated).Scan(&sessionID)
	if err != nil {
		return nil, err
	}

	var generatedPlanID string
	err = tx.QueryRow(ctx, `
		INSERT INTO generated_plans (
			issue_session_id,
			plan_markdown,
			current_revision,
			updated_at
		) VALUES ($1, $2, 1, now())
		ON CONFLICT (issue_session_id) DO UPDATE SET
			plan_markdown = EXCLUDED.plan_markdown,
			current_revision = EXCLUDED.current_revision,
			updated_at = now()
		RETURNING id::text
	`, sessionID, planMarkdown).Scan(&generatedPlanID)
	if err != nil {
		return nil, err
	}

	taskID, err := insertAgentTask(ctx, tx, sessionID, generatedPlanID, "initial_plan")
	if err != nil {
		return nil, err
	}
	if err := updateAgentTaskStatus(ctx, tx, taskID, "processing", "", "Initial plan generation started."); err != nil {
		return nil, err
	}
	if err := updateAgentTaskStatus(ctx, tx, taskID, "completed", "", "Initial plan generated and stored."); err != nil {
		return nil, err
	}
	if err := upsertPlanRevision(ctx, tx, sessionID, generatedPlanID, taskID, 1, planMarkdown, "", "initial"); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.getIssueSession(ctx, request.Issue.ID)
}

func (s *PostgresStore) GetIssueSession(ctx context.Context, issueOrSessionID string) (*IssueSession, bool, error) {
	session, err := s.getIssueSession(ctx, issueOrSessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return session, true, nil
}

func (s *PostgresStore) UpdateIssueSession(ctx context.Context, session *IssueSession) (*IssueSession, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollbackTx(ctx, tx)

	workflowJSON, err := jsonOrNil(session.GitWorkflow)
	if err != nil {
		return nil, err
	}

	var sessionID string
	err = tx.QueryRow(ctx, `
		UPDATE issue_sessions
		SET status = $2,
			current_revision = $3,
			git_workflow_json = $4::jsonb,
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text
	`, session.SessionID, session.Status, session.Revision, workflowJSON).Scan(&sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("issue session %s was not found", session.SessionID)
	}
	if err != nil {
		return nil, err
	}

	var generatedPlanID string
	err = tx.QueryRow(ctx, `
		UPDATE generated_plans
		SET plan_markdown = $2,
			current_revision = $3,
			updated_at = now()
		WHERE issue_session_id = $1::uuid
		RETURNING id::text
	`, sessionID, session.PlanMarkdown, session.Revision).Scan(&generatedPlanID)
	if err != nil {
		return nil, err
	}

	switch session.Status {
	case statusCorrectionRequested:
		feedback := latestFeedback(session.FeedbackHistory)
		taskID, err := insertAgentTask(ctx, tx, sessionID, generatedPlanID, "plan_revision")
		if err != nil {
			return nil, err
		}
		if err := updateAgentTaskStatus(ctx, tx, taskID, "processing", "", "Plan revision generation started."); err != nil {
			return nil, err
		}
		if err := updateAgentTaskStatus(ctx, tx, taskID, "completed", "", "Plan revision generated after user correction feedback."); err != nil {
			return nil, err
		}
		if err := upsertPlanRevision(ctx, tx, sessionID, generatedPlanID, taskID, session.Revision, session.PlanMarkdown, feedback, "correction"); err != nil {
			return nil, err
		}
		if err := insertUserResponse(ctx, tx, sessionID, "correct", feedback, session.Request.Issue.Author); err != nil {
			return nil, err
		}
	case statusApproved:
		if err := insertUserResponse(ctx, tx, sessionID, "approve", "", session.Request.Issue.Author); err != nil {
			return nil, err
		}
	case statusRejected:
		if err := insertUserResponse(ctx, tx, sessionID, "reject", "", session.Request.Issue.Author); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.getIssueSession(ctx, session.SessionID)
}

func (s *PostgresStore) SaveRecommendations(ctx context.Context, request RecommendationAnalyzeRequest, cfg AIConfig, summary string, cards []RecommendationCard) (*RecommendationReport, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollbackTx(ctx, tx)

	repositoryID, err := upsertRepository(ctx, tx, request.Repository)
	if err != nil {
		return nil, err
	}
	configID, err := insertAIConfig(ctx, tx, repositoryID, cfg)
	if err != nil {
		return nil, err
	}

	var runID string
	err = tx.QueryRow(ctx, `
		INSERT INTO recommendation_runs (
			repository_id,
			ai_config_id,
			summary,
			status,
			retention_days,
			expires_at,
			updated_at
		) VALUES ($1, $2, $3, 'completed', $4, now() + ($4::text || ' days')::interval, now())
		RETURNING id::text
	`, repositoryID, configID, summary, cfg.RetentionDays).Scan(&runID)
	if err != nil {
		return nil, err
	}

	for _, card := range cards {
		if card.ID == "" {
			card.ID = newID()
		}
		if card.State == "" {
			card.State = recommendationOpen
		}
		if card.Severity == "" {
			card.Severity = "medium"
		}
		if err := insertRecommendationCard(ctx, tx, runID, card); err != nil {
			return nil, err
		}
	}

	taskID, err := insertAgentTask(ctx, tx, "", "", "recommendation_analysis")
	if err != nil {
		return nil, err
	}
	if err := updateAgentTaskStatus(ctx, tx, taskID, "processing", "", "Recommendation analysis persistence started."); err != nil {
		return nil, err
	}
	if err := updateAgentTaskStatus(ctx, tx, taskID, "completed", "", "Recommendation summary and cards stored."); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.getRecommendationReport(ctx, request.Repository.ID)
}

func (s *PostgresStore) GetRecommendationReport(ctx context.Context, repositoryID string) (*RecommendationReport, bool, error) {
	report, err := s.getRecommendationReport(ctx, repositoryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return report, true, nil
}

func (s *PostgresStore) CloseRecommendation(ctx context.Context, recommendationID string) (RecommendationCard, bool, error) {
	card, err := s.updateRecommendationStatus(ctx, recommendationID, recommendationClosed)
	if errors.Is(err, pgx.ErrNoRows) {
		return RecommendationCard{}, false, nil
	}
	if err != nil {
		return RecommendationCard{}, false, err
	}
	return card, true, nil
}

func (s *PostgresStore) DeleteRecommendation(ctx context.Context, recommendationID string) (bool, error) {
	_, err := s.updateRecommendationStatus(ctx, recommendationID, "deleted")
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *PostgresStore) getIssueSession(ctx context.Context, issueOrSessionID string) (*IssueSession, error) {
	var session IssueSession
	var rawConfig string
	var workflowText string
	err := s.pool.QueryRow(ctx, `
		SELECT
			s.id::text,
			r.external_id,
			r.name,
			r.default_branch,
			r.web_url,
			s.external_issue_id,
			s.issue_title,
			s.issue_body,
			s.issue_author,
			ac.raw_yml,
			COALESCE(gp.plan_markdown, ''),
			s.status,
			s.current_revision,
			COALESCE(s.git_workflow_json::text, ''),
			s.created_at,
			s.updated_at
		FROM issue_sessions s
		JOIN repositories r ON r.id = s.repository_id
		JOIN ai_configs ac ON ac.id = s.ai_config_id
		LEFT JOIN generated_plans gp ON gp.issue_session_id = s.id
		WHERE s.id::text = $1 OR s.external_issue_id = $1
		ORDER BY s.updated_at DESC
		LIMIT 1
	`, issueOrSessionID).Scan(
		&session.SessionID,
		&session.Request.Repository.ID,
		&session.Request.Repository.Name,
		&session.Request.Repository.DefaultBranch,
		&session.Request.Repository.WebURL,
		&session.Request.Issue.ID,
		&session.Request.Issue.Title,
		&session.Request.Issue.Body,
		&session.Request.Issue.Author,
		&rawConfig,
		&session.PlanMarkdown,
		&session.Status,
		&session.Revision,
		&workflowText,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	cfg, err := ParseAIConfig(rawConfig)
	if err != nil {
		return nil, err
	}
	session.Config = cfg
	session.Request.YAMLConfig = rawConfig

	if workflowText != "" {
		var workflow GitWorkflowResponse
		if err := json.Unmarshal([]byte(workflowText), &workflow); err == nil {
			session.GitWorkflow = &workflow
		}
	}

	rows, err := s.pool.Query(ctx, `
		SELECT correction_feedback
		FROM plan_revisions
		WHERE issue_session_id = $1::uuid
			AND correction_feedback <> ''
		ORDER BY revision_number
	`, session.SessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var feedback string
		if err := rows.Scan(&feedback); err != nil {
			return nil, err
		}
		session.FeedbackHistory = append(session.FeedbackHistory, feedback)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *PostgresStore) getRecommendationReport(ctx context.Context, repositoryExternalID string) (*RecommendationReport, error) {
	var report RecommendationReport
	var runID string
	err := s.pool.QueryRow(ctx, `
		SELECT
			rr.id::text,
			r.external_id,
			rr.summary,
			rr.status,
			rr.retention_days,
			rr.expires_at,
			rr.created_at,
			rr.updated_at
		FROM recommendation_runs rr
		JOIN repositories r ON r.id = rr.repository_id
		WHERE r.external_id = $1
			AND rr.expires_at > now()
		ORDER BY rr.created_at DESC
		LIMIT 1
	`, repositoryExternalID).Scan(&runID, &report.RepositoryID, &report.Summary, &report.Status, &report.RetentionDays, &report.ExpiresAt, &report.CreatedAt, &report.UpdatedAt)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			id::text,
			severity,
			file_path,
			line_number,
			problem,
			suggestion,
			confidence,
			current_status
		FROM recommendations
		WHERE recommendation_run_id = $1::uuid
			AND current_status <> 'deleted'
		ORDER BY created_at, id
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var card RecommendationCard
		var line sql.NullInt64
		var confidence sql.NullFloat64
		if err := rows.Scan(&card.ID, &card.Severity, &card.File, &line, &card.Problem, &card.Suggestion, &confidence, &card.State); err != nil {
			return nil, err
		}
		card.Line = nullableInt(line)
		card.Confidence = nullableFloat(confidence)
		report.Recommendations = append(report.Recommendations, card)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &report, nil
}

func (s *PostgresStore) updateRecommendationStatus(ctx context.Context, recommendationID string, status string) (RecommendationCard, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RecommendationCard{}, err
	}
	defer rollbackTx(ctx, tx)

	var card RecommendationCard
	var line sql.NullInt64
	var confidence sql.NullFloat64
	err = tx.QueryRow(ctx, `
		UPDATE recommendations
		SET current_status = $2,
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, severity, file_path, line_number, problem, suggestion, confidence, current_status
	`, recommendationID, status).Scan(&card.ID, &card.Severity, &card.File, &line, &card.Problem, &card.Suggestion, &confidence, &card.State)
	if err != nil {
		return RecommendationCard{}, err
	}
	card.Line = nullableInt(line)
	card.Confidence = nullableFloat(confidence)

	_, err = tx.Exec(ctx, `
		INSERT INTO recommendation_statuses (
			recommendation_id,
			status,
			changed_by
		) VALUES ($1, $2, 'system')
	`, recommendationID, status)
	if err != nil {
		return RecommendationCard{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RecommendationCard{}, err
	}
	return card, nil
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

func (s *MemoryStore) Close() {}

func (s *MemoryStore) SaveIssueSession(_ context.Context, request IssueAnalyzeRequest, cfg AIConfig, planMarkdown string) (*IssueSession, error) {
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
	return cloneIssueSession(session), nil
}

func (s *MemoryStore) GetIssueSession(_ context.Context, issueOrSessionID string) (*IssueSession, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if session, ok := s.issueSessions[issueOrSessionID]; ok {
		return cloneIssueSession(session), true, nil
	}
	sessionID, ok := s.issueIndex[issueOrSessionID]
	if !ok {
		return nil, false, nil
	}
	session, ok := s.issueSessions[sessionID]
	if !ok {
		return nil, false, nil
	}
	return cloneIssueSession(session), true, nil
}

func (s *MemoryStore) UpdateIssueSession(_ context.Context, session *IssueSession) (*IssueSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session.UpdatedAt = time.Now().UTC()
	s.issueSessions[session.SessionID] = cloneIssueSession(session)
	s.issueIndex[session.Request.Issue.ID] = session.SessionID
	return cloneIssueSession(session), nil
}

func (s *MemoryStore) SaveRecommendations(_ context.Context, request RecommendationAnalyzeRequest, cfg AIConfig, summary string, cards []RecommendationCard) (*RecommendationReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	report := &RecommendationReport{
		RepositoryID:    request.Repository.ID,
		Summary:         summary,
		Recommendations: append([]RecommendationCard(nil), cards...),
		Status:          "ready",
		RetentionDays:   cfg.RetentionDays,
		ExpiresAt:       now.Add(time.Duration(cfg.RetentionDays) * 24 * time.Hour),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.recommendationReports[request.Repository.ID] = report
	return cloneRecommendationReport(report), nil
}

func (s *MemoryStore) GetRecommendationReport(_ context.Context, repositoryID string) (*RecommendationReport, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	report, ok := s.recommendationReports[repositoryID]
	if !ok {
		return nil, false, nil
	}
	return cloneRecommendationReport(report), true, nil
}

func (s *MemoryStore) CloseRecommendation(_ context.Context, recommendationID string) (RecommendationCard, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, report := range s.recommendationReports {
		for i := range report.Recommendations {
			if report.Recommendations[i].ID == recommendationID {
				report.Recommendations[i].State = recommendationClosed
				report.UpdatedAt = time.Now().UTC()
				return report.Recommendations[i], true, nil
			}
		}
	}
	return RecommendationCard{}, false, nil
}

func (s *MemoryStore) DeleteRecommendation(_ context.Context, recommendationID string) (bool, error) {
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
			return true, nil
		}
	}
	return false, nil
}

func upsertRepository(ctx context.Context, tx pgx.Tx, repository RepositoryMetadata) (string, error) {
	name := strings.TrimSpace(repository.Name)
	if name == "" {
		name = repository.ID
	}
	defaultBranch := strings.TrimSpace(repository.DefaultBranch)
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	var repositoryID string
	err := tx.QueryRow(ctx, `
		INSERT INTO repositories (
			external_id,
			name,
			owner,
			default_branch,
			web_url,
			updated_at
		) VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (external_id) DO UPDATE SET
			name = EXCLUDED.name,
			owner = EXCLUDED.owner,
			default_branch = EXCLUDED.default_branch,
			web_url = EXCLUDED.web_url,
			updated_at = now()
		RETURNING id::text
	`, repository.ID, name, ownerFromWebURL(repository.WebURL), defaultBranch, repository.WebURL).Scan(&repositoryID)
	return repositoryID, err
}

func insertAIConfig(ctx context.Context, tx pgx.Tx, repositoryID string, cfg AIConfig) (string, error) {
	parsed, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}

	var configID string
	err = tx.QueryRow(ctx, `
		INSERT INTO ai_configs (
			repository_id,
			raw_yml,
			parsed_config_json,
			is_valid,
			retention_days
		) VALUES ($1, $2, $3::jsonb, true, $4)
		RETURNING id::text
	`, repositoryID, cfg.Raw, string(parsed), cfg.RetentionDays).Scan(&configID)
	return configID, err
}

func insertAgentTask(ctx context.Context, tx pgx.Tx, sessionID string, generatedPlanID string, taskType string) (string, error) {
	var sessionValue any
	var planValue any
	if sessionID != "" {
		sessionValue = sessionID
	}
	if generatedPlanID != "" {
		planValue = generatedPlanID
	}

	var taskID string
	err := tx.QueryRow(ctx, `
		INSERT INTO agent_tasks (
			issue_session_id,
			generated_plan_id,
			task_type,
			status,
			updated_at
		) VALUES ($1, $2, $3, 'queued', now())
		RETURNING id::text
	`, sessionValue, planValue, taskType).Scan(&taskID)
	return taskID, err
}

func updateAgentTaskStatus(ctx context.Context, tx pgx.Tx, taskID string, status string, errorMessage string, toolSummary string) error {
	_, err := tx.Exec(ctx, `
		UPDATE agent_tasks
		SET status = $2,
			error_message = $3,
			tool_execution_summary = $4,
			started_at = CASE
				WHEN $2 = 'processing' AND started_at IS NULL THEN now()
				ELSE started_at
			END,
			completed_at = CASE
				WHEN $2 IN ('completed', 'failed') THEN now()
				ELSE completed_at
			END,
			updated_at = now()
		WHERE id = $1::uuid
	`, taskID, status, errorMessage, toolSummary)
	return err
}

func upsertPlanRevision(ctx context.Context, tx pgx.Tx, sessionID string, generatedPlanID string, taskID string, revision int, planMarkdown string, feedback string, source string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO plan_revisions (
			generated_plan_id,
			issue_session_id,
			agent_task_id,
			revision_number,
			plan_markdown,
			correction_feedback,
			source
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (generated_plan_id, revision_number) DO UPDATE SET
			agent_task_id = EXCLUDED.agent_task_id,
			plan_markdown = EXCLUDED.plan_markdown,
			correction_feedback = EXCLUDED.correction_feedback,
			source = EXCLUDED.source
	`, generatedPlanID, sessionID, taskID, revision, planMarkdown, feedback, source)
	return err
}

func insertUserResponse(ctx context.Context, tx pgx.Tx, sessionID string, responseType string, message string, author string) error {
	if strings.TrimSpace(author) == "" {
		author = "system"
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO user_responses (
			issue_session_id,
			response_type,
			message,
			author
		) VALUES ($1, $2, $3, $4)
	`, sessionID, responseType, message, author)
	return err
}

func insertRecommendationCard(ctx context.Context, tx pgx.Tx, runID string, card RecommendationCard) error {
	var line any
	if card.Line != nil {
		line = *card.Line
	}
	var confidence any
	if card.Confidence != nil {
		confidence = *card.Confidence
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO recommendations (
			id,
			recommendation_run_id,
			file_path,
			line_number,
			category,
			severity,
			problem,
			suggestion,
			confidence,
			current_status,
			updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
	`, card.ID, runID, card.File, line, "code_quality", card.Severity, card.Problem, card.Suggestion, confidence, card.State)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO recommendation_statuses (
			recommendation_id,
			status,
			changed_by
		) VALUES ($1, $2, 'system')
	`, card.ID, card.State)
	return err
}

func rollbackTx(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}

func jsonOrNil(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(raw), nil
}

func latestFeedback(history []string) string {
	if len(history) == 0 {
		return ""
	}
	return history[len(history)-1]
}

func nullableInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}

func nullableFloat(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func ownerFromWebURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Path == "" {
		return "gitflame"
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "gitflame"
	}
	return parts[0]
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
