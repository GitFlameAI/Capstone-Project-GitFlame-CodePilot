// In-memory mock of the GitFlame CodePilot backend.
//
// It returns the SAME response shapes as the Go backend so that switching to the
// real backend (via VITE_API_BASE) requires no UI changes. It is used so the demo
// is fully runnable WITHOUT the Go backend, Redis, PostgreSQL and the Agent Engine
// (which needs a GPU). This keeps the master branch demoable for screenshots/GIFs.
//
// Sprint 2: the issue -> plan flow is asynchronous. analyzeIssue / correctIssue
// return an agent *task* that the UI polls via getTask() until it is `completed`
// or `failed`. The mock advances the task status over time so the queued ->
// processing -> completed progression is visible, and can simulate a recoverable
// failure (to demo the retry state) when the issue title contains "fail" or
// "timeout".
//
// Recommendation card shape (matches backend domain.RecommendationCard, plus the
// optional `category` field that the ML recommendation_schema.json provides):
//   { id, severity, category?, file, line?, problem, suggestion, confidence?, state }

import { ApiError } from './client.js'

const LATENCY = 600 // ms, makes the loading spinner visible
const delay = (ms = LATENCY) => new Promise((r) => setTimeout(r, ms))

let nextId = 100
const uid = (prefix) => `${prefix}-${(nextId++).toString(36)}-${Date.now().toString(36)}`

// repositoryId -> { summary, recommendations[], status }
const reports = new Map()
// sessionId -> session ; issueId -> sessionId (alias index)
const sessions = new Map()
const issueAlias = new Map()
// taskId -> task
const tasks = new Map()

// Task lifecycle timing (ms from creation).
const QUEUED_MS = 700
const PROCESSING_MS = 2000

function resolveSession(idOrIssueId) {
  if (sessions.has(idOrIssueId)) return sessions.get(idOrIssueId)
  if (issueAlias.has(idOrIssueId)) return sessions.get(issueAlias.get(idOrIssueId))
  return null
}

function plannedOutcome(issue) {
  const title = (issue.title || '').toLowerCase()
  if (title.includes('timeout')) return { kind: 'failed', status: 504, code: 'inference_timeout', detail: 'Agent Engine inference exceeded the configured timeout.' }
  if (title.includes('fail')) return { kind: 'failed', status: 502, code: 'agent_engine_error', detail: 'Agent Engine returned an unexpected error while generating the plan.' }
  return { kind: 'completed' }
}

function buildPlan(issue) {
  return `# Implementation Plan

## Issue summary
${issue.title}

## Context
${issue.body ? issue.body.trim() : 'No additional description was provided.'}

## Relevant files
- backend/internal/httpapi/server.go
- backend/internal/service/workflow.go

## Implementation steps
1. Validate the repository \`.yml\` configuration and branch rules.
2. Review the issue and the relevant repository files retrieved via RAG.
3. Identify the files most likely to change for this request.
4. Implement the change on an AI-generated branch after user approval.
5. Open a pull request and assign the issue author as reviewer.

## Tests
- Add unit tests for the new behaviour.
- Keep existing checks green.

## Risks
- Touching shared orchestration code may affect other flows.

## Open questions
- Should pagination defaults be configurable via \`.yml\`?
`
}

function taskView(task) {
  const elapsed = Date.now() - task.createdAt
  let status = 'queued'
  if (elapsed >= QUEUED_MS) status = 'processing'
  if (elapsed >= PROCESSING_MS) status = task.outcome.kind // completed | failed

  const base = {
    task_id: task.id,
    session_id: task.sessionId,
    issue_id: task.issueId,
    type: task.type,
    status,
    attempt: task.attempt,
  }

  if (status === 'completed') {
    // Apply terminal side-effects to the session exactly once.
    const session = sessions.get(task.sessionId)
    if (session) {
      session.planMarkdown = task.plan
      session.status = 'plan_generated'
      session.revision = task.revision
    }
    return {
      ...base,
      plan_markdown: task.plan,
      tool_execution_summary:
        'model=mock-coder; tool_calls=4; prompt_tokens=1820; completion_tokens=540; total_tokens=2360; generation_seconds=2.000',
      relevant_files: [
        { path: 'backend/internal/httpapi/server.go', reason: 'Owns the affected endpoint', create: false },
        { path: 'backend/internal/service/workflow.go', reason: 'Orchestration logic', create: false },
      ],
      model: 'mock-coder',
      usage: {
        prompt_tokens: 1820,
        completion_tokens: 540,
        total_tokens: 2360,
        tool_calls: 4,
        reasoning_chars: 0,
        generation_time_seconds: 2.0,
      },
    }
  }
  if (status === 'failed') {
    const session = sessions.get(task.sessionId)
    if (session) session.status = 'failed'
    return {
      ...base,
      error: {
        http_status: task.outcome.status,
        code: task.outcome.code,
        detail: task.outcome.detail,
      },
    }
  }
  return base
}

function seedReport(repositoryId) {
  const recommendations = [
    {
      id: uid('rec'), severity: 'high', category: 'security',
      file: 'backend/internal/httpapi/server.go', line: 142,
      problem: 'User-supplied repository identifiers are concatenated into log strings without sanitisation, which allows log injection.',
      suggestion: 'Escape or encode the repository id before logging, or use structured logging with separate fields.',
      confidence: 0.86, state: 'open',
    },
    {
      id: uid('rec'), severity: 'high', category: 'performance',
      file: 'backend/internal/repository/memory.go', line: 148,
      problem: 'DeleteRecommendation rescans every report on each call, giving O(reports x cards) behaviour that will not scale.',
      suggestion: 'Maintain a recommendation-id -> report index so lookups and deletes become O(1).',
      confidence: 0.74, state: 'open',
    },
    {
      id: uid('rec'), severity: 'medium', category: 'code_duplication',
      file: 'backend/internal/service/config.go', line: 61,
      problem: 'Two functions reimplement line-by-line YAML parsing of the same config.',
      suggestion: 'Extract a single small YAML reader used by both functions to remove duplicated scanning logic.',
      confidence: 0.69, state: 'open',
    },
    {
      id: uid('rec'), severity: 'medium', category: 'maintainability',
      file: 'backend/cmd/server/main.go', line: 14,
      problem: 'There is no graceful shutdown, so in-flight requests can be dropped on restart.',
      suggestion: 'Wrap http.Server with signal handling and call Shutdown(ctx) on SIGTERM/SIGINT.',
      confidence: 0.71, state: 'open',
    },
    {
      id: uid('rec'), severity: 'low', category: 'architecture',
      file: 'backend/internal/httpapi/server.go', line: 56,
      problem: 'Routing mixes manual prefix matching with the Go 1.22 method+pattern router.',
      suggestion: 'Adopt the Go 1.22 method+pattern router consistently across all routes.',
      confidence: 0.63, state: 'open',
    },
  ]
  const summary =
    'Repository analysis found 5 items: 2 high, 2 medium and 1 low. The highest-impact issues are a potential log-injection point and an O(n) recommendation delete path. No blocking defects were detected.'
  reports.set(repositoryId, { repositoryId, summary, recommendations, status: 'ready' })
}

export const mockApi = {
  async getHealth() {
    await delay(150)
    return { status: 'ok', service: 'mock' }
  },

  // --- Recommendation flow ---
  async analyzeRepository(repositoryId) {
    await delay(1100)
    seedReport(repositoryId)
    const report = reports.get(repositoryId)
    return { repository_id: repositoryId, status: report.status, summary: report.summary, recommendations: report.recommendations }
  },
  async getRecommendationStatus(repositoryId) {
    await delay()
    const report = reports.get(repositoryId)
    if (!report) throw new ApiError('recommendation report was not found for repository', 404, 'recommendations_not_found')
    const closed = report.recommendations.filter((r) => r.state === 'closed').length
    const total = report.recommendations.length
    return { repository_id: repositoryId, status: report.status, total, open: total - closed, closed }
  },
  async getRecommendationSummary(repositoryId) {
    await delay()
    const report = reports.get(repositoryId)
    if (!report) throw new ApiError('recommendation report was not found for repository', 404, 'recommendations_not_found')
    return { repository_id: repositoryId, summary: report.summary }
  },
  async listRecommendations(repositoryId) {
    await delay()
    const report = reports.get(repositoryId)
    if (!report) throw new ApiError('recommendation report was not found for repository', 404, 'recommendations_not_found')
    return { repository_id: repositoryId, recommendations: report.recommendations.map((r) => ({ ...r })) }
  },
  async closeRecommendation(recommendationId) {
    await delay(300)
    for (const report of reports.values()) {
      const card = report.recommendations.find((r) => r.id === recommendationId)
      if (card) { card.state = 'closed'; return { ...card } }
    }
    throw new ApiError('recommendation was not found', 404, 'recommendation_not_found')
  },
  async deleteRecommendation(recommendationId) {
    await delay(300)
    for (const report of reports.values()) {
      const idx = report.recommendations.findIndex((r) => r.id === recommendationId)
      if (idx !== -1) { report.recommendations.splice(idx, 1); return null }
    }
    throw new ApiError('recommendation was not found', 404, 'recommendation_not_found')
  },

  // --- Issue -> plan flow ("Work with AI"), async in Sprint 2 ---
  async analyzeIssue(payload) {
    await delay(500)
    if (!payload.yaml_config || !payload.yaml_config.trim()) {
      throw new ApiError('yaml_config is required', 422, 'validation_error')
    }
    const issue = payload.issue || {}
    if (!issue.title || !issue.title.trim()) throw new ApiError('issue.title is required', 422, 'validation_error')
    if (!issue.body || !issue.body.trim()) throw new ApiError('issue.body is required', 422, 'validation_error')
    if (!issue.author || !issue.author.trim()) throw new ApiError('issue.author is required', 422, 'validation_error')
    const ctx = payload.repository_files?.length || payload.repository_context?.length
    if (!ctx) throw new ApiError('repository_files must contain at least one file', 422, 'validation_error')

    const session = {
      sessionId: uid('sess'),
      issueId: issue.id || uid('issue'),
      repositoryId: payload.repository?.id || 'demo-repo',
      status: 'queued',
      planMarkdown: '',
      revision: 0,
      title: issue.title,
      author: issue.author || 'demo-user',
      yamlConfig: payload.yaml_config,
      targetBranchPrefix: 'ai/',
    }
    sessions.set(session.sessionId, session)
    issueAlias.set(session.issueId, session.sessionId)

    const task = {
      id: uid('task'), sessionId: session.sessionId, issueId: session.issueId,
      type: 'initial_plan', attempt: 1, createdAt: Date.now(),
      outcome: plannedOutcome(issue), plan: buildPlan(issue), revision: 1,
    }
    tasks.set(task.id, task)

    return {
      session_id: session.sessionId,
      task_id: task.id,
      issue_id: session.issueId,
      repository_id: session.repositoryId,
      status: 'queued',
      status_url: `/ai/tasks/${task.id}`,
    }
  },

  async getTask(taskId) {
    await delay(250)
    const task = tasks.get(taskId)
    if (!task) throw new ApiError('agent task was not found', 404, 'task_not_found')
    return taskView(task)
  },

  async retryTask(taskId) {
    await delay(300)
    const task = tasks.get(taskId)
    if (!task) throw new ApiError('agent task was not found', 404, 'task_not_found')
    // On retry the mock always succeeds, so the recovery path is demoable.
    const newTask = {
      id: uid('task'), sessionId: task.sessionId, issueId: task.issueId,
      type: task.type, attempt: task.attempt + 1, createdAt: Date.now(),
      outcome: { kind: 'completed' }, plan: task.plan, revision: task.revision,
    }
    tasks.set(newTask.id, newTask)
    const session = sessions.get(task.sessionId)
    if (session) session.status = 'queued'
    return {
      session_id: newTask.sessionId, issue_id: newTask.issueId, status: 'queued',
      message: 'Retry task queued.', task_id: newTask.id, status_url: `/ai/tasks/${newTask.id}`,
    }
  },

  async getIssuePlan(idOrIssueId) {
    await delay()
    const session = resolveSession(idOrIssueId)
    if (!session) throw new ApiError('issue session was not found', 404, 'session_not_found')
    if (!session.planMarkdown) throw new ApiError('plan generation has not completed', 409, 'plan_not_ready')
    return {
      session_id: session.sessionId, issue_id: session.issueId, repository_id: session.repositoryId,
      status: session.status, plan_markdown: session.planMarkdown, revision: session.revision,
    }
  },

  async approveIssue(idOrIssueId) {
    await delay(600)
    const session = resolveSession(idOrIssueId)
    if (!session) throw new ApiError('issue session was not found', 404, 'session_not_found')
    if (session.status !== 'plan_generated') {
      throw new ApiError(`plan cannot be approved while session status is ${session.status}`, 422, 'invalid_workflow_state')
    }
    session.status = 'approved'
    const slug = (session.title || idOrIssueId).toString().toLowerCase()
      .replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '').slice(0, 40)
    const contract = {
      branch_name: `${session.targetBranchPrefix}${session.issueId}-${slug || 'change'}`,
      files: [],
      commit_message: `Implement ${session.title}`,
      pr_title: session.title,
      reviewer: session.author,
    }
    session.generatedFiles = contract
    return {
      session_id: session.sessionId, issue_id: session.issueId, status: session.status,
      message: 'Plan approved. Contract is ready for a future code-generation worker.',
      generated_files_contract: contract,
    }
  },

  async correctIssue(idOrIssueId, feedback) {
    await delay(500)
    const session = resolveSession(idOrIssueId)
    if (!session) throw new ApiError('issue session was not found', 404, 'session_not_found')
    if (!feedback || !feedback.trim()) throw new ApiError('feedback is required', 422, 'validation_error')
    if (session.status !== 'plan_generated') {
      throw new ApiError(`plan cannot be corrected while session status is ${session.status}`, 422, 'invalid_workflow_state')
    }
    session.status = 'correction_requested'
    const revisedPlan =
      `${session.planMarkdown}\n\n## Revision feedback (revision ${session.revision + 1})\n- ${feedback.trim()}\n- Updated implementation steps to address the feedback above.\n`
    const task = {
      id: uid('task'), sessionId: session.sessionId, issueId: session.issueId,
      type: 'plan_revision', attempt: 1, createdAt: Date.now(),
      outcome: { kind: 'completed' }, plan: revisedPlan, revision: session.revision + 1,
    }
    tasks.set(task.id, task)
    return {
      session_id: session.sessionId, issue_id: session.issueId, status: 'queued',
      message: 'Correction task queued.', task_id: task.id, status_url: `/ai/tasks/${task.id}`,
    }
  },

  async rejectIssue(idOrIssueId) {
    await delay(400)
    const session = resolveSession(idOrIssueId)
    if (!session) throw new ApiError('issue session was not found', 404, 'session_not_found')
    if (session.status === 'approved') throw new ApiError('approved plan cannot be rejected', 422, 'invalid_workflow_state')
    session.status = 'rejected'
    return { session_id: session.sessionId, issue_id: session.issueId, status: session.status, message: 'Plan rejected.' }
  },
}

// Pre-seed the demo repository so the recommendations widget shows data on first load.
seedReport('demo-repo')
