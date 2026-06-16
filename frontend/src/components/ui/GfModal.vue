<script setup>
import { onMounted, onBeforeUnmount } from 'vue'
import GfIcon from './GfIcon.vue'

const props = defineProps({
  title: { type: String, default: '' },
  subtitle: { type: String, default: '' },
  wide: { type: Boolean, default: false },
})
const emit = defineEmits(['close'])

function onKey(e) {
  if (e.key === 'Escape') emit('close')
}
onMounted(() => {
  document.addEventListener('keydown', onKey)
  document.body.style.overflow = 'hidden'
})
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKey)
  document.body.style.overflow = ''
})
</script>

<template>
  <Teleport to="body">
    <div class="gf-modal-backdrop" @mousedown.self="emit('close')">
      <div
        class="gf-modal"
        :class="{ 'gf-modal_wide': wide }"
        role="dialog"
        aria-modal="true"
      >
        <header class="gf-modal__head">
          <div class="gf-modal__titles">
            <h2 class="gf-modal__title">{{ title }}</h2>
            <p v-if="subtitle" class="gf-modal__subtitle">{{ subtitle }}</p>
          </div>
          <button class="gf-modal__close" aria-label="Close" @click="emit('close')">
            <GfIcon name="close" :size="18" />
          </button>
        </header>
        <div class="gf-modal__body">
          <slot />
        </div>
        <footer v-if="$slots.footer" class="gf-modal__foot">
          <slot name="footer" />
        </footer>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.gf-modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(39, 39, 53, 0.42);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 48px 16px;
  z-index: 1000;
  overflow-y: auto;
  animation: gf-fade 0.15s ease;
}
.gf-modal {
  width: 100%;
  max-width: 560px;
  background: var(--gf-surface);
  border: 1px solid var(--gf-line);
  border-radius: var(--gf-radius-lg);
  box-shadow: var(--gf-shadow-pop);
  animation: gf-pop 0.16s ease;
}
.gf-modal_wide {
  max-width: 760px;
}
.gf-modal__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 22px 14px;
  border-bottom: 1px solid var(--gf-line);
}
.gf-modal__title {
  margin: 0;
  font-size: 18px;
  font-weight: 700;
}
.gf-modal__subtitle {
  margin: 4px 0 0;
  font-size: 13px;
  color: var(--gf-text-2);
}
.gf-modal__close {
  flex: none;
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: var(--gf-text-2);
  cursor: pointer;
}
.gf-modal__close:hover {
  background: var(--gf-surface-3);
  color: var(--gf-text);
}
.gf-modal__body {
  padding: 20px 22px;
}
.gf-modal__foot {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 14px 22px;
  border-top: 1px solid var(--gf-line);
  background: var(--gf-surface-2);
  border-radius: 0 0 var(--gf-radius-lg) var(--gf-radius-lg);
}
@keyframes gf-fade {
  from {
    opacity: 0;
  }
}
@keyframes gf-pop {
  from {
    opacity: 0;
    transform: translateY(-6px) scale(0.99);
  }
}
</style>
