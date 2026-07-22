package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gitflame-codepilot/backend/internal/agent"
	"gitflame-codepilot/backend/internal/config"
	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/repository"
	"gitflame-codepilot/backend/internal/security"
)

type fakeGenerator struct {
	mu           sync.Mutex
	requests     []domain.AgentPlanRequest
	fileRequests []domain.AgentCodeGenerationRequest
	err          error
	fileErr      error
}

func (f *fakeGenerator) GeneratePlan(_ context.Context, req domain.AgentPlanRequest) (domain.AgentPlanResponse, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	if f.err != nil {
		return domain.AgentPlanResponse{}, f.err
	}
	path := "TBD"
	if len(req.RepositoryFiles) > 0 {
		path = req.RepositoryFiles[0].Path
	}
	plan := `# Implementation Plan

## Issue Summary
Add asynchronous task status.

## Goal
Expose observable generation state.

## Relevant Files
- ` + "`" + path + "`" + `: contains relevant implementation.

## Proposed Changes
- Add task status handling.

## Implementation Steps
1. Update the API.

## Expected Files to Change
- ` + "`" + path + "`" + `: modify.

## Tests and Verification
- Run integration tests.

## Risks and Open Questions
- TBD.
`
	return domain.AgentPlanResponse{RequestID: req.RequestID, Status: domain.TaskCompleted, PlanMarkdown: plan, Model: "test-model", Usage: domain.AgentUsage{ToolCalls: 2}}, nil
}

func (f *fakeGenerator) GenerateFiles(_ context.Context, req domain.AgentCodeGenerationRequest) (domain.AgentGeneratedFilesResponse, error) {
	f.mu.Lock()
	f.fileRequests = append(f.fileRequests, req)
	f.mu.Unlock()
	if f.fileErr != nil {
		return domain.AgentGeneratedFilesResponse{}, f.fileErr
	}
	return domain.AgentGeneratedFilesResponse{
		RequestID: req.RequestID,
		Status:    domain.TaskCompleted,
		Summary:   "Generated test file operations.",
		Files: []domain.GeneratedFileOperation{{
			Action:      "modify",
			Path:        req.RepositoryFiles[0].Path,
			Content:     "package httpapi\n// updated",
			Diff:        "@@\n+// updated\n",
			Explanation: "Applies the approved plan.",
		}},
		Model: "test-codegen-model",
		Usage: domain.AgentUsage{TotalTokens: 42},
	}, nil
}

type fakeGitFlameSource struct {
	request         GitFlameIssueWebhook
	result          domain.IssueAnalyzeRequest
	err             error
	applyRepository domain.RepositoryMetadata
	applyContract   domain.GeneratedFilesContract
	applyResult     domain.GitFlameApplyResult
	applyErr        error
	tree            []GitFlameTreeEntry
	files           []domain.RepositoryFile
	issues          []domain.IssuePayload
}

func (f *fakeGitFlameSource) BuildAnalyzeRequest(_ context.Context, req GitFlameIssueWebhook) (domain.IssueAnalyzeRequest, error) {
	f.request = req
	return f.result, f.err
}

func (f *fakeGitFlameSource) ApplyGeneratedFiles(_ context.Context, repository domain.RepositoryMetadata, contract domain.GeneratedFilesContract) (domain.GitFlameApplyResult, error) {
	f.applyRepository = repository
	f.applyContract = contract
	if f.applyErr != nil {
		return domain.GitFlameApplyResult{}, f.applyErr
	}
	if f.applyResult.BranchName == "" {
		f.applyResult = domain.GitFlameApplyResult{BranchName: contract.BranchName, CommitSHA: "commit-123", PullRequestID: "7", PullRequestURL: "https://gitflame.test/pulls/7"}
	}
	return f.applyResult, nil
}

func (f *fakeGitFlameSource) CurrentUser(context.Context) (GitFlameUserProfile, error) {
	return GitFlameUserProfile{ID: "gitflame-user-1", Username: "artur"}, nil
}

func (f *fakeGitFlameSource) RepositoryTree(context.Context, string, string) ([]GitFlameTreeEntry, error) {
	return f.tree, f.err
}

func (f *fakeGitFlameSource) RepositoryFiles(_ context.Context, _ string, _ string, yamlConfig string, requested []domain.RepositoryFile) (string, []domain.RepositoryFile, error) {
	if f.err != nil {
		return "", nil, f.err
	}
	if yamlConfig == "" {
		yamlConfig = "version: 1"
	}
	if len(f.files) > 0 {
		return yamlConfig, append([]domain.RepositoryFile(nil), f.files...), nil
	}
	files := append([]domain.RepositoryFile(nil), requested...)
	for index := range files {
		if files[index].Content == "" {
			files[index].Content = "content for " + files[index].Path
		}
	}
	return yamlConfig, files, nil
}

func (f *fakeGitFlameSource) RepositoryIssues(context.Context, string) ([]domain.IssuePayload, error) {
	return f.issues, f.err
}

func TestRepositoryDataRequiresOwnedConnection(t *testing.T) {
	store := repository.NewMemoryStore()
	user, err := store.UpsertAppUser(domain.AppUser{GitFlameUserID: "gitflame-user-1", Username: "artur"})
	if err != nil {
		t.Fatal(err)
	}
	cookieValue, tokenHash, err := security.GenerateSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAppSession(user.ID, tokenHash, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	connection, err := store.SaveGitFlameConnection(domain.GitFlameConnection{
		UserID:        user.ID,
		Repository:    domain.RepositoryMetadata{ID: "owner/repo", DefaultBranch: "main"},
		DefaultBranch: "main",
		TokenStatus:   "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	gitflame := &fakeGitFlameSource{
		tree:   []GitFlameTreeEntry{{Path: "README.md", Type: "file"}},
		files:  []domain.RepositoryFile{{Path: "README.md", Content: "# Backend"}},
		issues: []domain.IssuePayload{{ID: "7", Title: "Test issue"}},
	}
	server := NewWithDependenciesAndIntegrations(store, &fakeGenerator{}, gitflame, nil)

	unauthorized := request(t, server.Router(), http.MethodGet, "/integrations/gitflame/connections/"+connection.ID+"/tree", "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized tree status = %d: %s", unauthorized.Code, unauthorized.Body.String())
	}
	cookie := "codepilot_session=" + cookieValue
	tree := requestWithCookie(t, server.Router(), http.MethodGet, "/integrations/gitflame/connections/"+connection.ID+"/tree", "", cookie)
	if tree.Code != http.StatusOK || !strings.Contains(tree.Body.String(), `"path":"README.md"`) {
		t.Fatalf("tree status = %d: %s", tree.Code, tree.Body.String())
	}
	files := requestWithCookie(t, server.Router(), http.MethodGet, "/integrations/gitflame/connections/"+connection.ID+"/files", "", cookie)
	if files.Code != http.StatusOK || !strings.Contains(files.Body.String(), `"content":"# Backend"`) {
		t.Fatalf("files status = %d: %s", files.Code, files.Body.String())
	}
	issues := requestWithCookie(t, server.Router(), http.MethodGet, "/integrations/gitflame/connections/"+connection.ID+"/issues", "", cookie)
	if issues.Code != http.StatusOK || !strings.Contains(issues.Body.String(), `"title":"Test issue"`) {
		t.Fatalf("issues status = %d: %s", issues.Code, issues.Body.String())
	}
}

func TestAnalyzeHydratesPathOnlyRepositoryFilesBeforeAgentRequest(t *testing.T) {
	store := repository.NewMemoryStore()
	generator := &fakeGenerator{}
	gitflame := &fakeGitFlameSource{files: []domain.RepositoryFile{{Path: "README.md", Content: "# Backend from GitFlame"}}}
	server := NewWithDependenciesAndIntegrations(store, generator, gitflame, nil)

	body := `{"repository":{"id":"owner/repo","default_branch":"main"},"issue":{"id":"47","title":"Hydrate files","body":"Use real contents","author":"artur"},"yaml_config":"version: 1","repository_files":[{"path":"README.md","content":""}]}`
	response := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/issues/analyze", body)
	if response.Code != http.StatusAccepted {
		t.Fatalf("analyze status = %d: %s", response.Code, response.Body.String())
	}
	var queued struct {
		TaskID string `json:"task_id"`
	}
	decodeResponse(t, response, &queued)
	waitTask(t, server.Router(), queued.TaskID)

	generator.mu.Lock()
	defer generator.mu.Unlock()
	if len(generator.requests) != 1 || len(generator.requests[0].RepositoryFiles) != 1 {
		t.Fatalf("agent requests = %+v", generator.requests)
	}
	if generator.requests[0].RepositoryFiles[0].Content != "# Backend from GitFlame" {
		t.Fatalf("repository content was not hydrated: %+v", generator.requests[0].RepositoryFiles)
	}
}

func TestAnalyzeHydratesEmptyRepositoryFilesBeforeAgentRequest(t *testing.T) {
	store := repository.NewMemoryStore()
	generator := &fakeGenerator{}
	gitflame := &fakeGitFlameSource{files: []domain.RepositoryFile{{Path: "README.md", Content: "# Backend from GitFlame"}}}
	server := NewWithDependenciesAndIntegrations(store, generator, gitflame, nil)

	body := `{"repository":{"id":"owner/repo","default_branch":"main"},"issue":{"id":"48","title":"Hydrate empty files","body":"Use real contents","author":"artur"},"yaml_config":"version: 1"}`
	response := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/issues/analyze", body)
	if response.Code != http.StatusAccepted {
		t.Fatalf("analyze status = %d: %s", response.Code, response.Body.String())
	}
	var queued struct {
		TaskID string `json:"task_id"`
	}
	decodeResponse(t, response, &queued)
	waitTask(t, server.Router(), queued.TaskID)

	generator.mu.Lock()
	defer generator.mu.Unlock()
	if len(generator.requests) != 1 || len(generator.requests[0].RepositoryFiles) != 1 {
		t.Fatalf("agent requests = %+v", generator.requests)
	}
	if generator.requests[0].RepositoryFiles[0].Content != "# Backend from GitFlame" {
		t.Fatalf("repository content was not hydrated: %+v", generator.requests[0].RepositoryFiles)
	}
}

type fakeRecommender struct {
	configYAML string
	files      []domain.RepositoryFile
	err        error
}

func (f *fakeRecommender) AnalyzeRecommendations(_ context.Context, configYAML string, files []domain.RepositoryFile) (string, []domain.RecommendationCard, error) {
	f.configYAML = configYAML
	f.files = append([]domain.RepositoryFile(nil), files...)
	if f.err != nil {
		return "", nil, f.err
	}
	confidence := .91
	line := 7
	return "ML-generated repository recommendations.", []domain.RecommendationCard{{
		Severity: "high", Category: "security", File: files[0].Path, Line: &line,
		Problem: "Sensitive value can be exposed.", Suggestion: "Move it to configuration.", Confidence: &confidence,
	}}, nil
}

func TestIssueToPlanCorrectionAndApprovalFlow(t *testing.T) {
	generator := &fakeGenerator{}
	server := NewWithDependencies(repository.NewMemoryStore(), generator)
	body := `{"repository":{"id":"repo-1","default_branch":"main","commit_sha":"abc123"},"issue":{"id":"42","title":"Add task status","body":"Expose async status","author":"artur"},"yaml_config":"version: 1\nanalysis:\n  enabled: true\n","repository_files":[{"path":"internal/httpapi/server.go","content":"package httpapi"}]}`
	analyze := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/issues/analyze", body)
	if analyze.Code != http.StatusAccepted {
		t.Fatalf("analyze status = %d: %s", analyze.Code, analyze.Body.String())
	}
	var queued struct {
		TaskID    string `json:"task_id"`
		SessionID string `json:"session_id"`
		Status    string `json:"status"`
	}
	decodeResponse(t, analyze, &queued)
	if queued.TaskID == "" || queued.SessionID == "" || queued.Status != domain.TaskQueued {
		t.Fatalf("unexpected queued response: %+v", queued)
	}

	task := waitTask(t, server.Router(), queued.TaskID)
	if task.Status != domain.TaskCompleted || !strings.Contains(task.PlanMarkdown, "# Implementation Plan") {
		t.Fatalf("unexpected completed task: %+v", task)
	}

	plan := request(t, server.Router(), http.MethodGet, "/ai/issues/42/plan", "")
	if plan.Code != http.StatusOK || !strings.Contains(plan.Body.String(), `"revision":1`) {
		t.Fatalf("plan response = %d: %s", plan.Code, plan.Body.String())
	}

	correct := request(t, server.Router(), http.MethodPost, "/ai/issues/42/correct", `{"feedback":"Add integration tests"}`)
	if correct.Code != http.StatusAccepted {
		t.Fatalf("correct status = %d: %s", correct.Code, correct.Body.String())
	}
	var correction struct {
		TaskID string `json:"task_id"`
	}
	decodeResponse(t, correct, &correction)
	waitTask(t, server.Router(), correction.TaskID)

	generator.mu.Lock()
	if len(generator.requests) != 2 || generator.requests[1].PreviousPlan == nil || *generator.requests[1].PreviousPlan == "" || generator.requests[1].CorrectionFeedback == nil || *generator.requests[1].CorrectionFeedback != "Add integration tests" {
		t.Fatalf("correction request did not include previous plan and feedback: %+v", generator.requests)
	}
	generator.mu.Unlock()

	approve := request(t, server.Router(), http.MethodPost, "/ai/issues/42/approve", "")
	if approve.Code != http.StatusAccepted || !strings.Contains(approve.Body.String(), `"generated_files_contract"`) || !strings.Contains(approve.Body.String(), `"reviewer":"artur"`) || !strings.Contains(approve.Body.String(), `"task_id"`) {
		t.Fatalf("approve response = %d: %s", approve.Code, approve.Body.String())
	}
	var approved struct {
		TaskID string `json:"task_id"`
	}
	decodeResponse(t, approve, &approved)
	codegenTask := waitTask(t, server.Router(), approved.TaskID)
	if codegenTask.Status != domain.TaskCompleted || codegenTask.GeneratedFiles == nil || len(codegenTask.GeneratedFiles.Files) != 1 {
		t.Fatalf("unexpected code generation task: %+v", codegenTask)
	}
	status := request(t, server.Router(), http.MethodGet, "/ai/issues/42/code-generation", "")
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"type":"code_generation"`) || !strings.Contains(status.Body.String(), `"action":"modify"`) {
		t.Fatalf("code generation status = %d: %s", status.Code, status.Body.String())
	}
	generator.mu.Lock()
	if len(generator.fileRequests) != 1 || generator.fileRequests[0].ApprovedPlanMarkdown == "" {
		t.Fatalf("code generation request did not include approved plan: %+v", generator.fileRequests)
	}
	generator.mu.Unlock()
}

func TestApproveUsesEditedPlanForCodeGeneration(t *testing.T) {
	generator := &fakeGenerator{}
	server := NewWithDependencies(repository.NewMemoryStore(), generator)
	body := `{"repository":{"id":"repo-1","default_branch":"main"},"issue":{"id":"44","title":"Use edited plan","body":"Apply user edits","author":"artur"},"yaml_config":"version: 1","repository_files":[{"path":"README.md","content":"# Backend"}]}`
	analyze := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/issues/analyze", body)
	var queued struct {
		TaskID string `json:"task_id"`
	}
	decodeResponse(t, analyze, &queued)
	waitTask(t, server.Router(), queued.TaskID)

	editedPlan := `# Implementation Plan

## Issue Summary
Apply the manually edited user plan.

## Goal
Ensure code generation receives the final approved markdown.

## Relevant Files
- ` + "`README.md`" + `: contains project documentation.

## Proposed Changes
- Update documentation using the edited plan.

## Implementation Steps
1. Modify README.md.

## Expected Files to Change
- ` + "`README.md`" + `: modify.

## Tests and Verification
- Run Go integration tests.

## Risks and Open Questions
- TBD.
`
	approve := request(t, server.Router(), http.MethodPost, "/ai/issues/44/approve", fmt.Sprintf(`{"plan_markdown":%q}`, editedPlan))
	if approve.Code != http.StatusAccepted {
		t.Fatalf("approve status = %d: %s", approve.Code, approve.Body.String())
	}
	var approved struct {
		TaskID string `json:"task_id"`
	}
	decodeResponse(t, approve, &approved)
	waitTask(t, server.Router(), approved.TaskID)

	generator.mu.Lock()
	defer generator.mu.Unlock()
	if len(generator.fileRequests) != 1 || generator.fileRequests[0].ApprovedPlanMarkdown != strings.TrimSpace(editedPlan) {
		t.Fatalf("code generation did not use edited plan: %+v", generator.fileRequests)
	}
}

func TestGitFlameWebhookFetchesRepositoryContextAndQueuesAnalyze(t *testing.T) {
	generator := &fakeGenerator{}
	source := &fakeGitFlameSource{result: domain.IssueAnalyzeRequest{
		Repository:      domain.RepositoryMetadata{ID: "repo-webhook", DefaultBranch: "main", CommitSHA: "abc"},
		Issue:           domain.IssuePayload{ID: "99", Title: "Webhook issue", Body: "Triggered from GitFlame", Author: "artur"},
		YAMLConfig:      "version: 1",
		RepositoryFiles: []domain.RepositoryFile{{Path: "README.md", Content: "# Backend"}},
	}}
	server := NewWithDependenciesAndIntegrations(repository.NewMemoryStore(), generator, source, nil)
	webhook := `{"event":"issue.updated","repository":{"id":"repo-webhook","default_branch":"main"},"issue":{"id":"99","title":"Webhook issue","body":"Triggered from GitFlame","author":"artur"},"commit_sha":"abc"}`
	response := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/webhooks/issues", webhook)
	if response.Code != http.StatusAccepted {
		t.Fatalf("webhook status = %d: %s", response.Code, response.Body.String())
	}
	if source.request.CommitSHA != "abc" || source.request.Repository.ID != "repo-webhook" {
		t.Fatalf("webhook was not passed to GitFlame source: %+v", source.request)
	}
	var queued struct {
		TaskID string `json:"task_id"`
	}
	decodeResponse(t, response, &queued)
	if completed := waitTask(t, server.Router(), queued.TaskID); completed.Status != domain.TaskCompleted {
		t.Fatalf("webhook task did not complete: %+v", completed)
	}
}

func TestApplyGeneratedFilesCreatesGitFlamePullRequest(t *testing.T) {
	generator := &fakeGenerator{}
	source := &fakeGitFlameSource{}
	server := NewWithDependenciesAndIntegrations(repository.NewMemoryStore(), generator, source, nil)
	body := `{"repository":{"id":"repo-apply","default_branch":"main","commit_sha":"abc123"},"issue":{"id":"45","title":"Apply files","body":"Create PR","author":"artur"},"yaml_config":"version: 1","repository_files":[{"path":"README.md","content":"# Backend"}]}`
	analyze := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/issues/analyze", body)
	var queued struct {
		TaskID string `json:"task_id"`
	}
	decodeResponse(t, analyze, &queued)
	waitTask(t, server.Router(), queued.TaskID)
	approve := request(t, server.Router(), http.MethodPost, "/ai/issues/45/approve", "")
	var approved struct {
		TaskID string `json:"task_id"`
	}
	decodeResponse(t, approve, &approved)
	waitTask(t, server.Router(), approved.TaskID)

	applied := request(t, server.Router(), http.MethodPost, "/ai/issues/45/gitflame/apply", "")
	if applied.Code != http.StatusOK {
		t.Fatalf("apply status = %d: %s", applied.Code, applied.Body.String())
	}
	if source.applyRepository.ID != "repo-apply" || source.applyContract.BranchName == "" || len(source.applyContract.Files) != 1 {
		t.Fatalf("GitFlame apply did not receive generated files contract: repo=%+v contract=%+v", source.applyRepository, source.applyContract)
	}
	if !strings.Contains(applied.Body.String(), `"pull_request_url":"https://gitflame.test/pulls/7"`) || !strings.Contains(applied.Body.String(), `"apply_status":"applied"`) {
		t.Fatalf("apply response did not include PR result: %s", applied.Body.String())
	}
	status := request(t, server.Router(), http.MethodGet, "/ai/issues/45/code-generation", "")
	if !strings.Contains(status.Body.String(), `"pull_request_url":"https://gitflame.test/pulls/7"`) {
		t.Fatalf("stored code generation contract missed PR URL: %s", status.Body.String())
	}
}

func TestRecommendationsUseExternalServiceAndPersistCards(t *testing.T) {
	recommender := &fakeRecommender{}
	server := NewWithDependenciesAndIntegrations(repository.NewMemoryStore(), &fakeGenerator{}, nil, recommender)
	body := `{"repository":{"id":"repo-rec","default_branch":"main"},"yaml_config":"version: 1","categories":["security"],"repository_files":[{"path":"src/app.go","content":"package app"}]}`
	response := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/repositories/repo-rec/recommendations/analyze", body)
	if response.Code != http.StatusOK {
		t.Fatalf("recommendations status = %d: %s", response.Code, response.Body.String())
	}
	if recommender.configYAML != "version: 1" || len(recommender.files) != 1 || recommender.files[0].Path != "src/app.go" {
		t.Fatalf("recommendation service was not called with repository files: %+v", recommender)
	}
	if !strings.Contains(response.Body.String(), `"category":"security"`) || strings.Contains(response.Body.String(), "fallback") {
		t.Fatalf("recommendation response did not persist ML card: %s", response.Body.String())
	}
	list := request(t, server.Router(), http.MethodGet, "/repositories/repo-rec/recommendations", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"state":"open"`) {
		t.Fatalf("persisted recommendations = %d: %s", list.Code, list.Body.String())
	}
}

func TestRecommendationsSupportRepositoryIDWithSlash(t *testing.T) {
	recommender := &fakeRecommender{}
	server := NewWithDependenciesAndIntegrations(repository.NewMemoryStore(), &fakeGenerator{}, nil, recommender)
	body := `{"repository":{"id":"owner/repo","default_branch":"main"},"yaml_config":"version: 1","repository_files":[{"path":"src/app.go","content":"package app"}]}`
	response := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/recommendations/analyze?repository_id=owner%2Frepo", body)
	if response.Code != http.StatusOK {
		t.Fatalf("recommendations status = %d: %s", response.Code, response.Body.String())
	}
	list := request(t, server.Router(), http.MethodGet, "/repositories/recommendations?repository_id=owner%2Frepo", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"repository_id":"owner/repo"`) || !strings.Contains(list.Body.String(), `"state":"open"`) {
		t.Fatalf("persisted slash-id recommendations = %d: %s", list.Code, list.Body.String())
	}
	summary := request(t, server.Router(), http.MethodGet, "/repositories/recommendations/summary?repository_id=owner%2Frepo", "")
	if summary.Code != http.StatusOK || !strings.Contains(summary.Body.String(), `"repository_id":"owner/repo"`) {
		t.Fatalf("slash-id summary = %d: %s", summary.Code, summary.Body.String())
	}
}

func TestRecommendationsDisabledByConfigReturnsEmptyReport(t *testing.T) {
	recommender := &fakeRecommender{}
	server := NewWithDependenciesAndIntegrations(repository.NewMemoryStore(), &fakeGenerator{}, nil, recommender)
	body := `{"repository":{"id":"repo-no-rec","default_branch":"main"},"yaml_config":"repository:\n  default_branch: main\nanalysis:\n  enabled: true\n  exclude:\n    []\nrecommendations:\n  enabled: false\n  categories:\n    []\nstorage:\n  recommendation_ttl_days: 30\n","repository_files":[{"path":"src/app.go","content":"package app"}]}`
	response := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/repositories/repo-no-rec/recommendations/analyze", body)
	if response.Code != http.StatusOK {
		t.Fatalf("recommendations status = %d: %s", response.Code, response.Body.String())
	}
	if recommender.configYAML != "" || len(recommender.files) != 0 {
		t.Fatalf("recommendation service should not be called when disabled: %+v", recommender)
	}
	if !strings.Contains(response.Body.String(), `"recommendations":[]`) {
		t.Fatalf("disabled recommendations should return an empty report: %s", response.Body.String())
	}
}

func TestGitFlameConnectionStoresEncryptedTokenAndSessionCookie(t *testing.T) {
	store := repository.NewMemoryStore()
	source := &fakeGitFlameSource{}
	server := NewWithDependenciesAndIntegrations(store, &fakeGenerator{}, source, nil)
	cipher, err := security.NewCredentialCipher("12345678901234567890123456789012", 1)
	if err != nil {
		t.Fatal(err)
	}
	server.credentialCipher = cipher

	response := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/connections", `{"access_token":"secret-access-token","repo_url":"https://gitflame.test/tiroro-20-10/test42/code","scopes":["repo:read","repo:write"]}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("connection status = %d: %s", response.Code, response.Body.String())
	}
	setCookie := response.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "codepilot_session=") || !strings.Contains(setCookie, "HttpOnly") || strings.Contains(setCookie, "secret-access-token") {
		t.Fatalf("session cookie leaked token or missed flags: %s", setCookie)
	}
	var saved domain.GitFlameConnection
	decodeResponse(t, response, &saved)
	if saved.UserID == "" || saved.TokenLast4 != "oken" || saved.TokenStatus != "active" || saved.Repository.ID != "tiroro-20-10/test42" {
		t.Fatalf("unexpected connection response: %+v", saved)
	}
	loaded, err := store.UserGitFlameConnection(saved.UserID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccessTokenEncrypted != "" || len(loaded.TokenMaterial.Ciphertext) == 0 || len(loaded.TokenMaterial.Nonce) == 0 || loaded.TokenMaterial.KeyVersion != 1 {
		t.Fatalf("connection was not stored with AES-GCM material: %+v", loaded)
	}
	plaintext, err := cipher.Decrypt(loaded.TokenMaterial.Ciphertext, loaded.TokenMaterial.Nonce, loaded.TokenMaterial.KeyVersion, loaded.UserID+":"+loaded.Repository.ID)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != "secret-access-token" {
		t.Fatalf("decrypted token mismatch: %q", plaintext)
	}

	revoke := requestWithCookie(t, server.Router(), http.MethodDelete, "/integrations/gitflame/connections/"+saved.ID, "", setCookie)
	if revoke.Code != http.StatusOK {
		t.Fatalf("revoke status = %d: %s", revoke.Code, revoke.Body.String())
	}
	revoked, err := store.UserGitFlameConnection(saved.UserID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.TokenStatus != "revoked" || revoked.RevokedAt == nil {
		t.Fatalf("connection was not revoked: %+v", revoked)
	}
}

func TestApplyGeneratedFilesUsesStoredGitFlameConnectionToken(t *testing.T) {
	var applyCalls int
	gitflameAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user" {
			if r.Header.Get("Authorization") != "Bearer secret-access-token" {
				t.Fatalf("current user auth header = %q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "gitflame-user-1", "username": "artur"})
			return
		}
		if r.Header.Get("Authorization") != "Bearer secret-access-token" {
			t.Fatalf("apply auth header = %q for %s", r.Header.Get("Authorization"), r.URL.Path)
		}
		applyCalls++
		switch r.URL.Path {
		case "/api/v1/repositories/repo-apply-secure/branches":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "created"})
		case "/api/v1/repositories/repo-apply-secure/commits":
			_ = json.NewEncoder(w).Encode(map[string]string{"sha": "secure-commit"})
		case "/api/v1/repositories/repo-apply-secure/pull-requests":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "9", "url": "https://gitflame.test/pulls/9"})
		default:
			t.Fatalf("unexpected GitFlame path: %s", r.URL.Path)
		}
	}))
	defer gitflameAPI.Close()

	store := repository.NewMemoryStore()
	server := NewWithDependencies(store, &fakeGenerator{})
	cipher, err := security.NewCredentialCipher("12345678901234567890123456789012", 1)
	if err != nil {
		t.Fatal(err)
	}
	server.credentialCipher = cipher
	server.gitflameBaseURL = gitflameAPI.URL
	server.gitflameTimeout = time.Second

	connection := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/connections", `{"access_token":"secret-access-token","repository":{"id":"repo-apply-secure","name":"secure","default_branch":"main"}}`)
	if connection.Code != http.StatusCreated {
		t.Fatalf("connection status = %d: %s", connection.Code, connection.Body.String())
	}
	cookie := connection.Header().Get("Set-Cookie")
	body := `{"repository":{"id":"repo-apply-secure","default_branch":"main","commit_sha":"abc123"},"issue":{"id":"46","title":"Apply with stored token","body":"Create PR","author":"artur"},"yaml_config":"version: 1","repository_files":[{"path":"README.md","content":"# Backend"}]}`
	analyze := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/issues/analyze", body)
	var queued struct {
		TaskID string `json:"task_id"`
	}
	decodeResponse(t, analyze, &queued)
	waitTask(t, server.Router(), queued.TaskID)
	approve := request(t, server.Router(), http.MethodPost, "/ai/issues/46/approve", "")
	var approved struct {
		TaskID string `json:"task_id"`
	}
	decodeResponse(t, approve, &approved)
	waitTask(t, server.Router(), approved.TaskID)

	applied := requestWithCookie(t, server.Router(), http.MethodPost, "/ai/issues/46/gitflame/apply", "", cookie)
	if applied.Code != http.StatusOK {
		t.Fatalf("apply status = %d: %s", applied.Code, applied.Body.String())
	}
	if applyCalls != 3 || !strings.Contains(applied.Body.String(), `"commit_sha":"secure-commit"`) {
		t.Fatalf("apply did not use stored token/client: calls=%d body=%s", applyCalls, applied.Body.String())
	}
	loaded, err := store.UserGitFlameConnectionByRepository(mustConnectionUserID(t, connection), "repo-apply-secure")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastUsedAt == nil {
		t.Fatalf("connection last_used_at was not updated: %+v", loaded)
	}
}

func TestAgentEngineErrorIsStoredOnTask(t *testing.T) {
	generator := &fakeGenerator{err: &agent.Error{Status: http.StatusServiceUnavailable, Code: "model_unavailable", Detail: "model is loading"}}
	server := NewWithDependencies(repository.NewMemoryStore(), generator)
	body := `{"repository":{"id":"repo-1","default_branch":"main"},"issue":{"id":"43","title":"Generate plan","body":"Please generate","author":"artur"},"yaml_config":"version: 1","repository_files":[{"path":"README.md","content":"# Backend"}]}`
	response := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/issues/analyze", body)
	var queued struct {
		TaskID string `json:"task_id"`
	}
	decodeResponse(t, response, &queued)
	task := waitTask(t, server.Router(), queued.TaskID)
	if task.Status != domain.TaskFailed || task.Error == nil || task.Error.HTTPStatus != 503 || task.Error.Code != "model_unavailable" {
		t.Fatalf("unexpected failed task: %+v", task)
	}
	generator.err = nil
	retry := request(t, server.Router(), http.MethodPost, "/ai/tasks/"+queued.TaskID+"/retry", "")
	if retry.Code != http.StatusAccepted {
		t.Fatalf("retry status=%d: %s", retry.Code, retry.Body.String())
	}
	var retried struct {
		TaskID string `json:"task_id"`
	}
	decodeResponse(t, retry, &retried)
	if completed := waitTask(t, server.Router(), retried.TaskID); completed.Status != domain.TaskCompleted {
		t.Fatalf("retried task=%+v", completed)
	}
}

func TestValidationAndOpenAPI(t *testing.T) {
	server := NewWithDependencies(repository.NewMemoryStore(), &fakeGenerator{})
	ready := request(t, server.Router(), http.MethodGet, "/ready", "")
	if ready.Code != http.StatusOK || !strings.Contains(ready.Body.String(), `"storage":"ready"`) {
		t.Fatalf("ready response = %d: %s", ready.Code, ready.Body.String())
	}
	invalid := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/issues/analyze", `{}`)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("validation status = %d", invalid.Code)
	}
	spec := request(t, server.Router(), http.MethodGet, "/openapi.json", "")
	var document map[string]any
	decodeResponse(t, spec, &document)
	if document["openapi"] != "3.0.3" || !strings.Contains(spec.Body.String(), "/ai/tasks/{taskId}") ||
		!strings.Contains(spec.Body.String(), "/ai/issues/{id}/code-generation") ||
		!strings.Contains(spec.Body.String(), "/ai/issues/{id}/gitflame/apply") ||
		!strings.Contains(spec.Body.String(), "/integrations/gitflame/webhooks/issues") ||
		!strings.Contains(spec.Body.String(), "pull_request_url") ||
		!strings.Contains(spec.Body.String(), "ApprovePlanRequest") ||
		!strings.Contains(spec.Body.String(), "RecommendationAnalyzeRequest") ||
		!strings.Contains(spec.Body.String(), `"code_generation"`) ||
		!strings.Contains(spec.Body.String(), `"/ready"`) {
		t.Fatal("Sprint 4 API contract is missing from OpenAPI")
	}
}

func TestConnectionSetupWithoutGitFlameBaseURLReturnsServiceUnavailable(t *testing.T) {
	server, err := New(config.Config{SessionCookieName: "codepilot_session", SessionTTL: time.Hour, GitFlameCredentialKey: "12345678901234567890123456789012", GitFlameCredentialKeyVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	response := request(t, server.Router(), http.MethodPost, "/integrations/gitflame/connections", `{"access_token":"secret-access-token","repo_url":"https://gitflame.test/tiroro-20-10/test42"}`)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"code":"gitflame_client_unavailable"`) {
		t.Fatalf("connection status = %d: %s", response.Code, response.Body.String())
	}
}

func request(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var source *bytes.Reader
	if body == "" {
		source = bytes.NewReader(nil)
	} else {
		source = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, source)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func requestWithCookie(t *testing.T, handler http.Handler, method, path, body, setCookie string) *httptest.ResponseRecorder {
	t.Helper()
	var source *bytes.Reader
	if body == "" {
		source = bytes.NewReader(nil)
	} else {
		source = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, source)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie := strings.Split(setCookie, ";")[0]; cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
}

func mustConnectionUserID(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var connection domain.GitFlameConnection
	decodeResponse(t, response, &connection)
	if connection.UserID == "" {
		t.Fatal("connection response did not include user_id")
	}
	return connection.UserID
}

func waitTask(t *testing.T, handler http.Handler, id string) taskResponse {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		response := request(t, handler, http.MethodGet, "/ai/tasks/"+id, "")
		var task taskResponse
		decodeResponse(t, response, &task)
		if task.Status == domain.TaskCompleted || task.Status == domain.TaskFailed {
			return task
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("task did not finish")
	return taskResponse{}
}
