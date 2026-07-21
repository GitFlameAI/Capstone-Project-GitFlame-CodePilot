// Real HTTP client for the GitFlame CodePilot Go backend.
//
// Endpoint paths and JSON field names below match the backend exactly
// (backend/internal/httpapi/*.go + backend/internal/domain/domain.go).
// This client is only used when VITE_API_BASE is set; otherwise the app uses the
// in-memory mock (see mock.js), which simulates the same shapes.
//
// Sprint 5 changes (secure, cookie-based GitFlame connection flow):
//   - Every request is sent with `credentials: 'include'` so the backend's
//     HttpOnly `codepilot_session` cookie is carried on each call. The frontend
//     NEVER stores the GitFlame access token; it only sends it once to
//     POST /integrations/gitflame/connections (or PUT .../{id} on reconnect).
//   - New connection lifecycle: createConnection / reconnectConnection /
//     revokeConnection / logout.
//   - New apply step: applyToGitFlame() -> POST /ai/issues/{id}/gitflame/apply,
//     which opens the branch/commit/pull-request on GitFlame and returns the
//     contract with commit_sha + pull_request_url.
//   - approveIssue() now forwards the (optionally edited) plan_markdown.

const BASE = (import.meta.env.VITE_API_BASE || '').replace(/\/$/, '')

export class ApiError extends Error {
  // `code` is the backend's machine-readable error code (e.g. "validation_error",
  // "queue_unavailable", "gitflame_connection_required"). It lets the UI pick the
  // right state (see api/errors.js) without string-matching the human message.
  constructor(message, status, code = '') {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

function repositoryQuery(repositoryId) {
  return `repository_id=${encodeURIComponent(repositoryId)}`
}

async function request(method, path, body) {
  let res
  try {
    res = await fetch(BASE + path, {
      method,
      // Carry the HttpOnly session cookie on every request. Required for all
      // authenticated endpoints (connections, apply, and repository-scoped calls).
      credentials: 'include',
      headers: body ? { 'Content-Type': 'application/json' } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    })
  } catch {
    throw new ApiError('Backend is unreachable. Is the CodePilot service running?', 0, 'network_error')
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

// Build the request body for POST/PUT connection. The backend's authoritative
// contract only needs { access_token, repo_url }, and it parses the repo_url to
// derive authoritative repository metadata through GitFlame.
function connectionBody({ token, repoUrl, defaultBranch }) {
  const body = { access_token: token, repo_url: repoUrl }
  if (defaultBranch) body.default_branch = defaultBranch
  return body
}

export const httpApi = {
  getHealth: () => request('GET', '/health'),
  getReady: () => request('GET', '/ready'),

  // --- GitFlame connection lifecycle (Sprint 5) ---
  // POST /integrations/gitflame/connections
  //   Validates the token via GitFlame, creates the app user + server session,
  //   sets the HttpOnly codepilot_session cookie, stores the token AES-GCM
  //   encrypted, and returns connection metadata (no token). 201 Created.
  createConnection: (opts) =>
    request('POST', '/integrations/gitflame/connections', connectionBody(opts)),
  // PUT /integrations/gitflame/connections/{id} — replace the token / repo.
  reconnectConnection: (connectionId, opts) =>
    request(
      'PUT',
      `/integrations/gitflame/connections/${encodeURIComponent(connectionId)}`,
      connectionBody(opts),
    ),
  // DELETE /integrations/gitflame/connections/{id} — revoke the connection.
  revokeConnection: (connectionId) =>
    request('DELETE', `/integrations/gitflame/connections/${encodeURIComponent(connectionId)}`),
  // DELETE /auth/session — end the server session (clears the cookie).
  logout: () => request('DELETE', '/auth/session'),
  getRepositoryTree: (connectionId, ref) =>
    request(
      'GET',
      `/integrations/gitflame/connections/${encodeURIComponent(connectionId)}/tree${ref ? `?ref=${encodeURIComponent(ref)}` : ''}`,
    ),
  listRepositoryIssues: (connectionId) =>
    request('GET', `/integrations/gitflame/connections/${encodeURIComponent(connectionId)}/issues`),

  // --- Recommendation flow ---
  analyzeRepository: (repositoryId, payload) =>
    request(
      'POST',
      `/integrations/gitflame/recommendations/analyze?${repositoryQuery(repositoryId)}`,
      payload,
    ),
  getRecommendationStatus: (repositoryId) =>
    request('GET', `/repositories/recommendations/status?${repositoryQuery(repositoryId)}`),
  getRecommendationSummary: (repositoryId) =>
    request('GET', `/repositories/recommendations/summary?${repositoryQuery(repositoryId)}`),
  listRecommendations: (repositoryId) =>
    request('GET', `/repositories/recommendations?${repositoryQuery(repositoryId)}`),
  closeRecommendation: (recommendationId) =>
    request('PATCH', `/recommendations/${encodeURIComponent(recommendationId)}/close`),
  deleteRecommendation: (recommendationId) =>
    request('DELETE', `/recommendations/${encodeURIComponent(recommendationId)}`),

  // --- Issue -> plan -> approve -> code generation -> apply flow ---
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
  // Approve forwards the (optionally edited) plan the user reviewed, so code
  // generation uses exactly that plan. The backend body is { plan_markdown? }.
  approveIssue: (sessionOrIssueId, planMarkdown) =>
    request(
      'POST',
      `/ai/issues/${encodeURIComponent(sessionOrIssueId)}/approve`,
      planMarkdown && planMarkdown.trim() ? { plan_markdown: planMarkdown } : undefined,
    ),
  // Poll target for the code-generation task created on approve. Returns the
  // agent task plus generated_files_contract once completed.
  getCodeGeneration: (sessionOrIssueId) =>
    request('GET', `/ai/issues/${encodeURIComponent(sessionOrIssueId)}/code-generation`),
  // Sprint 5: apply the generated files to GitFlame (branch + commit + PR).
  // Uses the session cookie + the stored connection; returns the contract with
  // commit_sha + pull_request_url. No body — the backend already has the files.
  applyToGitFlame: (sessionOrIssueId) =>
    request('POST', `/ai/issues/${encodeURIComponent(sessionOrIssueId)}/gitflame/apply`),
  // Returns 202 with a new agent task (the correction is generated asynchronously).
  correctIssue: (sessionOrIssueId, feedback) =>
    request('POST', `/ai/issues/${encodeURIComponent(sessionOrIssueId)}/correct`, { feedback }),
  rejectIssue: (sessionOrIssueId) =>
    request('POST', `/ai/issues/${encodeURIComponent(sessionOrIssueId)}/reject`),
}
