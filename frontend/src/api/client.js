// Real HTTP client for the GitFlame CodePilot Go backend.
//
// Endpoint paths and JSON field names below match the backend exactly
// (backend/internal/app/server.go and models.go). This client is only used when
// VITE_API_BASE is set; otherwise the app uses the in-memory mock (see mock.js).

const BASE = (import.meta.env.VITE_API_BASE || '').replace(/\/$/, '')

export class ApiError extends Error {
  constructor(message, status) {
    super(message)
    this.name = 'ApiError'
    this.status = status
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
  } catch (networkError) {
    throw new ApiError('Backend is unreachable. Is the Go service running on :8000?', 0)
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
    throw new ApiError(detail, res.status)
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

  // --- Issue → plan flow ("Work with AI") ---
  analyzeIssue: (payload) =>
    request('POST', '/integrations/gitflame/issues/analyze', payload),
  getIssuePlan: (issueId) =>
    request('GET', `/ai/issues/${encodeURIComponent(issueId)}/plan`),
  approveIssue: (issueId) =>
    request('POST', `/ai/issues/${encodeURIComponent(issueId)}/approve`),
  correctIssue: (issueId, feedback) =>
    request('POST', `/ai/issues/${encodeURIComponent(issueId)}/correct`, { feedback }),
  rejectIssue: (issueId) =>
    request('POST', `/ai/issues/${encodeURIComponent(issueId)}/reject`),
}
