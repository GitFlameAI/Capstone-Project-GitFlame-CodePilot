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
import { reactive, ref, computed, watch } from 'vue'
import { session, saveConfig } from '../../store/session.js'
import { buildYaml, RECOMMENDATION_CATEGORIES, excludePathOptions } from '../../data/demo.js'
import GfIcon from '../ui/GfIcon.vue'
import GfButton from '../ui/GfButton.vue'
import GfTooltip from '../ui/GfTooltip.vue'
import ContextPicker from '../ContextPicker.vue'

const emit = defineEmits(['saved'])

// Local editable copy of the form, seeded from the session (arrays copied so
// editing before Save does not mutate the stored configuration).
const form = reactive({
  defaultBranch: session.repo.defaultBranch || session.configForm.defaultBranch,
  excludePaths: [...(session.configForm.excludePaths || [])],
  categories: [...(session.configForm.categories || [])],
  retentionDays: session.configForm.retentionDays,
})
const saving = ref(false)
const justSaved = ref(false)
const copied = ref(false)

const yamlPreview = computed(() => buildYaml(form))
const noCategories = computed(() => !form.categories || form.categories.length === 0)

function toggleCategory(id) {
  const i = form.categories.indexOf(id)
  if (i === -1) form.categories.push(id)
  else form.categories.splice(i, 1)
}

watch(form, () => { justSaved.value = false }, { deep: true })

async function save() {
  saving.value = true
  await new Promise((r) => setTimeout(r, 450))
  saveConfig({ ...form })
  saving.value = false
  justSaved.value = true
}

async function copyYaml() {
  try {
    await navigator.clipboard.writeText(yamlPreview.value)
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  } catch {
    /* non-critical */
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
        <h3 class="sect__title">Analysis</h3>
        <div class="field">
          <span class="field__label">Exclude paths
            <GfTooltip text="Glob patterns CodePilot must ignore during analysis (build output, vendored code, etc.). Type to pick a common pattern or add your own." /></span>
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
            <GfTooltip text="Problem categories the system looks for. If none are selected, no recommendations are produced." /></span>
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
            <GfTooltip text="How long generated recommendation reports are retained before they expire." /></span>
          <input v-model.number="form.retentionDays" type="number" min="1" max="365" class="input input_sm" placeholder="30" />
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

      <GfButton variant="primary" size="l" :loading="saving" class="savebtn" @click="save">
        <GfIcon name="check" :size="16" /> Save .ai.yml
      </GfButton>
      <p class="savehint gf-muted">
        Saves to the <span class="mono">{{ form.defaultBranch || 'main' }}</span> branch and unlocks
        Autogeneration &amp; Recommendations.
      </p>

      <transition name="okfade">
        <p v-if="justSaved" class="okmsg">
          <GfIcon name="check" :size="15" />
          Saved. <button class="inline-link" @click="emit('saved')">Go to Autogeneration →</button>
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
}
.cfg__form {
  display: grid;
  gap: 16px;
  min-width: 0;
}
.sect {
  padding: 18px 20px;
}
.sect__title {
  margin: 0 0 14px;
  font-size: 14px;
  color: var(--gf-accent);
}
.field {
  display: block;
  margin-bottom: 12px;
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
  margin-bottom: 7px;
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
.savebtn {
  width: 100%;
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
