<script setup>
// Small reusable multi-select dropdown. The button shows a label and a
// selected/total count; the menu lists checkbox options with All / None shortcuts.
// v-model is an array of selected option ids. Options may carry an optional color
// used to render a small dot (e.g. category / severity colours).
import { ref } from 'vue'
import GfIcon from './ui/GfIcon.vue'

const props = defineProps({
  label: { type: String, required: true },
  options: { type: Array, default: () => [] }, // [{ id, label, color? }]
  modelValue: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:modelValue'])

const open = ref(false)
const isOn = (id) => props.modelValue.includes(id)

function toggle(id) {
  const next = isOn(id) ? props.modelValue.filter((x) => x !== id) : [...props.modelValue, id]
  emit('update:modelValue', next)
}
const all = () => emit('update:modelValue', props.options.map((o) => o.id))
const none = () => emit('update:modelValue', [])
</script>

<template>
  <div class="ms">
    <button class="ms__btn" :class="{ ms__btn_open: open }" @click="open = !open">
      <span class="ms__label">{{ label }}</span>
      <span class="ms__count">{{ modelValue.length }}/{{ options.length }}</span>
      <GfIcon name="chevronDown" :size="14" class="ms__caret" :class="{ ms__caret_open: open }" />
    </button>
    <template v-if="open">
      <div class="ms__back" @click="open = false"></div>
      <div class="ms__menu">
        <div class="ms__quick">
          <button @click="all">All</button>
          <span class="ms__sep">·</span>
          <button @click="none">None</button>
        </div>
        <ul class="ms__list">
          <li v-for="o in options" :key="o.id" class="ms__opt" @click="toggle(o.id)">
            <span class="ms__check" :class="{ ms__check_on: isOn(o.id) }">
              <GfIcon v-if="isOn(o.id)" name="check" :size="12" />
            </span>
            <span v-if="o.color" class="ms__dot" :style="{ background: o.color }"></span>
            <span class="ms__optlabel">{{ o.label }}</span>
          </li>
        </ul>
      </div>
    </template>
  </div>
</template>

<style scoped>
.ms {
  position: relative;
}
.ms__btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 34px;
  padding: 0 12px;
  border: 1px solid var(--gf-line-2);
  border-radius: 9px;
  background: var(--gf-surface);
  color: var(--gf-text);
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
}
.ms__btn:hover,
.ms__btn_open {
  border-color: var(--gf-purple);
}
.ms__count {
  display: inline-grid;
  place-items: center;
  min-width: 32px;
  height: 18px;
  padding: 0 6px;
  border-radius: 999px;
  background: var(--gf-purple-soft);
  color: var(--gf-accent);
  font-size: 11px;
}
.ms__caret {
  color: var(--gf-text-3);
  transition: transform 0.12s ease;
}
.ms__caret_open {
  transform: rotate(180deg);
}
.ms__back {
  position: fixed;
  inset: 0;
  z-index: 40;
}
.ms__menu {
  position: absolute;
  z-index: 41;
  top: calc(100% + 6px);
  left: 0;
  min-width: 200px;
  padding: 6px;
  border: 1px solid var(--gf-line);
  border-radius: 12px;
  background: var(--gf-surface);
  box-shadow: var(--gf-shadow-pop);
}
.ms__quick {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px 8px;
  border-bottom: 1px solid var(--gf-line);
  margin-bottom: 5px;
}
.ms__quick button {
  border: 0;
  background: transparent;
  font: inherit;
  font-size: 12px;
  font-weight: 700;
  color: var(--gf-accent);
  cursor: pointer;
  padding: 0;
}
.ms__sep {
  color: var(--gf-text-3);
}
.ms__list {
  list-style: none;
  margin: 0;
  padding: 0;
  max-height: 260px;
  overflow: auto;
}
.ms__opt {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 8px 9px;
  border-radius: 8px;
  cursor: pointer;
}
.ms__opt:hover {
  background: var(--gf-surface-2);
}
.ms__check {
  display: grid;
  place-items: center;
  width: 18px;
  height: 18px;
  border: 1.5px solid var(--gf-line-2);
  border-radius: 5px;
  color: #fff;
  flex: none;
}
.ms__check_on {
  background: var(--gf-purple);
  border-color: var(--gf-purple);
}
.ms__dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  flex: none;
}
.ms__optlabel {
  font-size: 13px;
  color: var(--gf-text);
  text-transform: capitalize;
}
</style>
