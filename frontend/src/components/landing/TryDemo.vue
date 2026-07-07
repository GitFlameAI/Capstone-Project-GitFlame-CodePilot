<script setup>
// "Try it yourself" — a tiny, self-contained interactive preview of the flow for
// visitors who would rather click than read. It is deliberately a MOCK: no
// backend, no repository, local timers only, clearly badged as a preview. It
// mirrors the real flow's emphasis on USER CONTROL: you can edit the plan, and
// approve / request a correction / reject it yourself.
import { ref, onBeforeUnmount } from 'vue'
import GfIcon from '../ui/GfIcon.vue'

// step: idle -> planning -> plan -> correcting -> generating -> done -> rejected
const step = ref('idle')
const view = ref('preview') // preview | edit
const revised = ref(false)
let timer = null

const basePlan = `# Implementation Plan

## Issue summary
Add pagination to the repository list endpoint.

## Steps
1. Read the current handler and confirm the response shape.
2. Parse ?limit and ?offset with safe defaults.
3. Return items plus a total count for the UI.`

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
function requestCorrection() {
  clear()
  step.value = 'correcting'
  timer = setTimeout(() => {
    planText.value = basePlan + '\n4. Add a test covering the empty and last page.'
    revised.value = true
    view.value = 'preview'
    step.value = 'plan'
  }, 1200)
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
  revised.value = false
  view.value = 'preview'
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
        <button class="try__btn" @click="generate"><GfIcon name="sparkles" :size="15" /> Generate plan</button>
      </div>

      <!-- planning / correcting / generating -->
      <div v-else-if="step === 'planning'" class="try__loading"><span class="try__spin" /> Drafting a plan…</div>
      <div v-else-if="step === 'correcting'" class="try__loading"><span class="try__spin" /> Revising the plan…</div>
      <div v-else-if="step === 'generating'" class="try__loading"><span class="try__spin" /> Generating code…</div>

      <!-- plan: you review, edit and decide -->
      <template v-else-if="step === 'plan'">
        <div class="try__plan-head">
          <span class="try__plan-status">
            <GfIcon name="check" :size="13" /> {{ revised ? 'Plan revised' : 'Plan generated' }}
          </span>
          <div class="try__viewtabs">
            <button class="try__viewtab" :class="{ 'try__viewtab_on': view === 'edit' }" @click="view = 'edit'"><GfIcon name="pencil" :size="12" /> Edit</button>
            <button class="try__viewtab" :class="{ 'try__viewtab_on': view === 'preview' }" @click="view = 'preview'"><GfIcon name="eye" :size="12" /> Preview</button>
          </div>
        </div>

        <textarea v-if="view === 'edit'" v-model="planText" class="try__editor mono" spellcheck="false"></textarea>
        <pre v-else class="try__plan">{{ planText }}</pre>

        <p class="try__control-note"><GfIcon name="info" :size="12" /> You are in control — approve, ask for a correction, or reject.</p>
        <div class="try__cta try__cta_row">
          <button class="try__btn" @click="approve"><GfIcon name="check" :size="15" /> Approve &amp; generate code</button>
          <button class="try__btn try__btn_ghost" @click="requestCorrection"><GfIcon name="refresh" :size="14" /> Request correction</button>
          <button class="try__btn try__btn_reject" @click="reject">Reject</button>
        </div>
      </template>

      <!-- rejected -->
      <template v-else-if="step === 'rejected'">
        <div class="try__rejected">
          <p class="try__rejected-head"><GfIcon name="close" :size="15" /> Plan rejected — nothing was generated.</p>
        </div>
        <div class="try__cta">
          <button class="try__btn try__btn_ghost" @click="reset"><GfIcon name="refresh" :size="14" /> Start over</button>
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
          <button class="try__btn try__btn_ghost" @click="reset"><GfIcon name="refresh" :size="14" /> Run it again</button>
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
.try__cta_row { gap: 10px; }
.try__btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 38px;
  padding: 0 16px;
  border: 0;
  border-radius: var(--gf-radius);
  background: var(--gf-purple);
  color: #fff;
  font: inherit;
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}
.try__btn:hover { background: var(--gf-purple-hover); }
.try__btn_ghost {
  background: transparent;
  border: 1px solid var(--gf-line-2);
  color: var(--gf-text-2);
}
.try__btn_ghost:hover { background: var(--gf-surface-3); color: var(--gf-text); }
.try__btn_reject {
  background: transparent;
  border: 1px solid #e2b4b4;
  color: #c0392b;
}
.try__btn_reject:hover { background: #fdecec; border-color: #c0392b; }
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
.try__plan-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
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
.try__viewtabs {
  display: inline-flex;
  gap: 3px;
  padding: 3px;
  border: 1px solid var(--gf-line-2);
  border-radius: 8px;
  background: var(--gf-surface-2);
}
.try__viewtab {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 24px;
  padding: 0 9px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--gf-text-2);
  font: inherit;
  font-size: 11.5px;
  font-weight: 600;
  cursor: pointer;
}
.try__viewtab_on {
  background: var(--gf-surface);
  color: var(--gf-accent);
  box-shadow: var(--gf-shadow-sm);
}
.try__plan {
  margin: 0;
  padding: 14px 16px;
  border: 1px solid var(--gf-line);
  border-radius: var(--gf-radius);
  background: var(--gf-surface);
  font-size: 12px;
  line-height: 1.55;
  color: var(--gf-text);
  white-space: pre-wrap;
  max-height: 190px;
  overflow: auto;
}
.try__editor {
  width: 100%;
  min-height: 160px;
  padding: 14px 16px;
  border: 1px solid var(--gf-purple);
  border-radius: var(--gf-radius);
  background: var(--gf-surface);
  font-size: 12px;
  line-height: 1.55;
  color: var(--gf-text);
  resize: vertical;
  outline: none;
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
