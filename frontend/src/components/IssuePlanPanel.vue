<script setup>
import { reactive, ref, computed, onBeforeUnmount } from 'vue'
import { api, ApiError, pollTask } from '../api/index.js'
import { demoRepo, defaultYaml, defaultRepositoryContext } from '../data/demo.js'
import GfIcon from './ui/GfIcon.vue'
import GfButton from './ui/GfButton.vue'
import GfSpinner from './ui/GfSpinner.vue'

// Sprint 2 flow (asynchronous):
//   form -> [POST /issues/analyze] -> task (queued|processing, polled) ->
//     completed -> plan -> approve | correct | reject
//     failed    -> failed  (retry if the Agent Engine error is recoverable)
//     timed out -> timeout (keep waiting, or start over)
const step = ref('form') // form | task | plan | failed | timeout | done

// Network/loading flags for the individual actions.
const analyzing = ref(false)
const actionLoading = ref(false)
const correcting = ref(false)

// Form-level validation message (422 from the backend) shown under the form.
const formError = ref('')
// Generic action error shown on the plan screen (approve/correct/reject failures).
const actionError = ref('')

// Workflow identifiers returned by the backend.
const sessionId = ref('')
const issueId = ref('')
const currentTaskId = ref('')

// Live task polling state.
const taskStatus = ref('queued') // queued | processing
const taskAttempt = ref(1)
const taskError = ref(null) // { http_status, code, detail }

// Results.
const plan = ref(null) // { plan_markdown, status, revision }
const contract = ref(null) // generated_files_contract after approve
const outcome = ref('') // approved | rejected

// Correction sub-form.
const showCorrect = ref(false)
const correctionText = ref('')

// AbortController so polling stops if the modal closes or the user starts over.
let poller = null

const issue = reactive({
  id: 'ISSUE-' + Math.floor(100 + Math.random() * 900),
  title: 'Add pagination to the repository list endpoint',
  body: 'The /repositories endpoint returns every record at once. We need offset/limit pagination and a total count so the UI can render pages.',
  author: 'roma',
})
// One file path per line (sent to the backend as repository_context).
const contextText = ref(defaultRepositoryContext.join('\n'))

const statusLabels = {
  queued: 'Queued',
  processing: 'Generating plan',
  plan_generated: 'Plan generated',
  correction_requested: 'Correction requested',
  approved: 'Approved',
  rejected: 'Rejected',
  failed: 'Failed',
}

// Mirrors the backend's recoverableTaskError (backend/internal/service/workflow.go):
// only a failed task with a *temporary* Agent Engine error may be retried.
// 503 and 504 are always recoverable; 502 is recoverable only for the codes
// agent_engine_error / agent_engine_unreachable. Every other failure
// (e.g. 502 invalid_agent_response, 422 invalid_output / empty_output) is
// terminal, so we must not offer a Retry the backend would reject with 422.
const canRetry = computed(() => {
  if (!taskError.value) return false
  const s = taskError.value.http_status
  if (s === 503 || s === 504) return true
  const c = taskError.value.code
  return s === 502 && (c === 'agent_engine_error' || c === 'agent_engine_unreachable')
})

function repositoryPayload() {
  return {
    id: demoRepo.id,
    name: demoRepo.name,
    default_branch: demoRepo.defaultBranch,
    commit_sha: demoRepo.commitSha,
    web_url: demoRepo.webUrl,
  }
}

function repositoryContext() {
  return contextText.value
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
}

function stopPolling() {
  if (poller) {
    poller.abort()
    poller = null
  }
}

// Polls an agent task to a terminal state and routes the UI accordingly.
async function runTask(taskId) {
  currentTaskId.value = taskId
  taskError.value = null
  taskStatus.value = 'queued'
  step.value = 'task'
  stopPolling()
  poller = new AbortController()
  try {
    const task = await pollTask(taskId, {
      signal: poller.signal,
      onTick: (t) => {
        if (t.status === 'queued' || t.status === 'processing') taskStatus.value = t.status
        if (typeof t.attempt === 'number') taskAttempt.value = t.attempt
      },
    })
    if (task.status === 'completed') {
      plan.value = {
        plan_markdown: task.plan_markdown || '',
        status: 'plan_generated',
        revision: plan.value ? plan.value.revision + 1 : 1,
      }
      step.value = 'plan'
    } else {
      // status === 'failed'
      taskError.value = task.error || { http_status: 502, code: 'agent_engine_error', detail: 'Plan generation failed.' }
      step.value = 'failed'
    }
  } catch (e) {
    if (e instanceof ApiError && e.code === 'cancelled') return // modal closed / restarted
    if (e instanceof ApiError && e.code === 'client_timeout') {
      step.value = 'timeout'
      return
    }
    // getTask itself failed (e.g. backend went away while polling).
    taskError.value = {
      http_status: e instanceof ApiError ? e.status : 0,
      code: e instanceof ApiError ? e.code || 'agent_engine_error' : 'network_error',
      detail: e.message || 'Failed to read task status.',
    }
    step.value = 'failed'
  } finally {
    poller = null
  }
}

async function analyze() {
  formError.value = ''
  const context = repositoryContext()
  if (!issue.title.trim() || !issue.body.trim() || !issue.author.trim()) {
    formError.value = 'Title, description and author are required.'
    return
  }
  if (context.length === 0) {
    formError.value = 'Add at least one repository file path as context.'
    return
  }
  analyzing.value = true
  plan.value = null
  try {
    const res = await api.analyzeIssue({
      repository: repositoryPayload(),
      issue: { id: issue.id, title: issue.title.trim(), body: issue.body.trim(), author: issue.author.trim() },
      yaml_config: defaultYaml,
      repository_context: context,
    })
    sessionId.value = res.session_id
    issueId.value = res.issue_id
    await runTask(res.task_id)
  } catch (e) {
    if (e instanceof ApiError && e.status === 422) {
      formError.value = e.message // validation error -> stay on the form
    } else if (e instanceof ApiError && (e.status === 503 || e.status === 0)) {
      // Queue/Agent Engine unavailable, or backend unreachable. No agent task
      // exists from our side yet, so we stay on the form and let the user
      // resubmit, rather than entering the task-failed state whose Retry button
      // would target a task id we never received.
      formError.value =
        e.status === 0
          ? e.message
          : 'The backend queue is temporarily unavailable. Please try again in a moment.'
    } else {
      formError.value = e.message || 'Failed to start plan generation.'
    }
  } finally {
    analyzing.value = false
  }
}

// Re-queues a failed-but-recoverable task on the backend, then polls the new task.
async function retry() {
  actionError.value = ''
  try {
    const res = await api.retryTask(currentTaskId.value)
    await runTask(res.task_id)
  } catch (e) {
    taskError.value = {
      http_status: e instanceof ApiError ? e.status : 0,
      code: e instanceof ApiError ? e.code : 'agent_engine_error',
      detail: e.message || 'Retry failed.',
    }
    step.value = 'failed'
  }
}

// For client-side timeouts the server task may still be running, so we simply
// resume polling the same task rather than creating a new one.
function keepWaiting() {
  runTask(currentTaskId.value)
}

async function approve() {
  actionLoading.value = true
  actionError.value = ''
  try {
    const res = await api.approveIssue(sessionId.value)
    contract.value = res.generated_files_contract || null
    outcome.value = 'approved'
    step.value = 'done'
  } catch (e) {
    actionError.value = e.message || 'Approve failed.'
  } finally {
    actionLoading.value = false
  }
}

async function reject() {
  actionLoading.value = true
  actionError.value = ''
  try {
    await api.rejectIssue(sessionId.value)
    outcome.value = 'rejected'
    contract.value = null
    step.value = 'done'
  } catch (e) {
    actionError.value = e.message || 'Reject failed.'
  } finally {
    actionLoading.value = false
  }
}

async function submitCorrection() {
  if (!correctionText.value.trim()) return
  correcting.value = true
  actionError.value = ''
  try {
    const res = await api.correctIssue(sessionId.value, correctionText.value.trim())
    correctionText.value = ''
    showCorrect.value = false
    // The correction is generated asynchronously -> poll the new task.
    await runTask(res.task_id)
  } catch (e) {
    actionError.value = e.message || 'Correction failed.'
  } finally {
    correcting.value = false
  }
}

function restart() {
  stopPolling()
  step.value = 'form'
  plan.value = null
  contract.value = null
  outcome.value = ''
  formError.value = ''
  actionError.value = ''
  taskError.value = null
  sessionId.value = ''
  currentTaskId.value = ''
  issue.id = 'ISSUE-' + Math.floor(100 + Math.random() * 900)
}

onBeforeUnmount(stopPolling)
</script>

<template>
  <div class="panel">
    <!-- Step 1: issue form -->
    <template v-if="step === 'form'">
      <p class="panel__intro gf-muted">
        Describe an issue the way it would arrive from GitFlame. The backend queues an Agent
        Engine task that returns a Markdown implementation plan you can approve, correct, or reject.
      </p>
      <p v-if="formError" class="error"><GfIcon name="alert" :size="15" /> {{ formError }}</p>
      <label class="field">
        <span class="field__label">Issue title</span>
        <input v-model="issue.title" class="input" placeholder="Short summary of the change" />
      </label>
      <label class="field">
        <span class="field__label">Description</span>
        <textarea v-model="issue.body" class="input textarea" rows="4" placeholder="What needs to change and why" />
      </label>
      <label class="field">
        <span class="field__label">Issue author (becomes reviewer)</span>
        <input v-model="issue.author" class="input" />
      </label>
      <label class="field">
        <span class="field__label">
          Repository context
          <span class="field__hint gf-muted">— one file path per line, used for RAG retrieval</span>
        </span>
        <textarea v-model="contextText" class="input textarea mono" rows="4" spellcheck="false" />
      </label>
      <GfButton variant="primary" :loading="analyzing" @click="analyze">
        <GfIcon name="sparkles" :size="16" /> Generate plan
      </GfButton>
    </template>

    <!-- Step 2: task is queued / processing -->
    <template v-else-if="step === 'task'">
      <div class="task">
        <GfSpinner :size="26" />
        <ol class="task__steps" aria-label="Plan generation progress">
          <li :class="{ task__step_active: taskStatus === 'queued', task__step_done: taskStatus === 'processing' }">
            <GfIcon name="history" :size="14" /> Queued
          </li>
          <li :class="{ task__step_active: taskStatus === 'processing' }">
            <GfIcon name="sparkles" :size="14" /> Generating plan
          </li>
          <li><GfIcon name="check" :size="14" /> Plan ready</li>
        </ol>
        <p class="task__hint gf-muted">
          {{ statusLabels[taskStatus] }}<span v-if="taskAttempt > 1"> · attempt {{ taskAttempt }}</span>.
          The Agent Engine analyses the repository and drafts the plan; this can take a moment on a demo GPU.
        </p>
      </div>
    </template>

    <!-- Step 3: plan generated + actions -->
    <template v-else-if="step === 'plan'">
      <p v-if="actionError" class="error"><GfIcon name="alert" :size="15" /> {{ actionError }}</p>
      <div class="planhead">
        <span class="gf-chip planhead__status">{{ statusLabels[plan.status] || plan.status }}</span>
        <span class="gf-muted planhead__id mono">{{ issueId }}</span>
        <span v-if="plan.revision > 1" class="gf-muted planhead__rev">revision {{ plan.revision }}</span>
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
        <GfButton variant="primary" :loading="actionLoading" @click="approve">
          <GfIcon name="check" :size="16" /> Approve
        </GfButton>
        <GfButton variant="secondary" @click="showCorrect = !showCorrect">
          <GfIcon name="refresh" :size="15" /> Request correction
        </GfButton>
        <GfButton variant="danger" :loading="actionLoading" @click="reject">Reject</GfButton>
      </div>
    </template>

    <!-- Step 4a: task failed -->
    <template v-else-if="step === 'failed'">
      <div class="result result_rejected">
        <div class="result__icon"><GfIcon name="alert" :size="22" /></div>
        <h4>Plan generation failed</h4>
        <p class="result__note">{{ taskError?.detail }}</p>
        <p v-if="taskError?.code" class="result__code mono gf-muted">
          {{ taskError.code }}<span v-if="taskError.http_status"> · HTTP {{ taskError.http_status }}</span>
        </p>
      </div>
      <div class="actions actions_center">
        <GfButton v-if="canRetry" variant="primary" @click="retry">
          <GfIcon name="refresh" :size="15" /> Retry
        </GfButton>
        <GfButton variant="secondary" @click="restart">Start over</GfButton>
      </div>
    </template>

    <!-- Step 4b: client-side timeout (server may still be working) -->
    <template v-else-if="step === 'timeout'">
      <div class="result result_timeout">
        <div class="result__icon"><GfIcon name="history" :size="22" /></div>
        <h4>Still generating…</h4>
        <p class="result__note gf-muted">
          The plan is taking longer than expected. The Agent Engine may still be working on it.
        </p>
      </div>
      <div class="actions actions_center">
        <GfButton variant="primary" @click="keepWaiting">
          <GfIcon name="refresh" :size="15" /> Keep waiting
        </GfButton>
        <GfButton variant="secondary" @click="restart">Start over</GfButton>
      </div>
    </template>

    <!-- Step 5: result -->
    <template v-else>
      <div v-if="outcome === 'approved'" class="result result_ok">
        <div class="result__icon"><GfIcon name="check" :size="22" /></div>
        <h4>Plan approved</h4>
        <dl v-if="contract" class="result__list">
          <div><dt>Branch</dt><dd class="mono">{{ contract.branch_name }}</dd></div>
          <div><dt>Commit</dt><dd>{{ contract.commit_message }}</dd></div>
          <div><dt>PR title</dt><dd>{{ contract.pr_title }}</dd></div>
          <div><dt>Reviewer</dt><dd>{{ contract.reviewer }}</dd></div>
        </dl>
        <p class="result__note gf-muted">
          This contract is returned for a future code-generation worker. GitFlame uses it to
          open a branch and pull request on its side.
        </p>
      </div>
      <div v-else class="result result_rejected">
        <div class="result__icon"><GfIcon name="close" :size="22" /></div>
        <h4>Plan rejected</h4>
        <p class="result__note gf-muted">GitFlame can close the issue as not planned.</p>
      </div>
      <GfButton variant="secondary" @click="restart">Start another issue</GfButton>
    </template>
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
  display: block;
  margin-bottom: 14px;
}
.field__label {
  display: block;
  font-size: 12px;
  font-weight: 600;
  color: var(--gf-text-2);
  margin-bottom: 6px;
}
.field__hint {
  font-weight: 400;
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
.textarea.mono {
  font-size: 12.5px;
}

/* Task progress */
.task {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  padding: 26px 8px 14px;
  text-align: center;
}
.task__steps {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.task__steps li {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 5px 11px;
  border: 1px solid var(--gf-line-2);
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  color: var(--gf-text-3);
}
.task__step_active {
  color: var(--gf-accent);
  background: var(--gf-purple-soft);
  border-color: var(--gf-line-2);
}
.task__step_done {
  color: var(--gf-green);
  border-color: var(--gf-green);
}
.task__hint {
  max-width: 440px;
  margin: 0;
  font-size: 12.5px;
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
.planhead__id,
.planhead__rev {
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
.actions_center {
  justify-content: center;
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
.result_timeout .result__icon {
  background: var(--gf-amber-bg);
  color: var(--gf-amber);
}
.result h4 {
  margin: 0 0 12px;
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
  word-break: break-word;
}
.result__note {
  font-size: 12.5px;
  line-height: 1.5;
  max-width: 460px;
  margin: 0 auto 6px;
}
.result__code {
  font-size: 11.5px;
}
</style>
