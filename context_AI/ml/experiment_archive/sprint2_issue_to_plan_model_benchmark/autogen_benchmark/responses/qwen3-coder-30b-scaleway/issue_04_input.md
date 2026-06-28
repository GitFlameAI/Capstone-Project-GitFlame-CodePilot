# Issue
Title: Support asynchronous plan generation in the frontend

Plan generation may take several minutes once real models and the queue are enabled. Update the Work with AI flow so the UI can display queued and processing states, poll for the final plan, stop polling after completion or failure, and allow retry after recoverable errors. Avoid duplicate submissions when the user clicks Generate plan repeatedly.

# Evaluation Contract
# Implementation Plan Format

Every issue-to-plan response must be valid Markdown and use the sections below in the same order.

```markdown
# Implementation Plan

## Issue Summary
Brief restatement of the requested change.

## Goal
The expected product or technical outcome.

## Relevant Files
- `path/to/existing_file`: why it is relevant.
- `path/to/new_file` (create): why it is needed.

## Proposed Changes
- Concrete behavior or interface changes.

## Implementation Steps
1. Ordered implementation step.
2. Ordered implementation step.

## Expected Files to Change
- `path/to/file`: create or modify.

## Tests and Verification
- Test or verification step.

## Risks and Open Questions
- Risk, dependency, or `TBD` when repository context is insufficient.
```

## Rules

- Return only the implementation plan, without source code or patches.
- Reference only files present in the supplied repository context.
- Mark proposed new files with `(create)`.
- Use `TBD` instead of inventing missing repository details.
- Keep steps concrete, ordered, and testable.



# Repository Context
## File: `frontend/src/components/IssuePlanPanel.vue`
```text
<script setup>
import { reactive, ref } from 'vue'
import { api } from '../api/index.js'
import { demoRepo, defaultYaml } from '../data/demo.js'
import GfIcon from './ui/GfIcon.vue'
import GfButton from './ui/GfButton.vue'
import GfSpinner from './ui/GfSpinner.vue'

const step = ref('form') // form | plan | done
const loading = ref(false)
const error = ref('')
const plan = ref(null) // analyze/plan response
const workflow = ref(null) // git workflow after approve
const correcting = ref(false)
const correctionText = ref('')
const showCorrect = ref(false)

const issue = reactive({
  id: 'ISSUE-' + Math.floor(100 + Math.random() * 900),
  title: 'Add pagination to the repository list endpoint',
  body: 'The /repositories endpoint returns every record at once. We need offset/limit pagination and a total count so the UI can render pages.',
  author: 'roma',
})

const statusLabels = {
  plan_generated: 'Plan generated',
  correction_requested: 'Correction requested',
  approved: 'Approved',
  rejected: 'Rejected',
}

async function analyze() {
  error.value = ''
  loading.value = true
  try {
    plan.value = await api.analyzeIssue({
      repository: {
        id: demoRepo.id,
        name: demoRepo.name,
        default_branch: demoRepo.defaultBranch,
        web_url: demoRepo.webUrl,
      },
      issue: { id: issue.id, title: issue.title, body: issue.body, author: issue.author },
      yaml_config: defaultYaml,
      repository_context: demoRepo.branches,
    })
    step.value = 'plan'
  } catch (e) {
    error.value = e.message || 'Failed to generate the plan.'
  } finally {
    loading.value = false
  }
}

async function approve() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.approveIssue(issue.id)
    workflow.value = res.git_workflow
    plan.value = { ...plan.value, status: res.status }
    step.value = 'done'
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

async function submitCorrection() {
  if (!correctionText.value.trim()) return
  correcting.value = true
  error.value = ''
  try {
    const res = await api.correctIssue(issue.id, correctionText.value.trim())
    plan.value = { ...plan.value, plan_markdown: res.plan_markdown, status: res.status }
    correctionText.value = ''
    showCorrect.value = false
  } catch (e) {
    error.value = e.message
  } finally {
    correcting.value = false
  }
}

async function reject() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.rejectIssue(issue.id)
    plan.value = { ...plan.value, status: res.status }
    step.value = 'done'
    workflow.value = null
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

function restart() {
  step.value = 'form'
  plan.value = null
  workflow.value = null
  error.value = ''
  issue.id = 'ISSUE-' + Math.floor(100 + Math.random() * 900)
}
</script>

<template>
  <div class="panel">
    <p v-if="error" class="error"><GfIcon name="alert" :size="15" /> {{ error }}</p>

    <!-- Step 1: issue form -->
    <template v-if="step === 'form'">
      <p class="panel__intro gf-muted">
        Describe an issue the way it would arrive from GitFlame. The assistant returns a
        Markdown implementation plan you can approve, correct, or reject.
      </p>
      <label class="field">
        <span class="field__label">Issue title</span>
        <input v-model="issue.title" class="input" />
      </label>
      <label class="field">
        <span class="field__label">Description</span>
        <textarea v-model="issue.body" class="input textarea" rows="4" />
      </label>
      <div class="field">
        <span class="field__label">Issue author (becomes reviewer)</span>
        <input v-model="issue.author" class="input" />
      </div>
      <GfButton variant="primary" :loading="loading" @click="analyze">
        <GfIcon name="sparkles" :size="16" /> Generate plan
      </GfButton>
    </template>

    <!-- Step 2: plan + actions -->
    <template v-else-if="step === 'plan'">
      <div class="planhead">
        <span class="gf-chip planhead__status">{{ statusLabels[plan.status] || plan.status }}</span>
        <span class="gf-muted planhead__id mono">{{ issue.id }}</span>
      </div>
      <pre class="plan mono">{{ plan.plan_markdown }}</pre>

      <div v-if="showCorrect" class="correctbox">
        <textarea
          v-model="correctionText"
          class="input textarea"
          rows="2"
          placeholder="What should change in the plan?"
        />
        <div class="correctbox__actions">
          <GfButton variant="secondary" size="s" @click="showCorrect = false">Cancel</GfButton>
          <GfButton variant="primary" size="s" :loading="correcting" @click="submitCorrection">
            Submit correction
          </GfButton>
        </div>
      </div>

      <div class="actions">
        <GfButton variant="primary" :loading="loading" @click="approve">
          <GfIcon name="check" :size="16" /> Approve & create PR
        </GfButton>
        <GfButton variant="secondary" @click="showCorrect = !showCorrect">
          <GfIcon name="refresh" :size="15" /> Request correction
        </GfButton>
        <GfButton variant="danger" :loading="loading" @click="reject">Reject</GfButton>
      </div>
    </template>

    <!-- Step 3: result -->
    <template v-else>
      <div v-if="workflow" class="result result_ok">
        <div class="result__icon"><GfIcon name="check" :size="22" /></div>
        <h4>Mock pull request created</h4>
        <dl class="result__list">
          <div><dt>Branch</dt><dd class="mono">{{ workflow.branch_name }}</dd></div>
          <div>
            <dt>Pull request</dt>
            <dd><a :href="workflow.pull_request_url" class="mono">{{ workflow.pull_request_url }}</a></dd>
          </div>
          <div><dt>Reviewer</dt><dd>{{ workflow.reviewer }}</dd></div>
          <div><dt>Provider</dt><dd>{{ workflow.provider }}</dd></div>
        </dl>
        <p class="result__note gf-muted">
          In production GitFlame uses this payload to open a real branch and PR.
        </p>
      </div>
      <div v-else class="result result_rejected">
        <div class="result__icon"><GfIcon name="close" :size="22" /></div>
        <h4>Plan rejected</h4>
        <p class="result__note gf-muted">GitFlame can close the issue as not planned.</p>
      </div>
      <GfButton variant="secondary" @click="restart">Start another issue</GfButton>
    </template>

    <div v-if="loading && step === 'form'" class="overlay"><GfSpinner label="Generating plan…" /></div>
  </div>
</template>

<style scoped>
.panel {
  position: relative;
}
.panel__intro {
  margin: 0 0 16px;
  font-size: 13px;
  line-height: 1.55;
}
.error {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0 0 14px;
  padding: 9px 12px;
  border-radius: 10px;
  background: var(--gf-red-bg);
  color: var(--gf-red);
  font-size: 13px;
}
.field {
  margin-bottom: 14px;
}
.field__label {
  display: block;
  font-size: 12px;
  font-weight: 600;
  color: var(--gf-text-2);
  margin-bottom: 6px;
}
.input {
  width: 100%;
  min-height: 36px;
  padding: 8px 12px;
  border: 1px solid var(--gf-line-2);
  border-radius: 10px;
  font: inherit;
  font-size: 13px;
  color: var(--gf-text);
  background: var(--gf-surface);
}
.input:focus {
  outline: none;
  border-color: var(--gf-purple);
}
.textarea {
  resize: vertical;
  line-height: 1.5;
}
.planhead {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}
.planhead__status {
  color: var(--gf-accent);
  background: var(--gf-purple-soft);
  border-color: var(--gf-line-2);
}
.planhead__id {
  font-size: 12px;
}
.plan {
  margin: 0 0 14px;
  padding: 14px;
  border: 1px solid var(--gf-line);
  border-radius: 12px;
  background: var(--gf-surface-2);
  font-size: 12.5px;
  line-height: 1.55;
  white-space: pre-wrap;
  max-height: 280px;
  overflow: auto;
}
.correctbox {
  margin-bottom: 14px;
}
.correctbox__actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
}
.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.result {
  text-align: center;
  padding: 18px 8px 8px;
  margin-bottom: 16px;
}
.result__icon {
  display: grid;
  place-items: center;
  width: 48px;
  height: 48px;
  margin: 0 auto 10px;
  border-radius: 50%;
}
.result_ok .result__icon {
  background: var(--gf-green-bg);
  color: var(--gf-green);
}
.result_rejected .result__icon {
  background: var(--gf-red-bg);
  color: var(--gf-red);
}
.result h4 {
  margin: 0 0 14px;
  font-size: 16px;
}
.result__list {
  display: grid;
  gap: 8px;
  text-align: left;
  max-width: 460px;
  margin: 0 auto 12px;
}
.result__list > div {
  display: grid;
  grid-template-columns: 96px 1fr;
  gap: 10px;
  align-items: baseline;
}
.result__list dt {
  font-size: 12px;
  font-weight: 600;
  color: var(--gf-text-2);
}
.result__list dd {
  margin: 0;
  font-size: 13px;
  word-break: break-all;
}
.result__note {
  font-size: 12px;
}
.overlay {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  background: rgba(255, 255, 255, 0.7);
  border-radius: 12px;
}
</style>

```

## File: `frontend/src/api/client.js`
```text
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

```

## File: `frontend/src/api/index.js`
```text
// Single entry point the UI imports. It transparently selects the mock backend
// or the real Go backend depending on whether VITE_API_BASE is configured.
//
//   import { api, USING_MOCK } from '@/api'
//
// Every function returns the same shape in both modes, so components never need
// to know which backend they are talking to.

import { httpApi, ApiError } from './client.js'
import { mockApi } from './mock.js'

export const USING_MOCK = !import.meta.env.VITE_API_BASE
export const api = USING_MOCK ? mockApi : httpApi
export { ApiError }

```

## File: `frontend/src/api/mock.js`
```text
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

```
