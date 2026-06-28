package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gitflame-codepilot/backend/internal/domain"
)

type PlanGenerator interface {
	GeneratePlan(context.Context, domain.AgentPlanRequest) (domain.AgentPlanResponse, error)
}

type FileGenerator interface {
	GenerateFiles(context.Context, domain.AgentCodeGenerationRequest) (domain.AgentGeneratedFilesResponse, error)
}

type Generator interface {
	PlanGenerator
	FileGenerator
}

type Error struct {
	Status       int
	Code, Detail string
}

func (e *Error) Error() string { return e.Detail }

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func (c *Client) Ready(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/ready", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Agent Engine readiness returned status %d", resp.StatusCode)
	}
	return nil
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: &http.Client{Timeout: timeout}}
}

func (c *Client) GeneratePlan(ctx context.Context, payload domain.AgentPlanRequest) (domain.AgentPlanResponse, error) {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return domain.AgentPlanResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/plans/generate", &body)
	if err != nil {
		return domain.AgentPlanResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return domain.AgentPlanResponse{}, &Error{Status: http.StatusGatewayTimeout, Code: "inference_timeout", Detail: "Agent Engine request timed out"}
		}
		return domain.AgentPlanResponse{}, &Error{Status: http.StatusBadGateway, Code: "agent_engine_unreachable", Detail: "Agent Engine is unreachable"}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var problem struct {
			Detail string `json:"detail"`
			Code   string `json:"code"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&problem)
		if problem.Detail == "" {
			problem.Detail = fmt.Sprintf("Agent Engine returned status %d", resp.StatusCode)
		}
		if problem.Code == "" {
			problem.Code = "agent_engine_error"
		}
		return domain.AgentPlanResponse{}, &Error{Status: normalizeStatus(resp.StatusCode), Code: problem.Code, Detail: problem.Detail}
	}
	var result domain.AgentPlanResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return result, &Error{Status: http.StatusBadGateway, Code: "invalid_agent_response", Detail: "Agent Engine returned invalid JSON"}
	}
	if result.RequestID != "" && result.RequestID != payload.RequestID {
		return result, &Error{Status: http.StatusBadGateway, Code: "request_id_mismatch", Detail: "Agent Engine returned a result for a different task"}
	}
	if result.Status != "" && result.Status != domain.TaskCompleted {
		return result, &Error{Status: http.StatusBadGateway, Code: "invalid_agent_status", Detail: fmt.Sprintf("Agent Engine returned unexpected status %q", result.Status)}
	}
	if strings.TrimSpace(result.PlanMarkdown) == "" {
		return result, &Error{Status: http.StatusUnprocessableEntity, Code: "empty_output", Detail: "Agent Engine returned an empty plan"}
	}
	return result, nil
}

func (c *Client) GenerateFiles(ctx context.Context, payload domain.AgentCodeGenerationRequest) (domain.AgentGeneratedFilesResponse, error) {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return domain.AgentGeneratedFilesResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/files/generate", &body)
	if err != nil {
		return domain.AgentGeneratedFilesResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return domain.AgentGeneratedFilesResponse{}, &Error{Status: http.StatusGatewayTimeout, Code: "inference_timeout", Detail: "Agent Engine code generation request timed out"}
		}
		return domain.AgentGeneratedFilesResponse{}, &Error{Status: http.StatusBadGateway, Code: "agent_engine_unreachable", Detail: "Agent Engine is unreachable"}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var problem struct {
			Detail string `json:"detail"`
			Code   string `json:"code"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&problem)
		if problem.Detail == "" {
			problem.Detail = fmt.Sprintf("Agent Engine returned status %d", resp.StatusCode)
		}
		if problem.Code == "" {
			problem.Code = "agent_engine_error"
		}
		return domain.AgentGeneratedFilesResponse{}, &Error{Status: normalizeStatus(resp.StatusCode), Code: problem.Code, Detail: problem.Detail}
	}
	var result domain.AgentGeneratedFilesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return result, &Error{Status: http.StatusBadGateway, Code: "invalid_agent_response", Detail: "Agent Engine returned invalid JSON"}
	}
	if result.RequestID != "" && result.RequestID != payload.RequestID {
		return result, &Error{Status: http.StatusBadGateway, Code: "request_id_mismatch", Detail: "Agent Engine returned a file contract for a different task"}
	}
	if result.Status != "" && result.Status != domain.TaskCompleted {
		return result, &Error{Status: http.StatusBadGateway, Code: "invalid_agent_status", Detail: fmt.Sprintf("Agent Engine returned unexpected status %q", result.Status)}
	}
	if len(result.Files) == 0 {
		return result, &Error{Status: http.StatusUnprocessableEntity, Code: "empty_output", Detail: "Agent Engine returned no generated file operations"}
	}
	return result, nil
}

func normalizeStatus(status int) int {
	switch status {
	case 400, 404, 422, 502, 503, 504:
		return status
	default:
		return http.StatusBadGateway
	}
}
