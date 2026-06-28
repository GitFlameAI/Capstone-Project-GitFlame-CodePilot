package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
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
		!strings.Contains(spec.Body.String(), `"code_generation"`) ||
		!strings.Contains(spec.Body.String(), `"/ready"`) {
		t.Fatal("Sprint 3 task endpoint is missing from OpenAPI")
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
