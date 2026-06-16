<script setup>
import { ref } from 'vue'
import GfModal from './ui/GfModal.vue'
import GfIcon from './ui/GfIcon.vue'
import YamlConfigPanel from './YamlConfigPanel.vue'
import IssuePlanPanel from './IssuePlanPanel.vue'

defineEmits(['close'])

// Two sub-flows live behind a small tab switcher inside the modal:
//   1. configure  -> build a valid .ai.yml for the repository
//   2. issue      -> analyze an issue and run the plan approve/correct/reject loop
const tab = ref('configure')

const tabs = [
  { id: 'configure', label: 'Configure AI', icon: 'shield' },
  { id: 'issue', label: 'Work on an issue', icon: 'sparkles' },
]
</script>

<template>
  <GfModal
    wide
    title="Work with AI"
    subtitle="Configure repository AI behaviour or generate an implementation plan from an issue"
    @close="$emit('close')"
  >
    <div class="wizard">
      <nav class="wizard__tabs" aria-label="AI actions">
        <button
          v-for="t in tabs"
          :key="t.id"
          class="wizard__tab"
          :class="{ wizard__tab_active: t.id === tab }"
          @click="tab = t.id"
        >
          <GfIcon :name="t.icon" :size="16" />
          {{ t.label }}
        </button>
      </nav>

      <div class="wizard__panel">
        <YamlConfigPanel v-if="tab === 'configure'" />
        <IssuePlanPanel v-else />
      </div>
    </div>
  </GfModal>
</template>

<style scoped>
.wizard__tabs {
  display: flex;
  gap: 6px;
  padding: 4px;
  border-radius: var(--gf-radius);
  background: var(--gf-surface-2);
  border: 1px solid var(--gf-line);
}
.wizard__tab {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 9px 12px;
  border: 0;
  border-radius: 11px;
  background: transparent;
  color: var(--gf-text-2);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.wizard__tab:hover {
  color: var(--gf-text);
}
.wizard__tab_active {
  background: var(--gf-surface);
  color: var(--gf-accent);
  box-shadow: 0 1px 2px rgba(39, 39, 53, 0.08);
}
.wizard__panel {
  margin-top: 18px;
}
</style>
