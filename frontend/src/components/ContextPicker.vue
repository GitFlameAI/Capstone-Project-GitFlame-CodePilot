<script setup>
// Autocomplete multi-select for repository context file paths. Replaces the old
// "one path per line" textarea: the user types, a filtered dropdown of known
// repository files appears, and picking one adds a chip. Enter also adds a custom
// path (so paths not in the suggestion list are still possible). v-model is the
// array of selected paths.
import { ref, computed } from 'vue'
import GfIcon from './ui/GfIcon.vue'

const props = defineProps({
  modelValue: { type: Array, default: () => [] },
  options: { type: Array, default: () => [] },
  placeholder: { type: String, default: 'Type to search repository files…' },
})
const emit = defineEmits(['update:modelValue'])

const query = ref('')
const open = ref(false)
const activeIndex = ref(0)

const suggestions = computed(() => {
  const q = query.value.trim().toLowerCase()
  const selected = new Set(props.modelValue)
  return props.options
    .filter((p) => !selected.has(p))
    .filter((p) => !q || p.toLowerCase().includes(q))
    .slice(0, 8)
})

function add(path) {
  const value = (path || '').trim()
  if (!value || props.modelValue.includes(value)) return
  emit('update:modelValue', [...props.modelValue, value])
  query.value = ''
  activeIndex.value = 0
}
function remove(path) {
  emit('update:modelValue', props.modelValue.filter((p) => p !== path))
}
function onEnter() {
  if (suggestions.value.length && activeIndex.value < suggestions.value.length) {
    add(suggestions.value[activeIndex.value])
  } else if (query.value.trim()) {
    add(query.value)
  }
}
function move(delta) {
  const n = suggestions.value.length
  if (!n) return
  open.value = true
  activeIndex.value = (activeIndex.value + delta + n) % n
}
function onBlur() {
  // Delay so a click on a suggestion registers before the list closes.
  setTimeout(() => (open.value = false), 120)
}
</script>

<template>
  <div class="picker">
    <div class="picker__box" :class="{ 'picker__box_open': open }">
      <span v-for="path in modelValue" :key="path" class="picker__chip">
        <GfIcon name="file" :size="12" />
        <span class="picker__chip-text mono">{{ path }}</span>
        <button class="picker__chip-x" :aria-label="`Remove ${path}`" @click="remove(path)">
          <GfIcon name="close" :size="12" />
        </button>
      </span>
      <input
        v-model="query"
        class="picker__input"
        :placeholder="modelValue.length ? '' : placeholder"
        @focus="open = true"
        @blur="onBlur"
        @keydown.enter.prevent="onEnter"
        @keydown.down.prevent="move(1)"
        @keydown.up.prevent="move(-1)"
        @keydown.delete="query === '' && modelValue.length && remove(modelValue[modelValue.length - 1])"
      />
    </div>

    <ul v-if="open && suggestions.length" class="picker__menu">
      <li
        v-for="(path, i) in suggestions"
        :key="path"
        class="picker__opt"
        :class="{ picker__opt_active: i === activeIndex }"
        @mousedown.prevent="add(path)"
        @mouseenter="activeIndex = i"
      >
        <GfIcon name="file" :size="14" />
        <span class="mono">{{ path }}</span>
      </li>
    </ul>
    <p v-else-if="open && query.trim()" class="picker__hint">
      Press <kbd>Enter</kbd> to add “<span class="mono">{{ query.trim() }}</span>”
    </p>
  </div>
</template>

<style scoped>
.picker {
  position: relative;
  min-width: 0;
}
.picker__box {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-width: 0;
  min-height: 38px;
  padding: 6px 8px;
  border: 1px solid var(--gf-line-2);
  border-radius: 10px;
  background: var(--gf-surface);
}
.picker__box_open {
  border-color: var(--gf-purple);
}
.picker__chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  max-width: 100%;
  min-width: 0;
  height: 24px;
  padding: 0 3px 0 8px;
  border-radius: 999px;
  background: var(--gf-purple-soft);
  color: var(--gf-accent);
  font-size: 11.5px;
}
.picker__chip-text {
  font-size: 11px;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.picker__chip :deep(.gf-icon) {
  flex: none;
}
.picker__chip-x {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: transparent;
  color: var(--gf-accent);
  cursor: pointer;
  line-height: 0;
  flex: none;
}
.picker__chip-x :deep(.gf-icon) {
  vertical-align: middle;
}
.picker__chip-x:hover {
  background: rgba(120, 34, 249, 0.12);
}
.picker__input {
  flex: 1;
  min-width: min(140px, 100%);
  border: 0;
  outline: 0;
  background: transparent;
  font: inherit;
  font-size: 13px;
  color: var(--gf-text);
}
.picker__menu {
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
  max-height: 240px;
  overflow: auto;
}
.picker__opt {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  padding: 8px 10px;
  border-radius: 9px;
  font-size: 12.5px;
  color: var(--gf-text-2);
  cursor: pointer;
}
.picker__opt .mono {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.picker__opt_active {
  background: var(--gf-purple-soft);
  color: var(--gf-accent);
}
.picker__hint {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--gf-text-3);
  word-break: break-word;
  overflow-wrap: anywhere;
}
.picker__hint kbd {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  padding: 1px 5px;
  border-radius: 5px;
  border: 1px solid var(--gf-line-2);
  background: var(--gf-surface-2);
}
</style>
