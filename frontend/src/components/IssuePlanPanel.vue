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
