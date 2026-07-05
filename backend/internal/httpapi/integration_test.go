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
	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/repository"
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
	request GitFlameIssueWebhook
	result  domain.IssueAnalyzeRequest
	err     error
}

func (f *fakeGitFlameSource) BuildAnalyzeRequest(_ context.Context, req GitFlameIssueWebhook) (domain.IssueAnalyzeRequest, error) {
	f.request = req
	return f.result, f.err
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

func TestRecommendationsUseExternalServiceAndPersistCards(t *testing.T) {
	recommender := &fakeRecommender{}
	server := NewWithDependenciesAndIntegrations(repository.NewMemoryStore(), &fakeGenerator{}, nil, recommender)
	body := `{"repository":{"id":"repo-rec","default_branch":"main"},"yaml_config":"version: 1","repository_files":[{"path":"src/app.go","content":"package app"}]}`
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
		!strings.Contains(spec.Body.String(), "/integrations/gitflame/webhooks/issues") ||
		!strings.Contains(spec.Body.String(), "ApprovePlanRequest") ||
		!strings.Contains(spec.Body.String(), "RecommendationAnalyzeRequest") ||
		!strings.Contains(spec.Body.String(), `"code_generation"`) ||
		!strings.Contains(spec.Body.String(), `"/ready"`) {
		t.Fatal("Sprint 4 API contract is missing from OpenAPI")
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

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
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
