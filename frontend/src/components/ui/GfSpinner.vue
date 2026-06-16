<script setup>
defineProps({
  size: { type: [Number, String], default: 22 },
  label: { type: String, default: '' },
})
</script>

<template>
  <div class="gf-spinner-wrap">
    <span
      class="gf-spinner"
      :style="{ width: size + 'px', height: size + 'px' }"
      role="status"
      aria-live="polite"
    />
    <span v-if="label" class="gf-spinner-label">{{ label }}</span>
  </div>
</template>

<style scoped>
.gf-spinner-wrap {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  color: var(--gf-text-2);
}
.gf-spinner {
  display: inline-block;
  border: 2.5px solid var(--gf-line-2);
  border-top-color: var(--gf-purple);
  border-radius: 50%;
  animation: gf-spin 0.7s linear infinite;
}
.gf-spinner-label {
  font-size: 13px;
}
@keyframes gf-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
