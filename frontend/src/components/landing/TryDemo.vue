<script setup>
// "Try it yourself" — a tiny, self-contained interactive preview of the flow for
// visitors who would rather click than read. It is deliberately a MOCK (no
// backend, no repository, local timers) and is clearly badged as a preview. It
// reuses the SAME plan editor (MarkdownView, with a working Edit/Preview) and the
// SAME "Request correction" interaction as the real Autogeneration tab, so the
// preview matches the product and emphasises that YOU approve.
import { ref, onBeforeUnmount } from 'vue'
import GfIcon from '../ui/GfIcon.vue'
import GfButton from '../ui/GfButton.vue'
import MarkdownView from '../MarkdownView.vue'

// step: idle -> planning -> plan -> correcting -> generating -> done -> rejected
const step = ref('idle')
const planMode = ref('preview')
const revision = ref(1)
const showCorrect = ref(false)
const correctionText = ref('')
let timer = null

const basePlan = `# Implementation Plan

## Issue summary
Add pagination to the repository list endpoint.

## Steps
1. Read the current handler and confirm the response shape.
2. Parse \`?limit\` and \`?offset\` with safe defaults.
3. Return \`items\` plus a \`total\` count for the UI.`

const planText = ref(basePlan)

const files = [
  { action: 'modify', path: 'internal/httpapi/server.go' },
  { action: 'create', path: 'internal/httpapi/pagination.go' },
  { action: 'modify', path: 'internal/httpapi/openapi.json' },
]

function clear() {
  if (timer) { clearTimeout(timer); timer = null }
}
function generate() {
  clear()
  step.value = 'planning'
  timer = setTimeout(() => (step.value = 'plan'), 1200)
}
function submitCorrection() {
  const note = correctionText.value.trim()
  if (!note) return
  clear()
  step.value = 'correcting'
  timer = setTimeout(() => {
    revision.value += 1
    planText.value = `${planText.value}\n\n## Revision ${revision.value}\nApplied your note: _${note}_`
    correctionText.value = ''
    showCorrect.value = false
    planMode.value = 'preview'
    step.value = 'plan'
  }, 1100)
}
function approve() {
  clear()
  step.value = 'generating'
  timer = setTimeout(() => (step.value = 'done'), 1200)
}
function reject() {
  clear()
  step.value = 'rejected'
}
function reset() {
  clear()
  planText.value = basePlan
  planMode.value = 'preview'
  revision.value = 1
  showCorrect.value = false
  correctionText.value = ''
  step.value = 'idle'
}
onBeforeUnmount(clear)
</script>

<template>
  <div class="try">
    <div class="try__head">
      <span class="try__badge"><span class="try__dot" /> Live preview · demo</span>
      <span class="try__hint">No repository needed — nothing here is saved.</span>
    </div>

    <div class="try__body">
      <!-- issue -->
      <div class="try__issue">
        <span class="try__label">Issue</span>
        <p class="try__issue-title">Add pagination to the repository list endpoint</p>
        <p class="try__issue-body">The endpoint returns every record at once. We need offset/limit paging and a total count.</p>
      </div>

      <!-- idle -->
      <div v-if="step === 'idle'" class="try__cta">
        <GfButton variant="primary" @click="generate"><GfIcon name="sparkles" :size="15" /> Generate plan</GfButton>
      </div>

      <!-- loading states -->
      <div v-else-if="step === 'planning'" class="try__loading"><span class="try__spin" /> Drafting a plan…</div>
      <div v-else-if="step === 'correcting'" class="try__loading"><span class="try__spin" /> Revising the plan…</div>
      <div v-else-if="step === 'generating'" class="try__loading"><span class="try__spin" /> Generating code…</div>

      <!-- plan: you review, edit and decide (same editor + correction as the real tab) -->
      <template v-else-if="step === 'plan'">
        <div class="try__planhead">
          <span class="try__plan-status"><GfIcon name="check" :size="13" /> Plan generated</span>
          <span v-if="revision > 1" class="try__rev">revision {{ revision }}</span>
        </div>

        <MarkdownView v-model="planText" v-model:mode="planMode" :rows="10" />

        <div v-if="showCorrect" class="try__correctbox">
          <textarea v-model="correctionText" class="try__correction mono" rows="2" placeholder="What should change in the plan?"></textarea>
          <div class="try__correctbox-actions">
            <GfButton variant="secondary" size="s" @click="showCorrect = false">Cancel</GfButton>
            <GfButton variant="primary" size="s" @click="submitCorrection">Submit correction</GfButton>
          </div>
        </div>

        <p class="try__control-note"><GfIcon name="info" :size="12" /> You are in control — approve, ask for a correction, or reject.</p>
        <div class="try__actions">
          <GfButton variant="primary" @click="approve"><GfIcon name="check" :size="16" /> Approve &amp; generate code</GfButton>
          <GfButton variant="secondary" @click="showCorrect = !showCorrect"><GfIcon name="refresh" :size="15" /> Request correction</GfButton>
          <GfButton variant="danger" @click="reject">Reject</GfButton>
        </div>
      </template>

      <!-- rejected -->
      <template v-else-if="step === 'rejected'">
        <div class="try__rejected">
          <p class="try__rejected-head"><GfIcon name="close" :size="15" /> Plan rejected — nothing was generated.</p>
        </div>
        <div class="try__cta">
          <GfButton variant="secondary" @click="reset"><GfIcon name="refresh" :size="14" /> Start over</GfButton>
        </div>
      </template>

      <!-- done -->
      <template v-else>
        <div class="try__done">
          <p class="try__done-head"><GfIcon name="check" :size="15" /> Plan approved · code generated</p>
          <ul class="try__files">
            <li v-for="f in files" :key="f.path">
              <span class="try__act" :class="`try__act_${f.action}`">{{ f.action }}</span>
              <span class="mono try__path">{{ f.path }}</span>
            </li>
          </ul>
          <p class="try__branch"><GfIcon name="branch" :size="13" /> branch <span class="mono">ai/ISSUE-101-add-pagination</span> · PR ready for review</p>
        </div>
        <div class="try__cta">
          <GfButton variant="secondary" @click="reset"><GfIcon name="refresh" :size="14" /> Run it again</GfButton>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.try {
  border: 1px solid var(--gf-line-2);
  border-radius: var(--gf-radius-lg);
  background: var(--gf-surface);
  overflow: hidden;
  box-shadow: var(--gf-shadow-sm);
}
.try__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px;
  padding: 11px 16px;
  background: var(--gf-surface-2);
  border-bottom: 1px solid var(--gf-line);
}
.try__badge {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 12px;
  font-weight: 700;
  color: var(--gf-accent);
}
.try__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--gf-purple);
  animation: blink 1.6s infinite;
}
@keyframes blink { 50% { opacity: 0.3; } }
.try__hint {
  font-size: 11.5px;
  color: var(--gf-text-3);
}
.try__body {
  padding: 16px;
  display: grid;
  gap: 12px;
}
.try__issue {
  padding: 12px 14px;
  border: 1px solid var(--gf-line);
  border-radius: var(--gf-radius);
  background: var(--gf-surface-2);
}
.try__label {
  display: inline-block;
  margin-bottom: 6px;
  font-size: 10.5px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--gf-text-3);
}
.try__issue-title {
  margin: 0 0 4px;
  font-size: 13.5px;
  font-weight: 700;
  color: var(--gf-text);
}
.try__issue-body {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--gf-text-2);
}
.try__cta {
  display: flex;
  justify-content: center;
  flex-wrap: wrap;
}
.try__loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 18px 0;
  color: var(--gf-text-2);
  font-size: 13px;
}
.try__spin {
  width: 18px;
  height: 18px;
  border: 2.5px solid var(--gf-line-2);
  border-top-color: var(--gf-purple);
  border-radius: 50%;
  animation: try-spin 0.7s linear infinite;
}
@keyframes try-spin { to { transform: rotate(360deg); } }
.try__planhead {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.try__plan-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  border-radius: 999px;
  background: var(--gf-purple-soft);
  color: var(--gf-accent);
  font-size: 11.5px;
  font-weight: 700;
}
.try__rev {
  font-size: 11.5px;
  color: var(--gf-text-3);
}
.try__correctbox {
  display: grid;
  gap: 8px;
}
.try__correction {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid var(--gf-line-2);
  border-radius: var(--gf-radius);
  background: var(--gf-surface);
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--gf-text);
  resize: vertical;
  outline: none;
}
.try__correction:focus {
  border-color: var(--gf-purple);
}
.try__correctbox-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.try__control-note {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  font-size: 11.5px;
  color: var(--gf-text-3);
}
.try__control-note :deep(.gf-icon) { color: var(--gf-purple); }
.try__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.try__rejected {
  padding: 14px 16px;
  border: 1px solid #e2b4b4;
  border-radius: var(--gf-radius);
  background: #fdecec;
}
.try__rejected-head {
  display: flex;
  align-items: center;
  gap: 7px;
  margin: 0;
  font-size: 13px;
  font-weight: 700;
  color: #c0392b;
}
.try__done {
  padding: 14px 16px;
  border: 1px solid var(--gf-green);
  border-radius: var(--gf-radius);
  background: var(--gf-green-bg);
}
.try__done-head {
  display: flex;
  align-items: center;
  gap: 7px;
  margin: 0 0 10px;
  font-size: 13.5px;
  font-weight: 700;
  color: var(--gf-green);
}
.try__files {
  list-style: none;
  margin: 0 0 10px;
  padding: 0;
  display: grid;
  gap: 6px;
}
.try__files li {
  display: flex;
  align-items: center;
  gap: 9px;
}
.try__act {
  flex: none;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 10.5px;
  font-weight: 700;
  text-transform: uppercase;
}
.try__act_create { color: var(--gf-green); background: #fff; }
.try__act_modify { color: var(--gf-blue); background: var(--gf-blue-bg); }
.try__path {
  font-size: 12px;
  color: var(--gf-text);
  word-break: break-all;
}
.try__branch {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  font-size: 12px;
  color: var(--gf-text-2);
}
.try__branch :deep(.gf-icon) { color: var(--gf-green); flex: none; }
</style>
