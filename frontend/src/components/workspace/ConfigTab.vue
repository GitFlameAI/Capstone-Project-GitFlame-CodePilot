<script setup>
// Config tab — the user-facing editor for the repository's .ai.yml.
//
// The form mirrors the agreed configuration contract in
//   docs/config/ai_config_spec.md (branch: sprint-3/danil-codegen-contracts)
// which is intentionally small. Only four things are configurable:
//   - repository.default_branch
//   - analysis.exclude            (paths AI ignores — chip multi-select)
//   - recommendations.categories  (what the system looks for)
//   - storage.recommendation_ttl_days
// Everything the old form exposed (include paths, RAG limits, severity threshold,
// the code-generation toggles, reviewer) was dropped from the contract, so it is
// not shown here. A live .ai.yml preview updates as the form changes; saving
// unlocks the Autogeneration and Recommendations tabs.
import { computed, ref, watch } from 'vue'
import { session, saveConfig, updateDraftExcludePaths, configDirty, persistDraft, resetDraft } from '../../store/session.js'
import { buildYaml, RECOMMENDATION_CATEGORIES } from '../../data/demo.js'
import GfIcon from '../ui/GfIcon.vue'
import GfButton from '../ui/GfButton.vue'
import GfTooltip from '../ui/GfTooltip.vue'
import ContextPicker from '../ContextPicker.vue'
import { copyText } from '../../utils/clipboard.js'
import { excludePathOptionsFromTree } from '../../utils/excludePaths.js'

const emit = defineEmits(['go'])

// The Config form edits the shared DRAFT held in the session, so edits persist
// across tab switches and stay in sync with the Repository file-tree Exclude
// toggles. The draft only becomes the saved .ai.yml when the user presses Save.
const form = computed(() => session.configDraft)

const saving = ref(false)
const justSaved = ref(false)
const copied = ref(false)

const yamlPreview = computed(() => buildYaml(session.configDraft))
const noCategories = computed(() => (session.configDraft.categories || []).length === 0)
const allCategoriesOn = computed(() => session.configDraft.categories.length === RECOMMENDATION_CATEGORIES.length)
const dirty = computed(() => configDirty())

// "Exclude paths" suggestions come from the REAL connected repository tree (both
// in live and demo mode) — no hard-coded placeholder patterns. Empty until the
// repository files have loaded; the user can still type a custom pattern.
const excludePathOptions = computed(() => excludePathOptionsFromTree(session.fileTree))

function toggleCategory(id) {
  const cats = session.configDraft.categories
  const i = cats.indexOf(id)
  if (i === -1) cats.push(id)
  else cats.splice(i, 1)
  persistDraft()
}
function allCategories() {
  session.configDraft.categories = RECOMMENDATION_CATEGORIES.map((c) => c.id)
  persistDraft()
}
function clearCategories() {
  session.configDraft.categories = []
  persistDraft()
}
function clearExcludes() {
  updateDraftExcludePaths([])
}

// Guard the retention input: keep it a whole number in [1, 365]. The number input
// still lets users type "-5" or "1.5"; clamp on every change.
function clampRetention() {
  let n = Math.floor(Number(session.configDraft.retentionDays))
  if (!Number.isFinite(n)) n = 30
  session.configDraft.retentionDays = Math.min(365, Math.max(1, n))
  persistDraft()
}

// Never allow the .ai.yml config file (or blank entries) into exclude paths; persist
// the draft on any exclude-list change (chip picker or tree toggle).
watch(
  () => session.configDraft.excludePaths.slice(),
  (paths) => {
    const clean = paths.filter((p) => p && p.trim() && p.trim() !== '.ai.yml')
    if (clean.length !== paths.length) {
      session.configDraft.excludePaths = clean
      return
    }
    persistDraft()
  },
  { deep: true },
)

// Persist typed field edits (branch, retention) so they survive tab switches.
watch(
  () => [session.configDraft.defaultBranch, session.configDraft.retentionDays],
  () => persistDraft(),
)
// Hide the "saved" banner as soon as there are unsaved changes again (but keep it
// showing right after a save, when the draft equals the saved config).
watch(dirty, (isDirty) => {
  if (isDirty) justSaved.value = false
})

async function save() {
  clampRetention()
  saving.value = true
  await new Promise((r) => setTimeout(r, 450))
  saveConfig(session.configDraft)
  saving.value = false
  justSaved.value = true
}

// Discard every unsaved change: revert the working draft to the saved config.
function discardChanges() {
  resetDraft()
  justSaved.value = false
}

async function copyYaml() {
  const ok = await copyText(yamlPreview.value)
  if (ok) {
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  }
}
</script>

<template>
  <div class="cfg">
    <div class="cfg__form">
      <!-- Repository -->
      <section class="sect gf-card">
        <h3 class="sect__title">Repository</h3>
        <label class="field">
          <span class="field__label">Default branch
            <GfTooltip text="Branch CodePilot reads from and where the .ai.yml is saved. Also the base for future generated branches." /></span>
          <input v-model="form.defaultBranch" class="input" placeholder="main" />
        </label>
      </section>

      <!-- Analysis -->
      <section class="sect gf-card">
        <div class="sect__head">
          <h3 class="sect__title">Analysis</h3>
          <button v-if="form.excludePaths.length" class="linkbtn" @click="clearExcludes">Clear all</button>
        </div>
        <div class="field">
          <span class="field__label">Exclude paths
            <GfTooltip text="Glob patterns CodePilot must ignore during analysis (build output, vendored code, etc.). Type to pick a common pattern or add your own. You can also toggle files and folders directly in the Repository tab." /></span>
          <ContextPicker
            v-model="form.excludePaths"
            :options="excludePathOptions"
            placeholder="Type a pattern, e.g. dist/**"
          />
        </div>
      </section>

      <!-- Recommendations -->
      <section class="sect gf-card">
        <h3 class="sect__title">Recommendations</h3>
        <div class="field">
          <span class="field__label">Categories
            <GfTooltip text="Problem categories the system looks for. If none are selected, no recommendations are produced." />
            <span class="field__quick">
              <button class="linkbtn" :disabled="allCategoriesOn" @click="allCategories">All</button>
              <span class="field__sep">·</span>
              <button class="linkbtn" :disabled="noCategories" @click="clearCategories">None</button>
            </span>
          </span>
          <div class="cats">
            <button
              v-for="c in RECOMMENDATION_CATEGORIES"
              :key="c.id"
              class="cat"
              :class="{ cat_on: form.categories.includes(c.id) }"
              @click="toggleCategory(c.id)"
            >
              <GfIcon v-if="form.categories.includes(c.id)" name="check" :size="13" />
              {{ c.label }}
            </button>
          </div>
          <p v-if="noCategories" class="field__warn">
            <GfIcon name="alert" :size="13" /> No categories selected — the system won't produce any recommendations.
          </p>
        </div>
        <label class="field">
          <span class="field__label">Keep reports for (days)
            <GfTooltip text="How long generated recommendation reports are retained before they expire. A whole number between 1 and 365." /></span>
          <input
            v-model.number="form.retentionDays"
            type="number"
            min="1"
            max="365"
            step="1"
            inputmode="numeric"
            class="input input_sm"
            placeholder="30"
            @input="clampRetention"
            @blur="clampRetention"
          />
        </label>
      </section>
    </div>

    <!-- Live YAML preview + save -->
    <aside class="cfg__preview">
      <div class="preview gf-card">
        <header class="preview__head">
          <span class="preview__title mono">.ai.yml</span>
          <button class="preview__copy" :title="copied ? 'Copied' : 'Copy'" @click="copyYaml">
            <GfIcon :name="copied ? 'check' : 'copy'" :size="15" />
          </button>
        </header>
        <pre class="preview__code mono">{{ yamlPreview }}</pre>
      </div>

      <div class="saverow">
        <GfButton variant="primary" size="l" :loading="saving" :disabled="!dirty && session.configExists" class="savebtn" @click="save">
          <GfIcon name="check" :size="16" /> {{ session.configExists && !dirty ? 'Saved' : 'Save .ai.yml' }}
        </GfButton>
        <GfButton v-if="dirty" variant="secondary" size="l" @click="discardChanges">
          Discard changes
        </GfButton>
      </div>
      <p class="savehint gf-muted">
        <template v-if="!session.configExists">
          Save to unlock Autogeneration &amp; Recommendations.
        </template>
        <template v-else-if="dirty">
          Save changes to the <span class="mono">{{ form.defaultBranch || 'main' }}</span> branch.
        </template>
        <template v-else>
          All changes are saved to the <span class="mono">{{ form.defaultBranch || 'main' }}</span> branch.
        </template>
      </p>

      <transition name="okfade">
        <p v-if="justSaved && !dirty" class="okmsg">
          <GfIcon name="check" :size="15" />
          Configuration saved. <button class="inline-link" @click="emit('go', 'autogen')">Go to Autogeneration →</button>
        </p>
      </transition>
    </aside>
  </div>
</template>

<style scoped>
.cfg {
  display: grid;
  grid-template-columns: 1fr 360px;
  gap: 20px;
  align-items: start;
  width: 100%;
  max-width: var(--ws-content);
  margin: 0 auto;
}
.cfg__form {
  display: grid;
  gap: 12px;
  min-width: 0;
}
.sect {
  padding: 14px 18px;
}
.sect__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}
.sect__head .sect__title {
  margin: 0;
}
.sect__title {
  margin: 0 0 12px;
  font-size: 14px;
  color: var(--gf-accent);
}
.linkbtn {
  border: 0;
  background: transparent;
  padding: 0;
  font: inherit;
  font-size: 12px;
  font-weight: 700;
  color: var(--gf-accent);
  cursor: pointer;
}
.linkbtn:hover:not(:disabled) {
  text-decoration: underline;
}
.linkbtn:disabled {
  color: var(--gf-text-3);
  cursor: default;
}
.field__quick {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}
.field__sep {
  color: var(--gf-text-3);
}
.field {
  display: block;
  margin-bottom: 10px;
}
.field:last-child {
  margin-bottom: 0;
}
.field__label {
  display: flex;
  align-items: center;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--gf-text-2);
  margin-bottom: 6px;
}
.field__warn {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 8px 0 0;
  font-size: 12px;
  color: var(--gf-amber);
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
.input_sm {
  max-width: 140px;
}
.cats {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.cat {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 30px;
  padding: 0 12px;
  border: 1px solid var(--gf-line-2);
  border-radius: 999px;
  background: var(--gf-surface);
  color: var(--gf-text-2);
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
}
.cat:hover {
  border-color: var(--gf-purple);
}
.cat_on {
  border-color: var(--gf-purple);
  background: var(--gf-purple-soft);
  color: var(--gf-accent);
}

.cfg__preview {
  position: sticky;
  top: 16px;
}
.preview {
  overflow: hidden;
  margin-bottom: 14px;
}
.preview__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  border-bottom: 1px solid var(--gf-line);
  background: var(--gf-surface-2);
}
.preview__title {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--gf-accent);
}
.preview__copy {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--gf-text-3);
  cursor: pointer;
}
.preview__copy:hover {
  background: var(--gf-surface-3);
  color: var(--gf-text);
}
.preview__code {
  margin: 0;
  padding: 14px;
  font-size: 12px;
  line-height: 1.55;
  white-space: pre;
  overflow: auto;
  max-height: 420px;
  color: var(--gf-text);
}
.saverow {
  display: flex;
  gap: 10px;
}
.savebtn {
  flex: 1;
}
.savehint {
  margin: 10px 0 0;
  font-size: 12px;
  line-height: 1.45;
}
.okmsg {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 12px 0 0;
  padding: 9px 12px;
  border-radius: 10px;
  background: var(--gf-green-bg);
  color: var(--gf-green);
  font-size: 12.5px;
}
.inline-link {
  border: 0;
  background: transparent;
  color: var(--gf-green);
  font: inherit;
  font-weight: 700;
  cursor: pointer;
  padding: 0;
}
.okfade-enter-active {
  transition: opacity 0.2s ease;
}
.okfade-enter-from {
  opacity: 0;
}
@media (max-width: 860px) {
  .cfg {
    grid-template-columns: 1fr;
  }
  .cfg__preview {
    position: static;
  }
}
</style>
