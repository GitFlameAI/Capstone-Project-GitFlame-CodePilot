<script setup>
defineProps({
  variant: { type: String, default: 'secondary' }, // primary | secondary | ghost | danger
  size: { type: String, default: 'm' }, // s | m | l
  loading: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
})
</script>

<template>
  <button
    class="gf-btn"
    :class="[`gf-btn_${variant}`, `gf-btn_${size}`, { 'gf-btn_loading': loading }]"
    :disabled="disabled || loading"
    type="button"
  >
    <span v-if="loading" class="gf-btn__spinner" aria-hidden="true" />
    <span class="gf-btn__content" :class="{ 'gf-btn__content_hidden': loading }">
      <slot />
    </span>
  </button>
</template>

<style scoped>
.gf-btn {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  border: 1px solid transparent;
  border-radius: var(--gf-radius);
  font-family: inherit;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}
.gf-btn:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}
.gf-btn_s {
  height: 28px;
  padding: 0 12px;
  font-size: 12px;
  border-radius: 10px;
}
.gf-btn_m {
  height: 34px;
  padding: 0 15px;
  font-size: 13px;
}
.gf-btn_l {
  height: 40px;
  padding: 0 22px;
  font-size: 14px;
  font-weight: 700;
  border-radius: var(--gf-radius-lg);
}

/* Primary — the signature GitFlame purple */
.gf-btn_primary {
  background: var(--gf-purple);
  color: #fff;
}
.gf-btn_primary:hover:not(:disabled) {
  background: var(--gf-purple-hover);
}
.gf-btn_primary:active:not(:disabled) {
  background: var(--gf-purple-active);
}

/* Secondary — white with purple text/border */
.gf-btn_secondary {
  background: var(--gf-surface);
  border-color: var(--gf-line-2);
  color: var(--gf-text);
}
.gf-btn_secondary:hover:not(:disabled) {
  border-color: var(--gf-purple);
  color: var(--gf-purple-active);
}

/* Ghost — transparent */
.gf-btn_ghost {
  background: transparent;
  color: var(--gf-text-2);
}
.gf-btn_ghost:hover:not(:disabled) {
  background: var(--gf-surface-3);
  color: var(--gf-text);
}

/* Danger */
.gf-btn_danger {
  background: var(--gf-surface);
  border-color: var(--gf-red);
  color: var(--gf-red);
}
.gf-btn_danger:hover:not(:disabled) {
  background: var(--gf-red-bg);
}

.gf-btn__content {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.gf-btn__content_hidden {
  visibility: hidden;
}
.gf-btn__spinner {
  position: absolute;
  width: 15px;
  height: 15px;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: gf-btn-spin 0.7s linear infinite;
}
@keyframes gf-btn-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
