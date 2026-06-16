<script setup>
import { reactive, computed, ref } from 'vue'
import GfIcon from './ui/GfIcon.vue'
import GfButton from './ui/GfButton.vue'

const ALL_CATEGORIES = [
  { id: 'code_duplication', label: 'Code duplication' },
  { id: 'security', label: 'Security' },
  { id: 'maintainability', label: 'Maintainability' },
  { id: 'performance', label: 'Performance' },
  { id: 'architecture', label: 'Architecture' },
]

const form = reactive({
  defaultBranch: 'main',
  branchPrefix: 'ai/',
  analysisEnabled: true,
  includePaths: 'src/**, internal/**',
  excludePaths: 'node_modules/**, dist/**, build/**, .git/**',
  codeGenEnabled: true,
  requireApproval: true,
  recommendationsEnabled: true,
  severityThreshold: 'low',
  categories: ALL_CATEGORIES.map((c) => c.id),
  maxFiles: 20,
})

const copied = ref(false)

function listToYaml(value, indent) {
  const items = value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
  if (!items.length) return `${indent}[]`
  return items.map((i) => `${indent}- ${i}`).join('\n')
}

const yaml = computed(() => {
  const lines = []
  lines.push('version: 1')
  lines.push('repository:')
  lines.push(`  default_branch: ${form.defaultBranch}`)
  lines.push(`  target_branch_prefix: ${form.branchPrefix}`)
  lines.push('analysis:')
  lines.push(`  enabled: ${form.analysisEnabled}`)
  lines.push('  include:')
  lines.push(listToYaml(form.includePaths, '    '))
  lines.push('  exclude:')
  lines.push(listToYaml(form.excludePaths, '    '))
  lines.push('code_generation:')
  lines.push(`  enabled: ${form.codeGenEnabled}`)
  lines.push(`  require_user_approval: ${form.requireApproval}`)
  lines.push('  reviewer_policy: issue_author')
  lines.push('  allowed_actions:')
  lines.push('    approve_command: "/approve"')
  lines.push('    correct_command: "/correct"')
  lines.push('    reject_command: "/reject"')
  lines.push('recommendations:')
  lines.push(`  enabled: ${form.recommendationsEnabled}`)
  lines.push(`  severity_threshold: ${form.severityThreshold}`)
  lines.push('  categories:')
  if (form.categories.length) {
    form.categories.forEach((c) => lines.push(`    - ${c}`))
  } else {
    lines.push('    []')
  }
  lines.push('rag:')
  lines.push(`  max_files: ${form.maxFiles}`)
  lines.push('  max_file_size_kb: 120')
  lines.push('  context_strategy: issue_relevant_files')
  lines.push('storage:')
  lines.push('  recommendation_ttl_days: 30')
  return lines.join('\n')
})

async function copyYaml() {
  try {
    await navigator.clipboard.writeText(yaml.value)
    copied.value = true
    setTimeout(() => (copied.value = false), 1600)
  } catch {
    copied.value = false
  }
}
</script>

<template>
  <div class="panel">
    <p class="panel__intro gf-muted">
      Choose how the AI assistant should behave for this repository. The settings below
      generate a valid <code>.ai.yml</code> you can commit to the repository root.
    </p>

    <div class="grid">
      <label class="field">
        <span class="field__label">Default branch</span>
        <input v-model="form.defaultBranch" class="input" />
      </label>
      <label class="field">
        <span class="field__label">AI branch prefix</span>
        <input v-model="form.branchPrefix" class="input" />
      </label>
    </div>

    <label class="field">
      <span class="field__label">Analyze paths (comma-separated globs)</span>
      <input v-model="form.includePaths" class="input" />
    </label>
    <label class="field">
      <span class="field__label">Exclude paths</span>
      <input v-model="form.excludePaths" class="input" />
    </label>

    <div class="toggles">
      <label class="toggle">
        <input v-model="form.codeGenEnabled" type="checkbox" />
        <span>Enable code generation from issues</span>
      </label>
      <label class="toggle">
        <input v-model="form.requireApproval" type="checkbox" />
        <span>Require user approval before creating a PR</span>
      </label>
      <label class="toggle">
        <input v-model="form.recommendationsEnabled" type="checkbox" />
        <span>Enable recommendation analysis</span>
      </label>
    </div>

    <div class="grid">
      <label class="field">
        <span class="field__label">Severity threshold</span>
        <select v-model="form.severityThreshold" class="input">
          <option value="low">low</option>
          <option value="medium">medium</option>
          <option value="high">high</option>
        </select>
      </label>
      <label class="field">
        <span class="field__label">Max files in context</span>
        <input v-model.number="form.maxFiles" type="number" min="1" max="100" class="input" />
      </label>
    </div>

    <div class="field">
      <span class="field__label">Recommendation categories</span>
      <div class="chips">
        <label
          v-for="c in ALL_CATEGORIES"
          :key="c.id"
          class="chip"
          :class="{ chip_on: form.categories.includes(c.id) }"
        >
          <input v-model="form.categories" type="checkbox" :value="c.id" />
          {{ c.label }}
        </label>
      </div>
    </div>

    <div class="preview">
      <div class="preview__head">
        <span class="preview__title">Generated <code>.ai.yml</code></span>
        <GfButton variant="secondary" size="s" @click="copyYaml">
          <GfIcon :name="copied ? 'check' : 'copy'" :size="14" />
          {{ copied ? 'Copied' : 'Copy' }}
        </GfButton>
      </div>
      <pre class="preview__code mono">{{ yaml }}</pre>
    </div>
  </div>
</template>

<style scoped>
.panel__intro {
  margin: 0 0 16px;
  font-size: 13px;
  line-height: 1.55;
}
.panel__intro code,
.preview__title code {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  color: var(--gf-accent);
}
.grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
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
.input {
  width: 100%;
  height: 36px;
  padding: 0 12px;
  border: 1px solid var(--gf-line-2);
  border-radius: 10px;
  font: inherit;
  font-size: 13px;
  color: var(--gf-text);
  background: var(--gf-surface);
}
select.input {
  padding: 0 8px;
}
.input:focus {
  outline: none;
  border-color: var(--gf-purple);
}
.toggles {
  display: grid;
  gap: 10px;
  margin: 4px 0 16px;
}
.toggle {
  display: flex;
  align-items: center;
  gap: 9px;
  font-size: 13px;
  cursor: pointer;
}
.toggle input {
  width: 16px;
  height: 16px;
  accent-color: var(--gf-purple);
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 12px;
  border: 1px solid var(--gf-line-2);
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  color: var(--gf-text-2);
  cursor: pointer;
  user-select: none;
}
.chip input {
  display: none;
}
.chip_on {
  border-color: var(--gf-purple);
  color: var(--gf-accent);
  background: var(--gf-purple-soft);
}
.preview {
  margin-top: 6px;
  border: 1px solid var(--gf-line);
  border-radius: 12px;
  overflow: hidden;
}
.preview__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  background: var(--gf-surface-2);
  border-bottom: 1px solid var(--gf-line);
}
.preview__title {
  font-size: 12px;
  font-weight: 600;
  color: var(--gf-text-2);
}
.preview__code {
  margin: 0;
  padding: 14px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--gf-text);
  background: var(--gf-surface);
  max-height: 260px;
  overflow: auto;
  white-space: pre;
}
@media (max-width: 560px) {
  .grid {
    grid-template-columns: 1fr;
  }
}
</style>
