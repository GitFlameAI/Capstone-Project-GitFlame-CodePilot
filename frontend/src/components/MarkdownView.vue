<script setup>
// Markdown editor with an Edit / Preview toggle, like the GitHub file editor.
// Used for the generated implementation plan so the user can tweak wording
// ("change half a word") before approving. v-model binds the raw markdown text.
import { computed } from 'vue'
import { renderMarkdown } from '../utils/markdown.js'
import GfIcon from './ui/GfIcon.vue'

const props = defineProps({
  modelValue: { type: String, default: '' },
  // when true the component is read-only (no Edit tab, just rendered markdown)
  readonly: { type: Boolean, default: false },
  mode: { type: String, default: 'preview' }, // 'edit' | 'preview'
  rows: { type: Number, default: 16 },
})
const emit = defineEmits(['update:modelValue', 'update:mode'])

const html = computed(() => renderMarkdown(props.modelValue || ''))

function setMode(m) {
  if (!props.readonly) emit('update:mode', m)
}
function onInput(e) {
  emit('update:modelValue', e.target.value)
}
</script>

<template>
  <div class="md">
    <div v-if="!readonly" class="md__tabs" role="tablist">
      <button
        class="md__tab"
        :class="{ md__tab_active: mode === 'edit' }"
        role="tab"
        :aria-selected="mode === 'edit'"
        @click="setMode('edit')"
      >
        <GfIcon name="pencil" :size="14" /> Edit
      </button>
      <button
        class="md__tab"
        :class="{ md__tab_active: mode === 'preview' }"
        role="tab"
        :aria-selected="mode === 'preview'"
        @click="setMode('preview')"
      >
        <GfIcon name="eye" :size="14" /> Preview
      </button>
    </div>

    <textarea
      v-if="!readonly && mode === 'edit'"
      class="md__editor mono"
      :rows="rows"
      :value="modelValue"
      spellcheck="false"
      @input="onInput"
    />
    <!-- eslint-disable-next-line vue/no-v-html — input is escaped in renderMarkdown -->
    <div v-else class="md__preview" v-html="html" />
  </div>
</template>

<style scoped>
.md {
  border: 1px solid var(--gf-line);
  border-radius: 12px;
  overflow: hidden;
  background: var(--gf-surface);
}
.md__tabs {
  display: flex;
  gap: 2px;
  padding: 6px 6px 0;
  background: var(--gf-surface-2);
  border-bottom: 1px solid var(--gf-line);
}
.md__tab {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 7px 12px;
  border: 1px solid transparent;
  border-bottom: 0;
  border-radius: 9px 9px 0 0;
  background: transparent;
  color: var(--gf-text-2);
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
}
.md__tab:hover {
  color: var(--gf-text);
}
.md__tab_active {
  background: var(--gf-surface);
  border-color: var(--gf-line);
  color: var(--gf-accent);
}
.md__editor {
  display: block;
  width: 100%;
  border: 0;
  outline: 0;
  resize: vertical;
  padding: 14px;
  font-size: 12.5px;
  line-height: 1.55;
  color: var(--gf-text);
  background: var(--gf-surface);
}
.md__preview {
  padding: 14px 16px;
  font-size: 13.5px;
  line-height: 1.6;
  color: var(--gf-text);
  max-height: 460px;
  overflow: auto;
}
.md__preview :deep(h1) {
  font-size: 19px;
  margin: 0 0 12px;
}
.md__preview :deep(h2) {
  font-size: 15.5px;
  margin: 18px 0 8px;
  padding-bottom: 5px;
  border-bottom: 1px solid var(--gf-line);
}
.md__preview :deep(h3) {
  font-size: 14px;
  margin: 14px 0 6px;
}
.md__preview :deep(p) {
  margin: 0 0 10px;
}
.md__preview :deep(ul),
.md__preview :deep(ol) {
  margin: 0 0 12px;
  padding-left: 22px;
}
.md__preview :deep(li) {
  margin: 3px 0;
}
.md__preview :deep(code) {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  padding: 1px 5px;
  border-radius: 6px;
  background: var(--gf-surface-3);
  color: var(--gf-accent);
}
.md__preview :deep(pre) {
  margin: 0 0 12px;
  padding: 12px 14px;
  border-radius: 10px;
  background: var(--gf-surface-2);
  border: 1px solid var(--gf-line);
  overflow: auto;
}
.md__preview :deep(pre code) {
  background: transparent;
  color: var(--gf-text);
  padding: 0;
}
.md__preview :deep(a) {
  color: var(--gf-accent);
}
.md__preview :deep(hr) {
  border: 0;
  border-top: 1px solid var(--gf-line);
  margin: 14px 0;
}
</style>
