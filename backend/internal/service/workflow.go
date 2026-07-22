package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"gitflame-codepilot/backend/internal/agent"
	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/queue"
	"gitflame-codepilot/backend/internal/repository"
)

type Workflow struct {
	store     repository.Store
	generator agent.Generator
	broker    queue.Broker
}

func NewWorkflow(store repository.Store, generator agent.Generator) *Workflow {
	return &Workflow{store: store, generator: generator}
}

func NewQueuedWorkflow(store repository.Store, broker queue.Broker) *Workflow {
	return &Workflow{store: store, broker: broker}
}

func (w *Workflow) Analyze(req domain.IssueAnalyzeRequest) (*domain.IssueSession, *domain.AgentTask, error) {
	if err := ValidateAnalyze(req); err != nil {
		return nil, nil, err
	}
	cfg, err := ParseAIConfig(req.YAMLConfig)
	if err != nil {
		return nil, nil, err
	}
	session, created, err := w.store.CreateSession(req, cfg)
	if err != nil {
		return nil, nil, err
	}
	if !created {
		task, err := w.store.LatestTask(session.ID)
		if err == nil {
			return session, task, nil
		}
		if !errors.Is(err, repository.ErrNotFound) || session.Status != domain.SessionGenerating {
			return nil, nil, err
		}
	}
	task, err := w.store.CreateTask(session.ID, req.Issue.ID, domain.TaskInitialPlan)
	if err != nil {
		return nil, nil, err
	}
	job := domain.AgentJob{TaskID: task.ID, SessionID: session.ID, Type: task.Type, Attempt: 1, Request: agentRequest(task.ID, session, nil, nil)}
	if err := w.dispatch(job); err != nil {
		return nil, nil, err
	}
	return session, task, nil
}

func (w *Workflow) Correct(issueID, feedback string) (*domain.AgentTask, error) {
	if strings.TrimSpace(feedback) == "" {
		return nil, errors.New("feedback is required")
	}
	session, err := w.store.Session(issueID)
	if err != nil {
		return nil, err
	}
	if session.Status != domain.SessionPlanGenerated {
		return nil, fmt.Errorf("plan cannot be corrected while session status is %s", session.Status)
	}
	session.Status = domain.SessionCorrectionRequested
	session.FeedbackHistory = append(session.FeedbackHistory, feedback)
	if err := w.store.UpdateSession(session); err != nil {
		return nil, err
	}
	task, err := w.store.CreateTask(session.ID, session.Request.Issue.ID, domain.TaskPlanRevision)
	if err != nil {
		return nil, err
	}
	previousPlan := session.PlanMarkdown
	job := domain.AgentJob{TaskID: task.ID, SessionID: session.ID, Type: task.Type, Attempt: 1, Request: agentRequest(task.ID, session, &previousPlan, &feedback)}
	if err := w.dispatch(job); err != nil {
		return nil, err
	}
	return task, nil
}

func (w *Workflow) Retry(taskID string) (*domain.AgentTask, error) {
	failed, err := w.store.Task(taskID)
	if err != nil {
		return nil, err
	}
	if failed.Status != domain.TaskFailed || failed.Error == nil || !recoverableTaskError(failed.Error) {
		return nil, errors.New("only failed tasks with a temporary Agent Engine error can be retried")
	}
	session, err := w.store.Session(failed.SessionID)
	if err != nil {
		return nil, err
	}
	task, err := w.store.CreateTask(session.ID, session.Request.Issue.ID, failed.Type)
	if err != nil {
		return nil, err
	}
	task.Attempt = failed.Attempt + 1
	if err := w.store.UpdateTask(task); err != nil {
		return nil, err
	}
	session.Status = queuedStatusForTask(failed.Type)
	if err := w.store.UpdateSession(session); err != nil {
		return nil, err
	}
	var previousPlan, feedback *string
	if failed.Type == domain.TaskPlanRevision {
		previous := session.PlanMarkdown
		previousPlan = &previous
		if len(session.FeedbackHistory) > 0 {
			value := session.FeedbackHistory[len(session.FeedbackHistory)-1]
			feedback = &value
		}
	}
	job := domain.AgentJob{TaskID: task.ID, SessionID: session.ID, Type: task.Type, Attempt: task.Attempt, Request: agentRequest(task.ID, session, previousPlan, feedback)}
	if failed.Type == domain.TaskCodeGeneration {
		job.Request = domain.AgentPlanRequest{}
		job.CodeGenerationRequest = codeGenerationRequest(task.ID, session)
	}
	if err := w.dispatch(job); err != nil {
		return nil, err
	}
	return task, nil
}

func (w *Workflow) dispatch(job domain.AgentJob) error {
	if w.broker != nil {
		if err := w.broker.Publish(context.Background(), job); err != nil {
			_ = w.failTask(job.TaskID, err)
			return fmt.Errorf("%w: %v", ErrDispatch, err)
		}
		return nil
	}
	if w.generator == nil {
		return fmt.Errorf("%w: task dispatcher is not configured", ErrDispatch)
	}
	go func() { _ = w.ExecuteTask(context.Background(), job) }()
	return nil
}

func (w *Workflow) ExecuteTask(ctx context.Context, job domain.AgentJob) error {
	if w.generator == nil {
		return errors.New("Agent Engine client is not configured")
	}
	task, err := w.store.Task(job.TaskID)
	if err != nil {
		return err
	}
	task.Status = domain.TaskProcessing
	task.Attempt = job.Attempt
	task.Error = nil
	if err := w.store.UpdateTask(task); err != nil {
		return err
	}
	session, err := w.store.Session(task.SessionID)
	if err != nil {
		return err
	}
	session.Status = processingStatusForTask(task.Type)
	if err := w.store.UpdateSession(session); err != nil {
		return err
	}
	if task.Type == domain.TaskCodeGeneration {
		return w.executeCodeGenerationTask(ctx, task, session, job)
	}
	return w.executePlanTask(ctx, task, session, job)
}

func (w *Workflow) executePlanTask(ctx context.Context, task *domain.AgentTask, session *domain.IssueSession, job domain.AgentJob) error {
	result, err := w.generator.GeneratePlan(ctx, job.Request)
	if err != nil {
		_ = w.failTask(job.TaskID, err)
		return err
	}
	validationFiles := append([]domain.RepositoryFile(nil), job.Request.RepositoryFiles...)
	for _, relevant := range result.RelevantFiles {
		validationFiles = append(validationFiles, domain.RepositoryFile{Path: relevant.Path})
	}
	if err := ValidatePlan(result.PlanMarkdown, validationFiles); err != nil {
		invalid := &agent.Error{Status: http.StatusUnprocessableEntity, Code: "invalid_output", Detail: err.Error()}
		_ = w.failTask(job.TaskID, invalid)
		return invalid
	}
	task.Status = domain.TaskCompleted
	task.PlanMarkdown = result.PlanMarkdown
	task.RelevantFiles = result.RelevantFiles
	task.Model = result.Model
	task.Usage = result.Usage
	task.ToolExecutionSummary = fmt.Sprintf(
		"model=%s; tool_calls=%d; prompt_tokens=%d; completion_tokens=%d; total_tokens=%d; reasoning_chars=%d; generation_seconds=%.3f",
		result.Model, result.Usage.ToolCalls, result.Usage.PromptTokens, result.Usage.CompletionTokens,
		result.Usage.TotalTokens, result.Usage.ReasoningChars, result.Usage.GenerationTimeSeconds,
	)
	if err := w.store.UpdateTask(task); err != nil {
		return err
	}
	session, err = w.store.Session(task.SessionID)
	if err != nil {
		return err
	}
	session.PlanMarkdown = result.PlanMarkdown
	session.Status = domain.SessionPlanGenerated
	session.Revision++
	return w.store.UpdateSession(session)
}

func (w *Workflow) executeCodeGenerationTask(ctx context.Context, task *domain.AgentTask, session *domain.IssueSession, job domain.AgentJob) error {
	request := job.CodeGenerationRequest
	if request.RequestID == "" {
		request = codeGenerationRequest(task.ID, session)
	}
	result, err := w.generator.GenerateFiles(ctx, request)
	if err != nil {
		_ = w.failTask(job.TaskID, err)
		return err
	}
	result.Files = DropUnsafePartialModifyFiles(
		DropNoopGeneratedFiles(NormalizeGeneratedFiles(result.Files), request.RepositoryFiles),
		request.RepositoryFiles,
	)
	if err := ValidateGeneratedFiles(result.Files, request.RepositoryFiles); err != nil {
		invalid := &agent.Error{Status: http.StatusUnprocessableEntity, Code: "invalid_generated_files", Detail: err.Error()}
		_ = w.failTask(job.TaskID, invalid)
		return invalid
	}
	for index := range result.Files {
		result.Files[index].Path = normalizePlanPath(result.Files[index].Path)
		result.Files[index].Status = "valid"
	}
	contract := branchPRPayload(session)
	contract.RequestID = result.RequestID
	contract.TaskID = task.ID
	contract.Summary = result.Summary
	contract.Files = result.Files

	task.Status = domain.TaskCompleted
	task.Model = result.Model
	task.Usage = result.Usage
	task.ToolExecutionSummary = fmt.Sprintf(
		"model=%s; file_operations=%d; prompt_tokens=%d; completion_tokens=%d; total_tokens=%d; reasoning_chars=%d; generation_seconds=%.3f",
		result.Model, len(result.Files), result.Usage.PromptTokens, result.Usage.CompletionTokens,
		result.Usage.TotalTokens, result.Usage.ReasoningChars, result.Usage.GenerationTimeSeconds,
	)
	if err := w.store.UpdateTask(task); err != nil {
		return err
	}
	session, err = w.store.Session(task.SessionID)
	if err != nil {
		return err
	}
	session.GeneratedFiles = &contract
	session.Status = domain.SessionCodeGenerated
	return w.store.UpdateSession(session)
}

func (w *Workflow) RetryTask(job domain.AgentJob) error {
	task, err := w.store.Task(job.TaskID)
	if err != nil {
		return err
	}
	task.Status = domain.TaskQueued
	task.Attempt = job.Attempt
	if err := w.store.UpdateTask(task); err != nil {
		return err
	}
	session, err := w.store.Session(task.SessionID)
	if err != nil {
		return err
	}
	session.Status = queuedStatusForTask(task.Type)
	return w.store.UpdateSession(session)
}

func (w *Workflow) failTask(taskID string, cause error) error {
	task, err := w.store.Task(taskID)
	if err != nil {
		return err
	}
	task.Status = domain.TaskFailed
	task.Error = toTaskError(cause)
	if err := w.store.UpdateTask(task); err != nil {
		return err
	}
	session, err := w.store.Session(task.SessionID)
	if err == nil {
		session.Status = domain.SessionFailed
		_ = w.store.UpdateSession(session)
	}
	return nil
}

func (w *Workflow) Task(id string) (*domain.AgentTask, error)       { return w.store.Task(id) }
func (w *Workflow) Session(id string) (*domain.IssueSession, error) { return w.store.Session(id) }

func (w *Workflow) Approve(issueID string, editedPlan *string) (*domain.IssueSession, *domain.AgentTask, error) {
	session, err := w.store.Session(issueID)
	if err != nil {
		return nil, nil, err
	}
	if session.Status != domain.SessionPlanGenerated {
		return nil, nil, fmt.Errorf("plan cannot be approved while session status is %s", session.Status)
	}
	if editedPlan != nil {
		plan := strings.TrimSpace(*editedPlan)
		if plan == "" {
			return nil, nil, errors.New("plan_markdown cannot be empty")
		}
		if plan != session.PlanMarkdown {
			if err := ValidatePlan(plan, repositoryFilesForSession(session)); err != nil {
				return nil, nil, fmt.Errorf("edited plan is invalid: %w", err)
			}
			session.PlanMarkdown = plan
			session.Revision++
			if err := w.store.UpdateSession(session); err != nil {
				return nil, nil, err
			}
		}
	}
	contract := branchPRPayload(session)
	session.GeneratedFiles = &contract
	session.Status = domain.SessionApproved
	if err := w.store.UpdateSession(session); err != nil {
		return nil, nil, err
	}
	task, err := w.store.CreateTask(session.ID, session.Request.Issue.ID, domain.TaskCodeGeneration)
	if err != nil {
		return nil, nil, err
	}
	session.Status = domain.SessionCodeGenerationQueued
	if err := w.store.UpdateSession(session); err != nil {
		return nil, nil, err
	}
	job := domain.AgentJob{TaskID: task.ID, SessionID: session.ID, Type: task.Type, Attempt: 1, CodeGenerationRequest: codeGenerationRequest(task.ID, session)}
	if err := w.dispatch(job); err != nil {
		return nil, nil, err
	}
	return session, task, nil
}

func (w *Workflow) Reject(issueID string) (*domain.IssueSession, error) {
	session, err := w.store.Session(issueID)
	if err != nil {
		return nil, err
	}
	if session.Status == domain.SessionApproved || session.Status == domain.SessionCodeGenerationQueued ||
		session.Status == domain.SessionCodeGenerationProcessing || session.Status == domain.SessionCodeGenerated {
		return nil, errors.New("approved plan cannot be rejected")
	}
	session.Status = domain.SessionRejected
	if err := w.store.UpdateSession(session); err != nil {
		return nil, err
	}
	return session, nil
}

func agentRequest(taskID string, session *domain.IssueSession, previousPlan, feedback *string) domain.AgentPlanRequest {
	return domain.AgentPlanRequest{
		RequestID:         taskID,
		Issue:             domain.AgentIssue{ID: session.Request.Issue.ID, Title: session.Request.Issue.Title, Body: session.Request.Issue.Body},
		Repository:        domain.AgentRepository{ID: session.Request.Repository.ID, DefaultBranch: session.Request.Repository.DefaultBranch, CommitSHA: session.Request.Repository.CommitSHA},
		ConfigurationYAML: session.Config.Raw,
		RepositoryFiles:   repositoryFilesForSession(session), PreviousPlan: previousPlan, CorrectionFeedback: feedback,
	}
}

func codeGenerationRequest(taskID string, session *domain.IssueSession) domain.AgentCodeGenerationRequest {
	return domain.AgentCodeGenerationRequest{
		RequestID:            taskID,
		Issue:                domain.AgentIssue{ID: session.Request.Issue.ID, Title: session.Request.Issue.Title, Body: session.Request.Issue.Body},
		Repository:           domain.AgentRepository{ID: session.Request.Repository.ID, DefaultBranch: session.Request.Repository.DefaultBranch, CommitSHA: session.Request.Repository.CommitSHA},
		ApprovedPlanMarkdown: session.PlanMarkdown,
		ConfigurationYAML:    session.Config.Raw,
		RepositoryFiles:      repositoryFilesForSession(session),
	}
}

func repositoryFilesForSession(session *domain.IssueSession) []domain.RepositoryFile {
	files := append([]domain.RepositoryFile(nil), session.Request.RepositoryFiles...)
	if len(files) == 0 {
		for _, path := range session.Request.RepositoryContext {
			files = append(files, domain.RepositoryFile{Path: path})
		}
	}
	return files
}

func branchPRPayload(session *domain.IssueSession) domain.GeneratedFilesContract {
	slug := regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(strings.ToLower(session.Request.Issue.Title), "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 40 {
		slug = slug[:40]
	}
	if slug == "" {
		slug = "issue"
	}
	branch := normalizeGitFlameBranchName(
		session.Config.TargetBranchPrefix + session.Request.Issue.ID + "-" + slug,
	)
	return domain.GeneratedFilesContract{
		BranchName:    branch,
		BaseBranch:    session.Request.Repository.DefaultBranch,
		Files:         []domain.GeneratedFileOperation{},
		CommitMessage: "Implement " + session.Request.Issue.Title,
		PRTitle:       session.Request.Issue.Title,
		Reviewer:      session.Request.Issue.Author,
	}
}

func normalizeGitFlameBranchName(value string) string {
	normalized := regexp.MustCompile(`[^a-zA-Z0-9._-]+`).ReplaceAllString(strings.TrimSpace(value), "-")
	normalized = strings.Trim(normalized, "-.")
	if normalized == "" {
		return "ai-issue"
	}
	return normalized
}

func queuedStatusForTask(taskType string) string {
	if taskType == domain.TaskCodeGeneration {
		return domain.SessionCodeGenerationQueued
	}
	return domain.SessionGenerating
}

func processingStatusForTask(taskType string) string {
	if taskType == domain.TaskCodeGeneration {
		return domain.SessionCodeGenerationProcessing
	}
	return domain.SessionProcessing
}

func ValidateAnalyze(v domain.IssueAnalyzeRequest) error {
	switch {
	case strings.TrimSpace(v.Repository.ID) == "":
		return errors.New("repository.id is required")
	case strings.TrimSpace(v.Repository.DefaultBranch) == "":
		return errors.New("repository.default_branch is required")
	case strings.TrimSpace(v.Issue.ID) == "":
		return errors.New("issue.id is required")
	case strings.TrimSpace(v.Issue.Title) == "":
		return errors.New("issue.title is required")
	case strings.TrimSpace(v.Issue.Body) == "":
		return errors.New("issue.body is required")
	case strings.TrimSpace(v.Issue.Author) == "":
		return errors.New("issue.author is required")
	case len(v.RepositoryFiles) == 0 && len(v.RepositoryContext) == 0:
		return errors.New("repository_files must contain at least one file")
	default:
		files := v.RepositoryFiles
		if len(files) == 0 {
			for _, legacyPath := range v.RepositoryContext {
				files = append(files, domain.RepositoryFile{Path: legacyPath})
			}
		}
		return validateRepositoryFiles(files)
	}
}

func validateRepositoryFiles(files []domain.RepositoryFile) error {
	if len(files) > 2_000 {
		return errors.New("repository_files cannot contain more than 2000 files")
	}
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		normalized := normalizePlanPath(file.Path)
		if strings.TrimSpace(file.Path) == "" || !safePlanPath(normalized) {
			return fmt.Errorf("repository_files contains unsafe path %q", file.Path)
		}
		if len(file.Content) > 500_000 {
			return fmt.Errorf("repository file %q exceeds 500000 characters", normalized)
		}
		if _, exists := seen[normalized]; exists {
			return fmt.Errorf("repository_files contains duplicate path %q", normalized)
		}
		seen[normalized] = struct{}{}
	}
	return nil
}

func ValidateRepositoryFilesForIntegration(files []domain.RepositoryFile) error {
	return validateRepositoryFiles(files)
}

func NormalizeGeneratedFiles(files []domain.GeneratedFileOperation) []domain.GeneratedFileOperation {
	merged := make([]domain.GeneratedFileOperation, 0, len(files))
	indexByPath := make(map[string]int, len(files))
	for _, file := range files {
		file.Path = normalizePlanPath(file.Path)
		if file.Path == "" {
			merged = append(merged, file)
			continue
		}
		index, exists := indexByPath[file.Path]
		if !exists {
			indexByPath[file.Path] = len(merged)
			merged = append(merged, file)
			continue
		}
		previous := merged[index]
		if strings.TrimSpace(file.Action) == "" {
			file.Action = previous.Action
		}
		if strings.TrimSpace(file.Content) == "" {
			file.Content = previous.Content
		}
		if strings.TrimSpace(file.Diff) == "" {
			file.Diff = previous.Diff
		}
		file.Explanation = mergeExplanation(previous.Explanation, file.Explanation)
		merged[index] = file
	}
	return merged
}

func DropNoopGeneratedFiles(files []domain.GeneratedFileOperation, repositoryFiles []domain.RepositoryFile) []domain.GeneratedFileOperation {
	contentByPath := make(map[string]string, len(repositoryFiles))
	for _, file := range repositoryFiles {
		contentByPath[normalizePlanPath(file.Path)] = normalizeGeneratedFileContent(file.Content)
	}
	filtered := make([]domain.GeneratedFileOperation, 0, len(files))
	for _, file := range files {
		if file.Action == "modify" && normalizeGeneratedFileContent(file.Content) == contentByPath[normalizePlanPath(file.Path)] {
			continue
		}
		filtered = append(filtered, file)
	}
	return filtered
}

func DropNoopGeneratedFilesForApply(files []domain.GeneratedFileOperation, repositoryFiles []domain.RepositoryFile) []domain.GeneratedFileOperation {
	contentByPath := make(map[string]string, len(repositoryFiles))
	for _, file := range repositoryFiles {
		contentByPath[normalizePlanPath(file.Path)] = normalizeGeneratedFileContent(file.Content)
	}
	filtered := make([]domain.GeneratedFileOperation, 0, len(files))
	for _, file := range files {
		if file.Action == "modify" && normalizeGeneratedFileContent(file.Content) == contentByPath[normalizePlanPath(file.Path)] {
			continue
		}
		filtered = append(filtered, file)
	}
	return filtered
}

func DropUnsafePartialModifyFiles(files []domain.GeneratedFileOperation, repositoryFiles []domain.RepositoryFile) []domain.GeneratedFileOperation {
	contentByPath := make(map[string]string, len(repositoryFiles))
	for _, file := range repositoryFiles {
		contentByPath[normalizePlanPath(file.Path)] = file.Content
	}
	filtered := make([]domain.GeneratedFileOperation, 0, len(files))
	for _, file := range files {
		if file.Action == "modify" {
			original, exists := contentByPath[normalizePlanPath(file.Path)]
			if exists && looksLikePartialModifyContent(file.Content, original) {
				continue
			}
		}
		filtered = append(filtered, file)
	}
	return filtered
}

func looksLikePartialModifyContent(generated, original string) bool {
	generated = strings.TrimSpace(normalizeGeneratedFileContent(generated))
	original = strings.TrimSpace(normalizeGeneratedFileContent(original))
	if generated == "" || original == "" {
		return false
	}
	generatedLines := meaningfulLines(generated)
	originalLines := meaningfulLines(original)
	if len(originalLines) < 3 && len(original) < 120 {
		return false
	}
	if len(originalLines) >= 3 && len(generatedLines) <= 1 {
		return true
	}
	if len(original) >= 120 {
		minChars := len(original) * 35 / 100
		if minChars < 80 {
			minChars = 80
		}
		if len(generated) < minChars {
			return true
		}
	}
	if len(originalLines) >= 8 {
		minLines := len(originalLines) * 35 / 100
		if minLines < 3 {
			minLines = 3
		}
		if len(generatedLines) < minLines {
			return true
		}
	}
	return false
}

func meaningfulLines(value string) []string {
	rawLines := strings.Split(value, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func normalizeGeneratedFileContent(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

func mergeExplanation(previous, next string) string {
	previous = strings.TrimSpace(previous)
	next = strings.TrimSpace(next)
	switch {
	case previous == "":
		return next
	case next == "":
		return previous
	case previous == next:
		return next
	default:
		return previous + "\n" + next
	}
}

func ValidateGeneratedFiles(files []domain.GeneratedFileOperation, repositoryFiles []domain.RepositoryFile) error {
	if len(files) == 0 {
		return errors.New("generated files response must contain at least one file operation")
	}
	if len(files) > 500 {
		return errors.New("generated files response cannot contain more than 500 file operations")
	}
	existing := make(map[string]struct{}, len(repositoryFiles))
	for _, file := range repositoryFiles {
		existing[normalizePlanPath(file.Path)] = struct{}{}
	}
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		filePath := normalizePlanPath(file.Path)
		if !safePlanPath(filePath) {
			return fmt.Errorf("generated file contains unsafe path: %s", file.Path)
		}
		if _, exists := seen[filePath]; exists {
			return fmt.Errorf("generated files response contains duplicate path: %s", filePath)
		}
		seen[filePath] = struct{}{}
		if strings.TrimSpace(file.Explanation) == "" {
			return fmt.Errorf("generated file %s must include an explanation", filePath)
		}
		_, alreadyExists := existing[filePath]
		switch file.Action {
		case "create":
			if alreadyExists {
				return fmt.Errorf("create operation targets an existing file: %s", filePath)
			}
			if strings.TrimSpace(file.Content) == "" {
				return fmt.Errorf("create operation for %s requires non-empty content", filePath)
			}
		case "modify":
			if !alreadyExists {
				return fmt.Errorf("modify operation targets an unavailable file: %s", filePath)
			}
			if strings.TrimSpace(file.Content) == "" {
				return fmt.Errorf("modify operation for %s requires non-empty content", filePath)
			}
		case "delete":
			if !alreadyExists {
				return fmt.Errorf("delete operation targets an unavailable file: %s", filePath)
			}
			if file.Content != "" || file.Diff != "" {
				return fmt.Errorf("delete operation for %s must not include content or diff", filePath)
			}
		default:
			return fmt.Errorf("generated file %s has unsupported action %q", filePath, file.Action)
		}
	}
	return nil
}

func toTaskError(err error) *domain.TaskError {
	var engineError *agent.Error
	if errors.As(err, &engineError) {
		return &domain.TaskError{HTTPStatus: engineError.Status, Code: engineError.Code, Detail: engineError.Detail}
	}
	return &domain.TaskError{HTTPStatus: http.StatusBadGateway, Code: "agent_engine_error", Detail: err.Error()}
}

func recoverableTaskError(taskError *domain.TaskError) bool {
	if taskError.HTTPStatus == http.StatusServiceUnavailable || taskError.HTTPStatus == http.StatusGatewayTimeout {
		return true
	}
	return taskError.HTTPStatus == http.StatusBadGateway &&
		(taskError.Code == "agent_engine_unreachable" || taskError.Code == "agent_engine_error")
}

var ErrDispatch = errors.New("task dispatch failed")
