// In-memory mock of the GitFlame CodePilot backend.
//
// It returns the SAME response shapes as the Go backend so that switching to the
// real backend (via VITE_API_BASE) requires no UI changes. The mock also adds a
// little latency so loading states are visible in the demo and in GIFs.
//
// Card shape (matches backend RecommendationCard + the optional `category`
// field from the ML recommendation_schema.json):
//   { id, severity, category?, file, line?, problem, suggestion, confidence?, state }

import { ApiError } from './client.js'

const LATENCY = 650 // ms, makes the loading spinner visible
const delay = (ms = LATENCY) => new Promise((r) => setTimeout(r, ms))

let nextId = 100
const uid = (prefix) => `${prefix}-${(nextId++).toString(36)}-${Date.now().toString(36)}`

// repositoryId -> { summary, recommendations[], status }
const reports = new Map()
// issueId -> session
const issueSessions = new Map()

function seedReport(repositoryId) {
  const recommendations = [
    {
      id: uid('rec'),
      severity: 'high',
      category: 'security',
      file: 'internal/app/server.go',
      line: 142,
      problem:
        'User-supplied repository identifiers are concatenated into log strings without sanitisation, which allows log injection.',
      suggestion:
        'Escape or encode the repository id before logging, or use structured logging with separate fields.',
      confidence: 0.86,
      state: 'open',
    },
    {
      id: uid('rec'),
      severity: 'high',
      category: 'performance',
      file: 'internal/app/storage.go',
      line: 188,
      problem:
        'DeleteRecommendation rescans every report on each call, giving O(reports × cards) behaviour that will not scale.',
      suggestion:
        'Maintain a recommendation-id → report index so lookups and deletes become O(1).',
      confidence: 0.74,
      state: 'open',
    },
    {
      id: uid('rec'),
      severity: 'medium',
      category: 'code_duplication',
      file: 'internal/app/services.go',
      line: 61,
      problem:
        'valueAfterKey and hasDisabledAnalysis both reimplement line-by-line YAML parsing of the same config.',
      suggestion:
        'Extract a single small YAML reader used by both functions to remove duplicated scanning logic.',
      confidence: 0.69,
      state: 'open',
    },
    {
      id: uid('rec'),
      severity: 'medium',
      category: 'maintainability',
      file: 'cmd/server/main.go',
      line: 14,
      problem: 'There is no graceful shutdown, so in-flight requests can be dropped on restart.',
      suggestion:
        'Wrap http.Server with signal handling and call Shutdown(ctx) on SIGTERM/SIGINT.',
      confidence: 0.71,
      state: 'open',
    },
    {
      id: uid('rec'),
      severity: 'low',
      category: 'architecture',
      file: 'internal/app/server.go',
      line: 38,
      problem:
        'Routing is done with manual string prefix matching, which is error-prone as the API grows.',
      suggestion:
        'Adopt the Go 1.22 method+pattern router consistently (it is already used for some routes).',
      confidence: 0.63,
      state: 'open',
    },
  ]
  const summary =
    'Repository analysis found 5 items: 2 high, 2 medium and 1 low. The highest-impact issues are a potential log-injection point and an O(n) recommendation delete path. No blocking defects were detected.'
  reports.set(repositoryId, { repositoryId, summary, recommendations, status: 'ready' })
}

function buildPlan(issue) {
  return `# Implementation Plan

## Issue
${issue.title}

## Context
${issue.body ? issue.body.trim() : 'No additional description was provided.'}

## Steps
1. Validate the repository \`.yml\` configuration and branch rules.
2. Review the issue and the relevant repository files in context.
3. Identify the files most likely to change for this request.
4. Implement the change on an AI-generated branch after user approval.
5. Open a pull request and assign the issue author as reviewer.

## Acceptance
- The change compiles and existing checks pass.
- A reviewer has approved the generated pull request.
`
}

export const mockApi = {
  async getHealth() {
    await delay(150)
    return { status: 'ok', service: 'mock' }
  },

  // --- Recommendation flow ---
  async analyzeRepository(repositoryId) {
    await delay(1100) // analysis is "slower"
    seedReport(repositoryId)
    const report = reports.get(repositoryId)
    return {
      repository_id: repositoryId,
      status: report.status,
      summary: report.summary,
      recommendations: report.recommendations,
    }
  },

  async getRecommendationStatus(repositoryId) {
    await delay()
    const report = reports.get(repositoryId)
    if (!report) throw new ApiError('recommendation report was not found for repository', 404)
    const closed = report.recommendations.filter((r) => r.state === 'closed').length
    const total = report.recommendations.length
    return {
      repository_id: repositoryId,
      status: report.status,
      total,
      open: total - closed,
      closed,
    }
  },

  async getRecommendationSummary(repositoryId) {
    await delay()
    const report = reports.get(repositoryId)
    if (!report) throw new ApiError('recommendation report was not found for repository', 404)
    return { repository_id: repositoryId, summary: report.summary }
  },

  async listRecommendations(repositoryId) {
    await delay()
    const report = reports.get(repositoryId)
    if (!report) throw new ApiError('recommendation report was not found for repository', 404)
    return { repository_id: repositoryId, recommendations: report.recommendations.map((r) => ({ ...r })) }
  },

  async closeRecommendation(recommendationId) {
    await delay(300)
    for (const report of reports.values()) {
      const card = report.recommendations.find((r) => r.id === recommendationId)
      if (card) {
        card.state = 'closed'
        return { ...card }
      }
    }
    throw new ApiError('recommendation was not found', 404)
  },

  async deleteRecommendation(recommendationId) {
    await delay(300)
    for (const report of reports.values()) {
      const idx = report.recommendations.findIndex((r) => r.id === recommendationId)
      if (idx !== -1) {
        report.recommendations.splice(idx, 1)
        return null
      }
    }
    throw new ApiError('recommendation was not found', 404)
  },

  // --- Issue → plan flow ("Work with AI") ---
  async analyzeIssue(payload) {
    await delay(1100)
    if (!payload.yaml_config || !payload.yaml_config.trim()) {
      throw new ApiError('missing .yml configuration', 422)
    }
    const issue = payload.issue || {}
    if (!issue.title || !issue.title.trim()) {
      throw new ApiError('issue.title is required', 422)
    }
    const plan = buildPlan(issue)
    const session = {
      sessionId: uid('sess'),
      issueId: issue.id || uid('issue'),
      repositoryId: payload.repository?.id || 'demo-repo',
      status: 'plan_generated',
      planMarkdown: plan,
      revision: 1,
      author: issue.author || 'demo-user',
      webUrl: payload.repository?.web_url || '',
    }
    issueSessions.set(session.issueId, session)
    return {
      session_id: session.sessionId,
      issue_id: session.issueId,
      repository_id: session.repositoryId,
      status: session.status,
      plan_markdown: session.planMarkdown,
      comment_body: session.planMarkdown,
      next_actions: { approve: '/approve', correct: '/correct', reject: '/reject' },
    }
  },

  async getIssuePlan(issueId) {
    await delay()
    const session = issueSessions.get(issueId)
    if (!session) throw new ApiError('issue session was not found', 404)
    return {
      session_id: session.sessionId,
      issue_id: session.issueId,
      repository_id: session.repositoryId,
      status: session.status,
      plan_markdown: session.planMarkdown,
      comment_body: session.planMarkdown,
      revision: session.revision,
    }
  },

  async approveIssue(issueId) {
    await delay(700)
    const session = issueSessions.get(issueId)
    if (!session) throw new ApiError('issue session was not found', 404)
    session.status = 'approved'
    const slug = (session.title || issueId)
      .toString()
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/(^-|-$)/g, '')
      .slice(0, 48)
    const base = session.webUrl || `https://gitflame.local/${session.repositoryId}`
    const workflow = {
      branch_name: `ai/${session.issueId}-${slug || 'change'}`,
      pull_request_url: `${base.replace(/\/$/, '')}/-/merge_requests/mock-${session.issueId}`,
      reviewer: session.author,
      provider: 'mock',
    }
    return {
      session_id: session.sessionId,
      issue_id: session.issueId,
      status: session.status,
      message: 'Plan approved. Mock Git workflow payload was created.',
      git_workflow: workflow,
    }
  },

  async correctIssue(issueId, feedback) {
    await delay(900)
    const session = issueSessions.get(issueId)
    if (!session) throw new ApiError('issue session was not found', 404)
    if (!feedback || !feedback.trim()) throw new ApiError('feedback is required', 422)
    session.revision += 1
    session.status = 'correction_requested'
    session.planMarkdown +=
      `\n\n## Revision feedback\n- ${feedback}\n- Update implementation steps before approval.\n`
    return {
      session_id: session.sessionId,
      issue_id: session.issueId,
      status: session.status,
      message: 'Correction request was saved and the plan revision was updated.',
      plan_markdown: session.planMarkdown,
    }
  },

  async rejectIssue(issueId) {
    await delay(400)
    const session = issueSessions.get(issueId)
    if (!session) throw new ApiError('issue session was not found', 404)
    session.status = 'rejected'
    return {
      session_id: session.sessionId,
      issue_id: session.issueId,
      status: session.status,
      message: 'Plan rejected. External GitFlame can close the issue as not planned.',
    }
  },
}

// Pre-seed the demo repository so the widget shows data on first load.
seedReport('demo-repo')
