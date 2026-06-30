<script setup>
// Autogeneration tab. Sprint 3 flow:
//
//   select issue ─▶ form ─analyze─▶ task ─poll─▶ editable plan
//   plan ─approve─▶ code-generation task ─poll─▶ done (generated files + PR)
//   plan ─correct─▶ revision task ─poll─▶ plan
//   plan ─reject──▶ done (rejected)
//
// The user first chooses an existing repository issue (fields auto-fill) or a new
// one (empty form). Repository context is NOT entered by the user — the Agent
// Engine prepares it via RAG (context_AI/ml/autogen_prompt.md). The plan is
// editable before approval; approval produces a generated-files contract whose
// file operations follow {path, action, description} (generated_files_contract.md).
import { reactive, ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { api, ApiError, pollTask } from '../../api/index.js'
import { session } from '../../store/session.js'
import { demoIssues } from '../../data/demo.js'
import GfIcon from '../ui/GfIcon.vue'
import GfButton from '../ui/GfButton.vue'
import GfSpinner from '../ui/GfSpinner.vue'
import GfTooltip from '../ui/GfTooltip.vue'
import MarkdownView from '../MarkdownView.vue'

const step = ref('select') // select | form | task | plan | codegen | done | failed | timeout
const issueSource = ref('new') // existing | new

// issue dropdown (existing issues)
const issueMenuOpen = ref(false)

const analyzing = ref(false)
const approveLoading = ref(false)
const rejectLoading = ref(false)
const correcting = ref(false)
const formError = ref('')
const actionError = ref('')

const sessionId = ref('')
const issueId = ref('')
const currentTaskId = ref('')

const taskStatus = ref('queued')
const taskAttempt = ref(1)
const taskError = ref(null)
const taskLabel = ref('Generating plan')

const planText = ref('')
const planMode = ref('preview')
const planRevision = ref(1)

const contract = ref(null)
const expanded = ref({})
const outcome = ref('')

const showCorrect = ref(false)
const correctionText = ref('')

let poller = null

const issue = reactive({ id: '', title: '', body: '', author: session.repo.owner || 'roma' })

const canRetry = computed(() => {
  if (!taskError.value) return false
  const s = taskError.value.http_status
  if (s === 503 || s === 504) return true
  const c = taskError.value.code
  return s === 502 && (c === 'agent_engine_error' || c === 'agent_engine_unreachable')
})

function newIssueId() {
  return 'ISSUE-' + Math.floor(100 + Math.random() * 900)
}

// --- issue selection ---
function startNewIssue() {
  issueSource.value = 'new'
  issue.id = newIssueId()
  issue.title = ''
  issue.body = ''
  issue.author = session.repo.owner || 'roma'
  step.value = 'form'
}
function pickIssue(it) {
  issueSource.value = 'existing'
  issue.id = it.id
  issue.title = it.title
  issue.body = it.body
  issue.author = it.author
  issueMenuOpen.value = false
  step.value = 'form'
}

function repositoryPayload() {
  return { id: session.repo.id, name: session.repo.name, default_branch: session.repo.defaultBranch, web_url: session.repo.url }
}
function stopPolling() {
  if (poller) { poller.abort(); poller = null }
}

async function runPlanTask(taskId) {
  currentTaskId.value = taskId
  taskError.value = null
  taskStatus.value = 'queued'
  taskLabel.value = 'Generating plan'
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
      planText.value = task.plan_markdown || ''
      planMode.value = 'preview'
      step.value = 'plan'
    } else {
      taskError.value = task.error || { http_status: 502, code: 'agent_engine_error', detail: 'Plan generation failed.' }
      step.value = 'failed'
    }
  } catch (e) {
    if (e instanceof ApiError && e.code === 'cancelled') return
    if (e instanceof ApiError && e.code === 'client_timeout') { step.value = 'timeout'; return }
    taskError.value = { http_status: e instanceof ApiError ? e.status : 0, code: e instanceof ApiError ? e.code || 'agent_engine_error' : 'network_error', detail: e.message || 'Failed to read task status.' }
    step.value = 'failed'
  } finally {
    poller = null
  }
}

async function analyze() {
  formError.value = ''
  if (!issue.title.trim() || !issue.body.trim() || !issue.author.trim()) {
    formError.value = 'Title, description and author are all required.'
    return
  }
  analyzing.value = true
  try {
    const res = await api.analyzeIssue({
      repository: repositoryPayload(),
      issue: { id: issue.id, title: issue.title.trim(), body: issue.body.trim(), author: issue.author.trim() },
      yaml_config: session.configYaml,
    })
    sessionId.value = res.session_id
    issueId.value = res.issue_id
    planRevision.value = 1
    await runPlanTask(res.task_id)
  } catch (e) {
    if (e instanceof ApiError && e.status === 422) formError.value = e.message
    else if (e instanceof ApiError && (e.status === 503 || e.status === 0)) formError.value = e.status === 0 ? e.message : 'The backend queue is temporarily unavailable. Please try again.'
    else formError.value = e.message || 'Failed to start plan generation.'
  } finally {
    analyzing.value = false
  }
}

async function retry() {
  try {
    const res = await api.retryTask(currentTaskId.value)
    await runPlanTask(res.task_id)
  } catch (e) {
    taskError.value = { http_status: e instanceof ApiError ? e.status : 0, code: e instanceof ApiError ? e.code : 'agent_engine_error', detail: e.message || 'Retry failed.' }
    step.value = 'failed'
  }
}
function keepWaiting() {
  runPlanTask(currentTaskId.value)
}

async function approve() {
  approveLoading.value = true
  actionError.value = ''
  try {
    const res = await api.approveIssue(sessionId.value, planText.value)
    contract.value = res.generated_files_contract || null
    outcome.value = 'approved'
    if (res.task_id) {
      taskLabel.value = 'Generating code'
      taskStatus.value = 'queued'
      step.value = 'codegen'
      stopPolling()
      poller = new AbortController()
      const task = await pollTask(res.task_id, {
        signal: poller.signal,
        onTick: (t) => { if (t.status === 'queued' || t.status === 'processing') taskStatus.value = t.status },
      })
      poller = null
      if (task.status === 'completed' && task.generated_files_contract) {
        contract.value = task.generated_files_contract
        step.value = 'done'
      } else if (task.status === 'failed') {
        taskError.value = task.error || { http_status: 502, code: 'agent_engine_error', detail: 'Code generation failed.' }
        step.value = 'failed'
      } else {
        step.value = 'done'
      }
    } else {
      step.value = 'done'
    }
  } catch (e) {
    if (e instanceof ApiError && e.code === 'cancelled') return
    if (e instanceof ApiError && e.code === 'client_timeout') { step.value = 'timeout'; return }
    actionError.value = e.message || 'Approve failed.'
    if (step.value === 'codegen') step.value = 'plan'
  } finally {
    approveLoading.value = false
  }
}

async function reject() {
  rejectLoading.value = true
  actionError.value = ''
  try {
    await api.rejectIssue(sessionId.value)
    outcome.value = 'rejected'
    contract.value = null
    step.value = 'done'
  } catch (e) {
    actionError.value = e.message || 'Reject failed.'
  } finally {
    rejectLoading.value = false
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
    planRevision.value += 1
    await runPlanTask(res.task_id)
  } catch (e) {
    actionError.value = e.message || 'Correction failed.'
  } finally {
    correcting.value = false
  }
}

// Back to the issue selection screen (used by "Back to Issues").
function backToIssues() {
  stopPolling()
  step.value = 'select'
  issueMenuOpen.value = false
  planText.value = ''
  contract.value = null
  outcome.value = ''
  formError.value = ''
  actionError.value = ''
  taskError.value = null
  sessionId.value = ''
  currentTaskId.value = ''
}

function goToPr() {
  if (!session.repo.url) return
  window.open(`${session.repo.url}/pulls`, '_blank', 'noopener')
}

function toggleFile(path) {
  expanded.value[path] = !expanded.value[path]
}

// When the user turned a recommendation into an issue, the Recommendations tab
// stored it on the session; pick it up and open a pre-filled new-issue form.
onMounted(() => {
  if (session.pendingIssue) {
    const p = session.pendingIssue
    issueSource.value = 'new'
    issue.id = newIssueId()
    issue.title = p.title || ''
    issue.body = p.body || ''
    issue.author = p.author || session.repo.owner || 'roma'
    step.value = 'form'
    session.pendingIssue = null
  }
})

onBeforeUnmount(stopPolling)
</script>

<template>
  <div class="ag">
    <!-- Step: choose an issue -->
    <template v-if="step === 'select'">
      <div class="card gf-card">
        <h3 class="card__title"><GfIcon name="sparkles" :size="16" /> Generate code from an issue</h3>
        <p class="card__sub gf-muted">
          Pick an existing issue from the repository (its fields fill in automatically) or
          start a new one. CodePilot drafts a plan you can edit and approve before any code
          is generated.
        </p>

        <div class="choose">
          <div class="choose__block">
            <span class="choose__label">Use an existing issue</span>
            <div class="dd">
              <button class="dd__btn" @click="issueMenuOpen = !issueMenuOpen">
                <GfIcon name="alert" :size="15" />
                <span class="dd__text">Select an issue from the repository…</span>
                <GfIcon name="chevronDown" :size="15" :class="{ dd__caret_open: issueMenuOpen }" class="dd__caret" />
              </button>
              <ul v-if="issueMenuOpen" class="dd__menu">
                <li v-for="it in demoIssues" :key="it.id" class="dd__opt" @click="pickIssue(it)">
                  <span class="dd__id mono">{{ it.id }}</span>
                  <span class="dd__title">{{ it.title }}</span>
                </li>
              </ul>
            </div>
          </div>

          <div class="choose__or">or</div>

          <div class="choose__block">
            <span class="choose__label">Start from scratch</span>
            <GfButton variant="secondary" @click="startNewIssue">
              <GfIcon name="plus" :size="15" /> Create a new issue
            </GfButton>
          </div>
        </div>
      </div>
    </template>

    <!-- Step: issue form -->
    <template v-else-if="step === 'form'">
      <div class="card gf-card">
        <div class="formhead">
          <h3 class="card__title">
            <GfIcon name="sparkles" :size="16" />
            {{ issueSource === 'existing' ? 'Existing issue' : 'New issue' }}
          </h3>
          <button class="back-link" @click="backToIssues"><GfIcon name="chevronRight" :size="13" class="back-link__ic" /> Back to issues</button>
        </div>
        <p v-if="formError" class="error"><GfIcon name="alert" :size="15" /> {{ formError }}</p>

        <label class="field">
          <span class="field__label">Issue title</span>
          <input v-model="issue.title" class="input" placeholder="Short summary of the change" />
        </label>
        <label class="field">
          <span class="field__label">Description</span>
          <textarea v-model="issue.body" class="input textarea" rows="4" placeholder="What needs to change and why" />
        </label>
        <label class="field field_narrow">
          <span class="field__label">Issue author
            <GfTooltip text="Becomes the reviewer on the generated pull request." /></span>
          <input v-model="issue.author" class="input" placeholder="your-username" />
        </label>

        <GfButton variant="primary" :loading="analyzing" @click="analyze">
          <GfIcon name="sparkles" :size="16" /> Generate plan
        </GfButton>
      </div>
    </template>

    <!-- Step: task running -->
    <template v-else-if="step === 'task' || step === 'codegen'">
      <div class="card gf-card center">
        <GfSpinner :size="26" />
        <ol class="steps">
          <li :class="{ step_active: taskStatus === 'queued', step_done: taskStatus === 'processing' }">
            <GfIcon name="history" :size="14" /> Queued
          </li>
          <li :class="{ step_active: taskStatus === 'processing' }">
            <GfIcon name="sparkles" :size="14" /> {{ taskLabel }}
          </li>
          <li><GfIcon name="check" :size="14" /> Ready</li>
        </ol>
        <p class="center__hint gf-muted">
          {{ taskLabel }}<span v-if="taskAttempt > 1"> · attempt {{ taskAttempt }}</span>.
          The Agent Engine is working on it; this can take a moment.
        </p>
      </div>
    </template>

    <!-- Step: plan -->
    <template v-else-if="step === 'plan'">
      <div class="card gf-card">
        <div class="planhead">
          <span class="gf-chip planhead__status">Plan generated</span>
          <span class="gf-muted mono planhead__id">{{ issueId }}</span>
          <span v-if="planRevision > 1" class="gf-muted planhead__rev">revision {{ planRevision }}</span>
          <GfTooltip text="Edit the plan freely before approving — switch to Preview to see the rendered Markdown." placement="bottom" />
        </div>
        <p v-if="actionError" class="error"><GfIcon name="alert" :size="15" /> {{ actionError }}</p>

        <MarkdownView v-model="planText" v-model:mode="planMode" :rows="16" />

        <div v-if="showCorrect" class="correctbox">
          <textarea v-model="correctionText" class="input textarea" rows="2" placeholder="What should change in the plan?" />
          <div class="correctbox__actions">
            <GfButton variant="secondary" size="s" @click="showCorrect = false">Cancel</GfButton>
            <GfButton variant="primary" size="s" :loading="correcting" @click="submitCorrection">Submit correction</GfButton>
          </div>
        </div>

        <div class="actions">
          <GfButton variant="primary" :loading="approveLoading" :disabled="rejectLoading" @click="approve">
            <GfIcon name="check" :size="16" /> Approve &amp; generate code
          </GfButton>
          <GfButton variant="secondary" :disabled="approveLoading || rejectLoading" @click="showCorrect = !showCorrect">
            <GfIcon name="refresh" :size="15" /> Request correction
          </GfButton>
          <GfButton variant="danger" :loading="rejectLoading" :disabled="approveLoading" @click="reject">Reject</GfButton>
        </div>
      </div>
    </template>

    <!-- Step: failed -->
    <template v-else-if="step === 'failed'">
      <div class="card gf-card">
        <div class="result result_bad">
          <div class="result__icon"><GfIcon name="alert" :size="22" /></div>
          <h4>{{ taskLabel === 'Generating code' ? 'Code generation failed' : 'Plan generation failed' }}</h4>
          <p class="result__note">{{ taskError?.detail }}</p>
          <p v-if="taskError?.code" class="result__code mono gf-muted">
            {{ taskError.code }}<span v-if="taskError.http_status"> · HTTP {{ taskError.http_status }}</span>
          </p>
        </div>
        <div class="actions actions_center">
          <GfButton v-if="canRetry" variant="primary" @click="retry"><GfIcon name="refresh" :size="15" /> Retry</GfButton>
          <GfButton variant="secondary" @click="backToIssues">Back to issues</GfButton>
        </div>
      </div>
    </template>

    <!-- Step: timeout -->
    <template v-else-if="step === 'timeout'">
      <div class="card gf-card">
        <div class="result result_warn">
          <div class="result__icon"><GfIcon name="history" :size="22" /></div>
          <h4>Still working…</h4>
          <p class="result__note gf-muted">This is taking longer than expected. The Agent Engine may still be running.</p>
        </div>
        <div class="actions actions_center">
          <GfButton variant="primary" @click="keepWaiting"><GfIcon name="refresh" :size="15" /> Keep waiting</GfButton>
          <GfButton variant="secondary" @click="backToIssues">Back to issues</GfButton>
        </div>
      </div>
    </template>

    <!-- Step: done -->
    <template v-else>
      <div v-if="outcome === 'approved'" class="card gf-card">
        <div class="result result_ok">
          <div class="result__icon"><GfIcon name="check" :size="22" /></div>
          <h4>Plan approved · code generated</h4>
        </div>
        <dl v-if="contract" class="contract">
          <div><dt>Branch</dt><dd class="mono">{{ contract.branch_name }}</dd></div>
          <div><dt>Commit</dt><dd>{{ contract.commit_message }}</dd></div>
          <div><dt>PR title</dt><dd>{{ contract.pr_title }}</dd></div>
          <div><dt>Reviewer</dt><dd>{{ contract.reviewer }}</dd></div>
        </dl>

        <h4 v-if="contract && contract.files && contract.files.length" class="files__title">Generated file operations</h4>
        <ul v-if="contract && contract.files" class="files">
          <li v-for="f in contract.files" :key="f.path" class="file">
            <button class="file__head" @click="toggleFile(f.path)">
              <span class="file__action" :class="`file__action_${f.action}`">{{ f.action }}</span>
              <span class="file__path mono">{{ f.path }}</span>
              <GfIcon name="chevronRight" :size="14" class="file__caret" :class="{ file__caret_open: expanded[f.path] }" />
            </button>
            <div v-if="expanded[f.path]" class="file__body">
              <p class="file__desc">{{ f.description }}</p>
            </div>
          </li>
        </ul>

        <div class="actions actions_split">
          <GfButton variant="secondary" @click="backToIssues">
            <GfIcon name="chevronRight" :size="14" class="back-link__ic" /> Back to issues
          </GfButton>
          <GfButton variant="primary" @click="goToPr">
            <GfIcon name="branch" :size="15" /> Go to pull request
          </GfButton>
        </div>
      </div>

      <div v-else class="card gf-card">
        <div class="result result_bad">
          <div class="result__icon"><GfIcon name="close" :size="22" /></div>
          <h4>Plan rejected</h4>
          <p class="result__note gf-muted">GitFlame can close the issue as not planned.</p>
        </div>
        <div class="actions actions_center">
          <GfButton variant="secondary" @click="backToIssues">Back to issues</GfButton>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.ag {
  max-width: 760px;
}
.card {
  padding: 22px 24px;
}
.card__title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 6px;
  font-size: 15px;
}
.card__title :deep(.gf-icon) {
  color: var(--gf-purple);
}
.card__sub {
  margin: 0 0 16px;
  font-size: 13px;
  line-height: 1.55;
}

/* choose issue */
.choose {
  display: flex;
  align-items: stretch;
  gap: 18px;
}
.choose__block {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 9px;
}
.choose__label {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--gf-text-2);
}
.choose__or {
  align-self: center;
  font-size: 12px;
  color: var(--gf-text-3);
  font-weight: 600;
}
.dd {
  position: relative;
}
.dd__btn {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  height: 40px;
  padding: 0 12px;
  border: 1px solid var(--gf-line-2);
  border-radius: 10px;
  background: var(--gf-surface);
  color: var(--gf-text-2);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.dd__btn:hover {
  border-color: var(--gf-purple);
}
.dd__btn :deep(.gf-icon):first-child {
  color: var(--gf-purple);
  flex: none;
}
.dd__text {
  flex: 1;
  text-align: left;
}
.dd__caret {
  transition: transform 0.12s ease;
  flex: none;
}
.dd__caret_open {
  transform: rotate(180deg);
}
.dd__menu {
  position: absolute;
  z-index: 30;
  left: 0;
  right: 0;
  margin: 6px 0 0;
  padding: 5px;
  list-style: none;
  border: 1px solid var(--gf-line);
  border-radius: 12px;
  background: var(--gf-surface);
  box-shadow: var(--gf-shadow-pop);
}
.dd__opt {
  display: flex;
  align-items: baseline;
  gap: 9px;
  padding: 9px 10px;
  border-radius: 9px;
  cursor: pointer;
}
.dd__opt:hover {
  background: var(--gf-purple-soft);
}
.dd__id {
  font-size: 11px;
  color: var(--gf-accent);
  flex: none;
}
.dd__title {
  font-size: 13px;
  color: var(--gf-text);
}

.formhead {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}
.formhead .card__title {
  margin: 0;
}
.back-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 0;
  background: transparent;
  color: var(--gf-accent);
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
}
.back-link__ic {
  transform: rotate(180deg);
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
.field_narrow {
  max-width: 280px;
}
.field__label {
  display: flex;
  align-items: center;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--gf-text-2);
  margin-bottom: 7px;
}
.input {
  width: 100%;
  height: 38px;
  padding: 0 12px;
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
.input::placeholder {
  color: var(--gf-text-3);
}
.textarea {
  height: auto;
  padding: 9px 12px;
  resize: vertical;
  line-height: 1.5;
}

.center {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  text-align: center;
  padding: 32px 20px;
}
.steps {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.steps li {
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
.step_active {
  color: var(--gf-accent);
  background: var(--gf-purple-soft);
}
.step_done {
  color: var(--gf-green);
  border-color: var(--gf-green);
}
.center__hint {
  max-width: 440px;
  margin: 0;
  font-size: 12.5px;
  line-height: 1.5;
}

.planhead {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
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
.correctbox {
  margin: 14px 0 0;
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
  margin-top: 16px;
}
.actions_center {
  justify-content: center;
}
.actions_split {
  justify-content: space-between;
}

.result {
  text-align: center;
  padding: 8px 8px 4px;
  margin-bottom: 14px;
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
.result_bad .result__icon {
  background: var(--gf-red-bg);
  color: var(--gf-red);
}
.result_warn .result__icon {
  background: var(--gf-amber-bg);
  color: var(--gf-amber);
}
.result h4 {
  margin: 0 0 6px;
  font-size: 16px;
}
.result__note {
  font-size: 12.5px;
  line-height: 1.5;
  max-width: 480px;
  margin: 10px auto 14px;
}
.result__code {
  font-size: 11.5px;
}

.contract {
  display: grid;
  gap: 8px;
  max-width: 520px;
  margin: 0 auto 18px;
}
.contract > div {
  display: grid;
  grid-template-columns: 96px 1fr;
  gap: 10px;
  align-items: baseline;
}
.contract dt {
  font-size: 12px;
  font-weight: 600;
  color: var(--gf-text-2);
}
.contract dd {
  margin: 0;
  font-size: 13px;
  word-break: break-word;
}
.files__title {
  margin: 0 0 10px;
  font-size: 14px;
}
.files {
  list-style: none;
  margin: 0 0 16px;
  padding: 0;
  display: grid;
  gap: 8px;
}
.file {
  border: 1px solid var(--gf-line);
  border-radius: 10px;
  overflow: hidden;
}
.file__head {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 10px 12px;
  border: 0;
  background: var(--gf-surface-2);
  font: inherit;
  cursor: pointer;
  text-align: left;
}
.file__head:hover {
  background: var(--gf-surface-3);
}
.file__action {
  flex: none;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
}
.file__action_create {
  color: var(--gf-green);
  background: var(--gf-green-bg);
}
.file__action_modify {
  color: var(--gf-blue);
  background: var(--gf-blue-bg);
}
.file__action_delete {
  color: var(--gf-red);
  background: var(--gf-red-bg);
}
.file__path {
  flex: 1;
  font-size: 12.5px;
  font-weight: 600;
  word-break: break-all;
}
.file__caret {
  color: var(--gf-text-3);
  transition: transform 0.12s ease;
  flex: none;
}
.file__caret_open {
  transform: rotate(90deg);
}
.file__body {
  padding: 12px;
  border-top: 1px solid var(--gf-line);
}
.file__desc {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--gf-text-2);
}
@media (max-width: 560px) {
  .choose {
    flex-direction: column;
  }
}
</style>
