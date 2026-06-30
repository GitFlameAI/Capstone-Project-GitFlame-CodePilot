<script setup>
// "i-in-circle" hint. The purple info glyph turns dark purple on hover/focus and
// reveals a short popover explaining what a field controls. Used next to the
// Config and Autogeneration fields so every setting is self-explanatory.
//
//   <GfTooltip text="Branches created for AI changes get this prefix, e.g. ai/" />
import { ref } from 'vue'
import GfIcon from './GfIcon.vue'

defineProps({
  text: { type: String, required: true },
  // 'top' (default) or 'bottom' — flips the popover when a field sits near the top.
  placement: { type: String, default: 'top' },
})

const open = ref(false)
</script>

<template>
  <span
    class="tip"
    @mouseenter="open = true"
    @mouseleave="open = false"
    @focusin="open = true"
    @focusout="open = false"
  >
    <button
      type="button"
      class="tip__btn"
      :class="{ 'tip__btn_open': open }"
      :aria-label="text"
    >
      <GfIcon name="info" :size="14" />
    </button>
    <span
      v-if="open"
      class="tip__pop"
      :class="`tip__pop_${placement}`"
      role="tooltip"
    >{{ text }}</span>
  </span>
</template>

<style scoped>
.tip {
  position: relative;
  display: inline-flex;
  vertical-align: middle;
  margin-left: 5px;
}
.tip__btn {
  display: grid;
  place-items: center;
  width: 18px;
  height: 18px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: transparent;
  color: var(--gf-purple);
  cursor: help;
  transition: color 0.12s ease;
}
.tip__btn:hover,
.tip__btn_open {
  color: var(--gf-accent); /* dark purple on hover */
}
.tip__pop {
  position: absolute;
  left: 50%;
  z-index: 40;
  width: max-content;
  max-width: 250px;
  padding: 8px 11px;
  border-radius: 10px;
  background: var(--gf-accent);
  color: #fff;
  font-size: 12px;
  font-weight: 500;
  line-height: 1.45;
  text-align: left;
  white-space: normal;
  box-shadow: var(--gf-shadow-pop);
  transform: translateX(-50%);
  animation: tip-in 0.12s ease;
  pointer-events: none;
}
.tip__pop_top {
  bottom: calc(100% + 8px);
}
.tip__pop_bottom {
  top: calc(100% + 8px);
}
/* little arrow */
.tip__pop::after {
  content: '';
  position: absolute;
  left: 50%;
  width: 8px;
  height: 8px;
  background: var(--gf-accent);
  transform: translateX(-50%) rotate(45deg);
}
.tip__pop_top::after {
  bottom: -4px;
}
.tip__pop_bottom::after {
  top: -4px;
}
@keyframes tip-in {
  from {
    opacity: 0;
    transform: translateX(-50%) translateY(2px);
  }
}
</style>
