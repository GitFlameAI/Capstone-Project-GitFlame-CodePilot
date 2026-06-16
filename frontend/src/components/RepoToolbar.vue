<script setup>
import GfIcon from './ui/GfIcon.vue'
import GfButton from './ui/GfButton.vue'

defineProps({
  repo: { type: Object, required: true },
})
const emit = defineEmits(['work-with-ai'])
</script>

<template>
  <div class="toolbar">
    <!-- Branch selector (left) -->
    <button class="branch">
      <GfIcon name="branch" :size="16" />
      <span class="branch__name">{{ repo.defaultBranch }}</span>
      <GfIcon name="chevronDown" :size="14" />
    </button>

    <span class="toolbar__meta gf-muted">
      {{ repo.branches.length }} branches
    </span>

    <div class="toolbar__spacer" />

    <!-- Right-side actions, mirroring GitFlame: History + Access + Work with AI -->
    <GfButton variant="secondary">
      <GfIcon name="history" :size="16" />
      History
    </GfButton>

    <GfButton variant="secondary">
      <GfIcon name="lock" :size="16" />
      Access
    </GfButton>

    <!-- Signature purple action that opens the AI flow -->
    <GfButton variant="primary" @click="emit('work-with-ai')">
      <GfIcon name="sparkles" :size="16" />
      Work with AI
    </GfButton>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 14px;
}
.branch {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 34px;
  padding: 0 14px;
  border: 1px solid var(--gf-line-2);
  border-radius: var(--gf-radius);
  background: var(--gf-surface);
  color: var(--gf-text);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.branch:hover {
  border-color: var(--gf-purple);
}
.branch__name {
  font-family: 'JetBrains Mono', monospace;
}
.toolbar__meta {
  font-size: 13px;
}
.toolbar__spacer {
  flex: 1;
}
</style>
