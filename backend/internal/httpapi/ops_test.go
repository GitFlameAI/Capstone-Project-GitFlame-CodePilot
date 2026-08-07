package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/repository"
	"gitflame-codepilot/backend/internal/security"
)

// opsTestServer builds a server whose session cookie is already valid, so the
// tests exercise the endpoints rather than the login flow.
func opsTestServer(t *testing.T) (*Server, *http.Cookie) {
	t.Helper()
	store := repository.NewMemoryStore()
	server := &Server{
		store:         store,
		checks:        map[string]func(context.Context) error{"storage": store.Ping},
		indexJobs:     newIndexJobRegistry(),
		sessionCookie: "codepilot_session",
		sessionTTL:    time.Hour,
	}
	server.router = http.NewServeMux()
	route(server.router, "GET /ops/status", server.opsStatus)
	route(server.router, "GET /ops/tasks", server.opsTasks)
	route(server.router, "GET /ops/dead-letter", server.opsDeadLetter)

	user, err := store.UpsertAppUser(domain.AppUser{GitFlameUserID: "1", Username: "ops"})
	if err != nil {
		t.Fatalf("UpsertAppUser: %v", err)
	}
	token, tokenHash, err := security.GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken: %v", err)
	}
	if _, err := store.CreateAppSession(user.ID, tokenHash, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateAppSession: %v", err)
	}
	return server, &http.Cookie{Name: "codepilot_session", Value: token}
}

func opsRequest(t *testing.T, server *Server, cookie *http.Cookie, target string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	server.router.ServeHTTP(recorder, request)
	return recorder
}

func TestOpsEndpointsRequireASession(t *testing.T) {
	server, _ := opsTestServer(t)
	for _, target := range []string{"/ops/status", "/ops/tasks", "/ops/dead-letter"} {
		if recorder := opsRequest(t, server, nil, target); recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s without a session = %d, want 401", target, recorder.Code)
		}
	}
}

func TestOpsStatusReportsDependenciesAndDegradesGracefully(t *testing.T) {
	server, cookie := opsTestServer(t)
	server.checks["rag"] = func(context.Context) error { return errors.New("CodeRAG is unreachable") }

	recorder := opsRequest(t, server, cookie, "/ops/status")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response opsStatusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if response.Status != "degraded" {
		t.Fatalf("overall status = %q, want degraded", response.Status)
	}
	if len(response.Dependencies) != 2 || response.Dependencies[0].Component != "rag" {
		t.Fatalf("dependencies are missing or unsorted: %#v", response.Dependencies)
	}
	if response.Dependencies[0].Status != "down" || response.Dependencies[1].Status != "ok" {
		t.Fatalf("dependency states = %#v", response.Dependencies)
	}
	if response.Build.Service != "backend" {
		t.Fatalf("build info = %#v", response.Build)
	}
}

func TestOpsTasksReturnsSummariesWithoutContent(t *testing.T) {
	server, cookie := opsTestServer(t)
	store := server.store.(*repository.MemoryStore)

	session, _, err := store.CreateSession(domain.IssueAnalyzeRequest{
		Repository: domain.RepositoryMetadata{ID: "acme/project"},
		Issue:      domain.IssuePayload{ID: "1", Title: "Add a feature"},
	}, domain.AIConfig{})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	task, err := store.CreateTask(session.ID, "1", domain.TaskInitialPlan)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	task.Status = domain.TaskFailed
	task.PlanMarkdown = "SECRET PLAN CONTENT"
	task.Error = &domain.TaskError{HTTPStatus: 502, Code: "agent_engine_unreachable", Detail: "Agent Engine is unreachable"}
	if err := store.UpdateTask(task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	recorder := opsRequest(t, server, cookie, "/ops/tasks?limit=10")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	body := recorder.Body.String()
	if strings.Contains(body, "SECRET PLAN CONTENT") {
		t.Fatalf("task content leaked into the operational endpoint: %s", body)
	}

	var response struct {
		Tasks []repository.AgentTaskSummary `json:"tasks"`
		Count int                           `json:"count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if response.Count != 1 || response.Tasks[0].ID != task.ID {
		t.Fatalf("response = %#v", response)
	}
	if response.Tasks[0].ErrorCode != "agent_engine_unreachable" {
		t.Fatalf("error code = %q", response.Tasks[0].ErrorCode)
	}
}

func TestOpsTasksRejectsAnOutOfRangeLimit(t *testing.T) {
	server, cookie := opsTestServer(t)
	recorder := opsRequest(t, server, cookie, "/ops/tasks?limit=100000")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestOpsDeadLetterExplainsItselfWithoutRedis(t *testing.T) {
	server, cookie := opsTestServer(t)
	recorder := opsRequest(t, server, cookie, "/ops/dead-letter")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response struct {
		Count  int    `json:"count"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if response.Count != 0 || !strings.Contains(response.Detail, "TASK_DISPATCH_MODE") {
		t.Fatalf("response = %#v", response)
	}
}
