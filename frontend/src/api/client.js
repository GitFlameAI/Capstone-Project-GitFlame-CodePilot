// Real HTTP client for the GitFlame CodePilot Go backend.
//
// Endpoint paths and JSON field names below match the Sprint 2 backend exactly
// (backend/internal/httpapi/server.go + backend/internal/domain/domain.go).
// This client is only used when VITE_API_BASE is set; otherwise the app uses the
// in-memory mock (see mock.js), which simulates the same async task lifecycle.
//
// Sprint 2 change: the "issue -> plan" flow is now ASYNCHRONOUS. analyzeIssue and
// correctIssue no longer return a plan directly; they return an agent task that
// must be polled via getTask() until it is `completed` or `failed`.

const BASE = (import.meta.env.VITE_API_BASE || '').replace(/\/$/, '')

export class ApiError extends Error {
  // `code` is the backend's machine-readable error code (e.g. "validation_error",
  // "queue_unavailable", "plan_not_ready"). It lets the UI pick the right state
  // without string-matching the human-readable detail.
  constructor(message, status, code = '') {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

async function request(method, path, body) {
  let res
  try {
    res = await fetch(BASE + path, {
      method,
      headers: body ? { 'Content-Type': 'application/json' } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    })
  } catch {
    throw new ApiError('Backend is unreachable. Is the Go service running on :8000?', 0, 'network_error')
  }

  if (res.status === 204) return null

  let payload = null
  const text = await res.text()
  if (text) {
    try {
      payload = JSON.parse(text)
    } catch {
      payload = { detail: text }
    }
  }

  if (!res.ok) {
    const detail = (payload && payload.detail) || `Request failed (${res.status})`
    const code = (payload && payload.code) || ''
    throw new ApiError(detail, res.status, code)
  }
  return payload
}

export const httpApi = {
  getHealth: () => request('GET', '/health'),

  // --- Recommendation flow ---
  analyzeRepository: (repositoryId, payload) =>
    request(
      'POST',
      `/integrations/gitflame/repositories/${encodeURIComponent(repositoryId)}/recommendations/analyze`,
      payload,
    ),
  getRecommendationStatus: (repositoryId) =>
    request('GET', `/repositories/${encodeURIComponent(repositoryId)}/recommendations/status`),
  getRecommendationSummary: (repositoryId) =>
    request('GET', `/repositories/${encodeURIComponent(repositoryId)}/recommendations/summary`),
  listRecommendations: (repositoryId) =>
    request('GET', `/repositories/${encodeURIComponent(repositoryId)}/recommendations`),
  closeRecommendation: (recommendationId) =>
    request('PATCH', `/recommendations/${encodeURIComponent(recommendationId)}/close`),
  deleteRecommendation: (recommendationId) =>
    request('DELETE', `/recommendations/${encodeURIComponent(recommendationId)}`),

  // --- Issue -> plan flow ("Work with AI"), async in Sprint 2 ---
  // Returns 202 { session_id, task_id, issue_id, repository_id, status, status_url }.
  analyzeIssue: (payload) =>
    request('POST', '/integrations/gitflame/issues/analyze', payload),
  // Poll target. Returns the agent task, including plan_markdown once completed.
  getTask: (taskId) =>
    request('GET', `/ai/tasks/${encodeURIComponent(taskId)}`),
  // Re-queues a failed-but-recoverable task. Returns 202 { task_id, status, ... }.
  retryTask: (taskId) =>
    request('POST', `/ai/tasks/${encodeURIComponent(taskId)}/retry`),
  // sessionOrIssueId: the backend accepts either the session_id or the issue_id.
  getIssuePlan: (sessionOrIssueId) =>
    request('GET', `/ai/issues/${encodeURIComponent(sessionOrIssueId)}/plan`),
  approveIssue: (sessionOrIssueId) =>
    request('POST', `/ai/issues/${encodeURIComponent(sessionOrIssueId)}/approve`),
  // Sprint 3: poll target for the code-generation task created on approve.
  // Returns the agent task plus generated_files_contract once completed.
  getCodeGeneration: (sessionOrIssueId) =>
    request('GET', `/ai/issues/${encodeURIComponent(sessionOrIssueId)}/code-generation`),
  // Returns 202 with a new agent task (the correction is generated asynchronously).
  correctIssue: (sessionOrIssueId, feedback) =>
    request('POST', `/ai/issues/${encodeURIComponent(sessionOrIssueId)}/correct`, { feedback }),
  rejectIssue: (sessionOrIssueId) =>
    request('POST', `/ai/issues/${encodeURIComponent(sessionOrIssueId)}/reject`),
}
