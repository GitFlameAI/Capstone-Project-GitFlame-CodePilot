package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitflame-codepilot/backend/internal/domain"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	store := &PostgresStore{pool: pool}
	if err := store.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgresStore) Close() { s.pool.Close() }

func (s *PostgresStore) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return err
	}
	var ready bool
	if err := s.pool.QueryRow(ctx, `SELECT to_regclass('public.agent_tasks') IS NOT NULL`).Scan(&ready); err != nil {
		return err
	}
	if !ready {
		return errors.New("database migrations are not applied")
	}
	return nil
}

func (s *PostgresStore) CreateSession(req domain.IssueAnalyzeRequest, cfg domain.AIConfig) (*domain.IssueSession, bool, error) {
	if existing, err := s.sessionByRepositoryIssue(req.Repository.ID, req.Issue.ID); err == nil {
		return existing, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	repositoryID, err := upsertRepository(ctx, tx, req.Repository)
	if err != nil {
		return nil, false, err
	}
	configID, err := insertAIConfig(ctx, tx, repositoryID, cfg)
	if err != nil {
		return nil, false, err
	}
	if err := upsertRepositoryFiles(ctx, tx, repositoryID, req); err != nil {
		return nil, false, err
	}
	requestJSON, err := json.Marshal(req)
	if err != nil {
		return nil, false, err
	}
	sessionID := NewID()
	command, err := tx.Exec(ctx, `
		INSERT INTO issue_sessions (
			id,repository_id,ai_config_id,external_issue_id,issue_title,issue_body,
			issue_author,status,current_revision,request_json,updated_at
		) VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,0,$9::jsonb,now())
		ON CONFLICT (repository_id,external_issue_id) DO NOTHING`,
		sessionID, repositoryID, configID, req.Issue.ID, req.Issue.Title, req.Issue.Body,
		req.Issue.Author, domain.SessionGenerating, string(requestJSON))
	if err != nil {
		return nil, false, err
	}
	if command.RowsAffected() == 0 {
		_ = tx.Rollback(ctx)
		existing, err := s.sessionByRepositoryIssue(req.Repository.ID, req.Issue.ID)
		return existing, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	created, err := s.Session(sessionID)
	return created, true, err
}

func (s *PostgresStore) Session(id string) (*domain.IssueSession, error) {
	return s.scanSession(s.pool.QueryRow(context.Background(), sessionSelect+`
		WHERE s.id::text=$1 OR s.external_issue_id=$1 ORDER BY s.updated_at DESC LIMIT 1`, id))
}

func (s *PostgresStore) sessionByRepositoryIssue(repositoryID, issueID string) (*domain.IssueSession, error) {
	return s.scanSession(s.pool.QueryRow(context.Background(), sessionSelect+`
		WHERE r.external_id=$1 AND s.external_issue_id=$2 LIMIT 1`, repositoryID, issueID))
}

const sessionSelect = `
	SELECT s.id::text,s.external_issue_id,s.issue_title,s.issue_body,s.issue_author,
	       s.status,s.current_revision,s.git_workflow_json::text,s.request_json::text,
	       s.created_at,s.updated_at,r.external_id,r.name,r.default_branch,r.web_url,
	       c.raw_yml,c.parsed_config_json::text,gp.id::text,gp.plan_markdown
	FROM issue_sessions s
	JOIN repositories r ON r.id=s.repository_id
	JOIN ai_configs c ON c.id=s.ai_config_id
	LEFT JOIN generated_plans gp ON gp.issue_session_id=s.id `

func (s *PostgresStore) scanSession(row pgx.Row) (*domain.IssueSession, error) {
	var session domain.IssueSession
	var issueID, title, body, author string
	var repository domain.RepositoryMetadata
	var rawYAML string
	var workflowJSON, requestJSON, configJSON, planID, planMarkdown pgtype.Text
	err := row.Scan(&session.ID, &issueID, &title, &body, &author, &session.Status, &session.Revision,
		&workflowJSON, &requestJSON, &session.CreatedAt, &session.UpdatedAt, &repository.ID,
		&repository.Name, &repository.DefaultBranch, &repository.WebURL, &rawYAML, &configJSON,
		&planID, &planMarkdown)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if requestJSON.Valid {
		_ = json.Unmarshal([]byte(requestJSON.String), &session.Request)
	}
	if session.Request.Issue.ID == "" {
		session.Request.Repository = repository
		session.Request.Issue = domain.IssuePayload{ID: issueID, Title: title, Body: body, Author: author}
	}
	if configJSON.Valid {
		_ = json.Unmarshal([]byte(configJSON.String), &session.Config)
	}
	session.Config.Raw = rawYAML
	if planMarkdown.Valid {
		session.PlanMarkdown = planMarkdown.String
	}
	if workflowJSON.Valid && workflowJSON.String != "null" {
		_ = json.Unmarshal([]byte(workflowJSON.String), &session.GeneratedFiles)
	}
	if err := s.loadGeneratedFiles(context.Background(), &session); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(context.Background(), `
		SELECT correction_feedback FROM plan_revisions
		WHERE issue_session_id=$1::uuid AND correction_feedback<>'' ORDER BY revision_number`, session.ID)
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
	return &session, rows.Err()
}

func (s *PostgresStore) UpdateSession(session *domain.IssueSession) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var workflow any
	if session.GeneratedFiles != nil {
		encoded, _ := json.Marshal(session.GeneratedFiles)
		workflow = string(encoded)
	}
	command, err := tx.Exec(ctx, `
		UPDATE issue_sessions SET status=$2,current_revision=$3,
		git_workflow_json=$4::jsonb,updated_at=now() WHERE id=$1::uuid`,
		session.ID, session.Status, session.Revision, workflow)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	if session.GeneratedFiles != nil {
		if err := saveGeneratedFiles(ctx, tx, session.ID, session.GeneratedFiles); err != nil {
			return err
		}
	}
	switch session.Status {
	case domain.SessionCorrectionRequested:
		if err := insertUserResponse(ctx, tx, session.ID, "correct", latestFeedback(session.FeedbackHistory), session.Request.Issue.Author); err != nil {
			return err
		}
	case domain.SessionApproved:
		if err := insertUserResponse(ctx, tx, session.ID, "approve", "", session.Request.Issue.Author); err != nil {
			return err
		}
	case domain.SessionRejected:
		if err := insertUserResponse(ctx, tx, session.ID, "reject", "", session.Request.Issue.Author); err != nil {
			return err
		}
	}
	if session.Status == domain.SessionPlanGenerated && session.Revision > 0 && strings.TrimSpace(session.PlanMarkdown) != "" {
		var generatedPlanID string
		err = tx.QueryRow(ctx, `
			INSERT INTO generated_plans (issue_session_id,plan_markdown,current_revision,updated_at)
			VALUES ($1::uuid,$2,$3,now())
			ON CONFLICT (issue_session_id) DO UPDATE SET
			plan_markdown=EXCLUDED.plan_markdown,current_revision=EXCLUDED.current_revision,updated_at=now()
			RETURNING id::text`, session.ID, session.PlanMarkdown, session.Revision).Scan(&generatedPlanID)
		if err != nil {
			return err
		}
		var taskID, taskType string
		var attempt int
		err = tx.QueryRow(ctx, `SELECT id::text,task_type,attempt FROM agent_tasks
			WHERE issue_session_id=$1::uuid ORDER BY created_at DESC LIMIT 1`, session.ID).Scan(&taskID, &taskType, &attempt)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		source := "initial"
		if attempt > 1 {
			source = "retry"
		} else if taskType == "plan_revision" {
			source = "correction"
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO plan_revisions (generated_plan_id,issue_session_id,agent_task_id,revision_number,plan_markdown,correction_feedback,source)
			VALUES ($1::uuid,$2::uuid,NULLIF($3::text,'')::uuid,$4,$5,$6,$7)
			ON CONFLICT (generated_plan_id,revision_number) DO UPDATE SET
			plan_markdown=EXCLUDED.plan_markdown,correction_feedback=EXCLUDED.correction_feedback,
			agent_task_id=EXCLUDED.agent_task_id,source=EXCLUDED.source`,
			generatedPlanID, session.ID, taskID, session.Revision, session.PlanMarkdown, latestFeedback(session.FeedbackHistory), source)
		if err != nil {
			return err
		}
		if taskID != "" {
			_, err = tx.Exec(ctx, `UPDATE agent_tasks SET generated_plan_id=$2::uuid,updated_at=now() WHERE id=$1::uuid`, taskID, generatedPlanID)
			if err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) loadGeneratedFiles(ctx context.Context, session *domain.IssueSession) error {
	var contract domain.GeneratedFilesContract
	var taskID pgtype.Text
	var payloadStatus string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(agent_task_id::text,''),branch_name,base_branch,commit_message,pr_title,reviewer,status
		FROM git_workflow_payloads WHERE issue_session_id=$1::uuid`, session.ID).
		Scan(&taskID, &contract.BranchName, &contract.BaseBranch, &contract.CommitMessage, &contract.PRTitle, &contract.Reviewer, &payloadStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if taskID.Valid {
		contract.TaskID = taskID.String
	}
	rows, err := s.pool.Query(ctx, `
		SELECT file_path,action,content,diff,explanation,status,validation_error
		FROM generated_files WHERE issue_session_id=$1::uuid ORDER BY created_at,id`, session.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var file domain.GeneratedFileOperation
		if err := rows.Scan(&file.Path, &file.Action, &file.Content, &file.Diff, &file.Explanation, &file.Status, &file.ValidationError); err != nil {
			return err
		}
		contract.Files = append(contract.Files, file)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if session.GeneratedFiles != nil {
		contract.RequestID = session.GeneratedFiles.RequestID
		contract.Summary = session.GeneratedFiles.Summary
		contract.ApplyStatus = session.GeneratedFiles.ApplyStatus
		contract.CommitSHA = session.GeneratedFiles.CommitSHA
		contract.PullRequestID = session.GeneratedFiles.PullRequestID
		contract.PullRequestURL = session.GeneratedFiles.PullRequestURL
		contract.ApplyError = session.GeneratedFiles.ApplyError
		contract.AppliedAt = session.GeneratedFiles.AppliedAt
		if contract.TaskID == "" {
			contract.TaskID = session.GeneratedFiles.TaskID
		}
	}
	session.GeneratedFiles = &contract
	return nil
}

func (s *PostgresStore) CreateTask(sessionID, issueID, taskType string) (*domain.AgentTask, error) {
	task := &domain.AgentTask{ID: NewID(), SessionID: sessionID, IssueID: issueID, Type: taskType, Status: domain.TaskQueued, Attempt: 1}
	err := s.pool.QueryRow(context.Background(), `
		INSERT INTO agent_tasks (id,issue_session_id,task_type,status,attempt)
		VALUES ($1::uuid,$2::uuid,$3,$4,1) RETURNING created_at,updated_at`,
		task.ID, sessionID, taskType, task.Status).Scan(&task.CreatedAt, &task.UpdatedAt)
	return task, err
}

func (s *PostgresStore) Task(id string) (*domain.AgentTask, error) {
	return scanTask(s.pool.QueryRow(context.Background(), taskSelect+` WHERE t.id::text=$1`, id))
}
func (s *PostgresStore) LatestTask(sessionID string) (*domain.AgentTask, error) {
	return scanTask(s.pool.QueryRow(context.Background(), taskSelect+` WHERE t.issue_session_id=$1::uuid ORDER BY t.created_at DESC LIMIT 1`, sessionID))
}

const taskSelect = `SELECT t.id::text,t.issue_session_id::text,t.generated_plan_id::text,
	s.external_issue_id,t.task_type,t.status,t.attempt,COALESCE(gp.plan_markdown,''),
	t.error_message,t.error_json::text,t.relevant_files::text,t.model,t.usage_json::text,
	t.tool_execution_summary,t.created_at,t.updated_at
	FROM agent_tasks t LEFT JOIN issue_sessions s ON s.id=t.issue_session_id
	LEFT JOIN generated_plans gp ON gp.id=t.generated_plan_id `

func scanTask(row pgx.Row) (*domain.AgentTask, error) {
	var task domain.AgentTask
	var sessionID, planID, issueID, errorJSON, relevantJSON, usageJSON pgtype.Text
	var errorMessage string
	err := row.Scan(&task.ID, &sessionID, &planID, &issueID, &task.Type, &task.Status, &task.Attempt,
		&task.PlanMarkdown, &errorMessage, &errorJSON, &relevantJSON, &task.Model, &usageJSON,
		&task.ToolExecutionSummary, &task.CreatedAt, &task.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if sessionID.Valid {
		task.SessionID = sessionID.String
	}
	if planID.Valid {
		task.GeneratedPlanID = planID.String
	}
	if issueID.Valid {
		task.IssueID = issueID.String
	}
	if errorJSON.Valid && errorJSON.String != "null" {
		_ = json.Unmarshal([]byte(errorJSON.String), &task.Error)
	} else if errorMessage != "" {
		task.Error = &domain.TaskError{HTTPStatus: 502, Code: "agent_engine_error", Detail: errorMessage}
	}
	if relevantJSON.Valid {
		_ = json.Unmarshal([]byte(relevantJSON.String), &task.RelevantFiles)
	}
	if usageJSON.Valid {
		_ = json.Unmarshal([]byte(usageJSON.String), &task.Usage)
	}
	return &task, nil
}

func (s *PostgresStore) UpdateTask(task *domain.AgentTask) error {
	errorJSON, _ := json.Marshal(task.Error)
	relevantJSON, _ := json.Marshal(task.RelevantFiles)
	usageJSON, _ := json.Marshal(task.Usage)
	errorMessage := ""
	if task.Error != nil {
		errorMessage = task.Error.Detail
	}
	command, err := s.pool.Exec(context.Background(), `
		UPDATE agent_tasks SET status=$2,attempt=$3,error_message=$4,error_json=$5::jsonb,
		relevant_files=$6::jsonb,model=$7,usage_json=$8::jsonb,tool_execution_summary=$9,
		started_at=CASE WHEN $2='processing' THEN COALESCE(started_at,now()) ELSE started_at END,
		completed_at=CASE WHEN $2 IN ('completed','failed') THEN now() ELSE completed_at END,
		updated_at=now() WHERE id=$1::uuid`, task.ID, task.Status, task.Attempt, errorMessage,
		string(errorJSON), string(relevantJSON), task.Model, string(usageJSON), task.ToolExecutionSummary)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) SaveRecommendations(repository domain.RepositoryMetadata, cfg domain.AIConfig, summary string, cards []domain.RecommendationCard) (*domain.RecommendationReport, error) {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	repositoryID, err := upsertRepository(ctx, tx, repository)
	if err != nil {
		return nil, err
	}
	configID, err := insertAIConfig(ctx, tx, repositoryID, cfg)
	if err != nil {
		return nil, err
	}
	var runID string
	err = tx.QueryRow(ctx, `INSERT INTO recommendation_runs
		(repository_id,ai_config_id,summary,status,retention_days,expires_at,updated_at)
		VALUES ($1::uuid,$2::uuid,$3,'completed',$4::int,now()+make_interval(days => $5::int),now()) RETURNING id::text`,
		repositoryID, configID, summary, cfg.RetentionDays, cfg.RetentionDays).Scan(&runID)
	if err != nil {
		return nil, err
	}
	for index := range cards {
		if cards[index].ID == "" {
			cards[index].ID = NewID()
		}
		if cards[index].State == "" {
			cards[index].State = "open"
		}
		if cards[index].Severity == "" {
			cards[index].Severity = "medium"
		}
		_, err = tx.Exec(ctx, `INSERT INTO recommendations
			(id,recommendation_run_id,file_path,line_number,category,severity,problem,suggestion,confidence,current_status,updated_at)
			VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,now())`, cards[index].ID, runID, cards[index].File, cards[index].Line, recommendationCategory(cards[index].Category), cards[index].Severity, cards[index].Problem, cards[index].Suggestion, cards[index].Confidence, cards[index].State)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &domain.RecommendationReport{RepositoryID: repository.ID, Summary: summary, Status: "ready", Recommendations: cards}, nil
}

func (s *PostgresStore) Recommendations(repositoryID string) (*domain.RecommendationReport, error) {
	var runID, summary, status string
	err := s.pool.QueryRow(context.Background(), `SELECT rr.id::text,rr.summary,rr.status FROM recommendation_runs rr
		JOIN repositories r ON r.id=rr.repository_id WHERE r.external_id=$1 AND rr.expires_at>now()
		ORDER BY rr.created_at DESC LIMIT 1`, repositoryID).Scan(&runID, &summary, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(context.Background(), `SELECT id::text,severity,category,file_path,line_number,problem,suggestion,confidence,current_status
		FROM recommendations WHERE recommendation_run_id=$1::uuid AND current_status<>'deleted' ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cards []domain.RecommendationCard
	for rows.Next() {
		card, err := scanRecommendation(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if status == "completed" {
		status = "ready"
	}
	return &domain.RecommendationReport{RepositoryID: repositoryID, Summary: summary, Status: status, Recommendations: cards}, nil
}

func (s *PostgresStore) CloseRecommendation(id string) (domain.RecommendationCard, error) {
	return s.updateRecommendation(id, "closed")
}
func (s *PostgresStore) DeleteRecommendation(id string) error {
	_, err := s.updateRecommendation(id, "deleted")
	return err
}

func (s *PostgresStore) updateRecommendation(id, status string) (domain.RecommendationCard, error) {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.RecommendationCard{}, err
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `UPDATE recommendations SET current_status=$2,updated_at=now() WHERE id::text=$1
		RETURNING id::text,severity,category,file_path,line_number,problem,suggestion,confidence,current_status`, id, status)
	card, err := scanRecommendation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return card, ErrNotFound
	}
	if err != nil {
		return card, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO recommendation_statuses (recommendation_id,status,changed_by) VALUES ($1::uuid,$2,'api')`, id, status)
	if err != nil {
		return card, err
	}
	return card, tx.Commit(ctx)
}

type rowScanner interface{ Scan(...any) error }

func scanRecommendation(row rowScanner) (domain.RecommendationCard, error) {
	var card domain.RecommendationCard
	var line pgtype.Int4
	var confidence pgtype.Float8
	err := row.Scan(&card.ID, &card.Severity, &card.Category, &card.File, &line, &card.Problem, &card.Suggestion, &confidence, &card.State)
	if line.Valid {
		value := int(line.Int32)
		card.Line = &value
	}
	if confidence.Valid {
		value := confidence.Float64
		card.Confidence = &value
	}
	return card, err
}

func recommendationCategory(value string) string {
	if strings.TrimSpace(value) == "" {
		return "general"
	}
	return value
}

func upsertRepository(ctx context.Context, tx pgx.Tx, repository domain.RepositoryMetadata) (string, error) {
	name := repository.Name
	if strings.TrimSpace(name) == "" {
		name = repository.ID
	}
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO repositories (external_id,name,owner,default_branch,web_url,updated_at)
		VALUES ($1,$2,'gitflame',$3,$4,now()) ON CONFLICT (external_id) DO UPDATE SET
		name=EXCLUDED.name,default_branch=EXCLUDED.default_branch,web_url=EXCLUDED.web_url,updated_at=now()
		RETURNING id::text`, repository.ID, name, repository.DefaultBranch, repository.WebURL).Scan(&id)
	return id, err
}

func upsertRepositoryFiles(ctx context.Context, tx pgx.Tx, repositoryID string, req domain.IssueAnalyzeRequest) error {
	files := append([]domain.RepositoryFile(nil), req.RepositoryFiles...)
	if len(files) == 0 {
		for _, filePath := range req.RepositoryContext {
			files = append(files, domain.RepositoryFile{Path: filePath})
		}
	}
	for _, file := range files {
		filePath := strings.TrimSpace(file.Path)
		if filePath == "" {
			continue
		}
		hash := ""
		if file.Content != "" {
			sum := sha256.Sum256([]byte(file.Content))
			hash = fmt.Sprintf("%x", sum)
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO repository_files (repository_id,file_path,file_name,content_hash,commit_sha,updated_at)
			VALUES ($1::uuid,$2,$3,$4,$5,now())
			ON CONFLICT (repository_id,file_path) DO UPDATE SET
			file_name=EXCLUDED.file_name,content_hash=EXCLUDED.content_hash,
			commit_sha=EXCLUDED.commit_sha,updated_at=now()`,
			repositoryID, filePath, path.Base(filePath), hash, req.Repository.CommitSHA)
		if err != nil {
			return err
		}
	}
	return nil
}

func saveGeneratedFiles(ctx context.Context, tx pgx.Tx, sessionID string, contract *domain.GeneratedFilesContract) error {
	taskID := contract.TaskID
	baseBranch := contract.BaseBranch
	if strings.TrimSpace(baseBranch) == "" {
		baseBranch = "main"
	}
	status := "pending"
	if len(contract.Files) > 0 {
		status = "generated"
	}
	if contract.ApplyStatus == "applied" || contract.ApplyStatus == "failed" {
		status = contract.ApplyStatus
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO git_workflow_payloads (
			issue_session_id,agent_task_id,branch_name,base_branch,commit_message,pr_title,reviewer,status,updated_at
		) VALUES (
			$1::uuid,NULLIF($2::text,'')::uuid,$3,$4,$5,$6,$7,$8,now()
		) ON CONFLICT (issue_session_id) DO UPDATE SET
			agent_task_id=EXCLUDED.agent_task_id,
			branch_name=EXCLUDED.branch_name,
			base_branch=EXCLUDED.base_branch,
			commit_message=EXCLUDED.commit_message,
			pr_title=EXCLUDED.pr_title,
			reviewer=EXCLUDED.reviewer,
			status=EXCLUDED.status,
			updated_at=now()`,
		sessionID, taskID, contract.BranchName, baseBranch, contract.CommitMessage, contract.PRTitle, contract.Reviewer, status)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM generated_files WHERE issue_session_id=$1::uuid`, sessionID)
	if err != nil {
		return err
	}
	for _, file := range contract.Files {
		fileStatus := file.Status
		if strings.TrimSpace(fileStatus) == "" {
			fileStatus = "pending"
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO generated_files (
				issue_session_id,agent_task_id,file_path,action,content,diff,explanation,status,validation_error,updated_at
			) VALUES (
				$1::uuid,NULLIF($2::text,'')::uuid,$3,$4,$5,$6,$7,$8,$9,now()
			)`,
			sessionID, taskID, file.Path, file.Action, file.Content, file.Diff, file.Explanation, fileStatus, file.ValidationError)
		if err != nil {
			return err
		}
	}
	return nil
}

func insertAIConfig(ctx context.Context, tx pgx.Tx, repositoryID string, cfg domain.AIConfig) (string, error) {
	parsed, _ := json.Marshal(cfg)
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO ai_configs (repository_id,raw_yml,parsed_config_json,is_valid,retention_days)
		VALUES ($1::uuid,$2,$3::jsonb,true,$4) RETURNING id::text`, repositoryID, cfg.Raw, string(parsed), cfg.RetentionDays).Scan(&id)
	return id, err
}

func insertUserResponse(ctx context.Context, tx pgx.Tx, sessionID, responseType, message, author string) error {
	_, err := tx.Exec(ctx, `INSERT INTO user_responses (issue_session_id,response_type,message,author) VALUES ($1::uuid,$2,$3,$4)`, sessionID, responseType, message, author)
	return err
}

func latestFeedback(history []string) string {
	if len(history) == 0 {
		return ""
	}
	return history[len(history)-1]
}
