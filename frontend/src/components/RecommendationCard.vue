<script setup>
import GfIcon from './ui/GfIcon.vue'
import SeverityBadge from './SeverityBadge.vue'

const props = defineProps({
  card: { type: Object, required: true },
  busy: { type: Boolean, default: false },
})
const emit = defineEmits(['close', 'delete'])
</script>

<template>
  <article class="rec" :class="{ rec_closed: card.state === 'closed' }">
    <div class="rec__head">
      <div class="rec__badges">
        <SeverityBadge :severity="card.severity" :category="card.category" />
        <span v-if="card.state === 'closed'" class="rec__resolved">
          <GfIcon name="check" :size="13" /> Resolved
        </span>
      </div>
      <span v-if="card.confidence != null" class="rec__conf gf-muted">
        {{ Math.round(card.confidence * 100) }}% confidence
      </span>
    </div>

    <div class="rec__loc mono">
      {{ card.file }}<span v-if="card.line"> : {{ card.line }}</span>
    </div>

    <p class="rec__problem">{{ card.problem }}</p>

    <div class="rec__suggestion">
      <span class="rec__suggestion-label">Suggested fix</span>
      <p>{{ card.suggestion }}</p>
    </div>

    <div class="rec__actions">
      <button
        v-if="card.state !== 'closed'"
        class="rec__act"
        :disabled="busy"
        @click="emit('close', card)"
      >
        <GfIcon name="check" :size="15" /> Mark resolved
      </button>
      <button class="rec__act rec__act_danger" :disabled="busy" @click="emit('delete', card)">
        <GfIcon name="trash" :size="15" /> Dismiss
      </button>
    </div>
  </article>
</template>

<style scoped>
.rec {
  border: 1px solid var(--gf-line);
  border-radius: var(--gf-radius);
  background: var(--gf-surface);
  padding: 16px;
  transition: box-shadow 0.15s ease, opacity 0.15s ease;
}
.rec:hover {
  box-shadow: var(--gf-shadow-sm);
}
.rec_closed {
  opacity: 0.6;
}
.rec__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 10px;
}
.rec__badges {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.rec__resolved {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  font-weight: 600;
  color: var(--gf-green);
}
.rec__conf {
  font-size: 12px;
  white-space: nowrap;
}
.rec__loc {
  font-size: 12.5px;
  color: var(--gf-accent);
  margin-bottom: 8px;
  word-break: break-all;
}
.rec__problem {
  margin: 0 0 12px;
  font-size: 14px;
  line-height: 1.5;
}
.rec__suggestion {
  background: var(--gf-purple-soft);
  border: 1px solid var(--gf-line-2);
  border-radius: 10px;
  padding: 10px 12px;
  margin-bottom: 12px;
}
.rec__suggestion-label {
  display: block;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--gf-accent);
  margin-bottom: 4px;
}
.rec__suggestion p {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: var(--gf-text);
}
.rec__actions {
  display: flex;
  gap: 8px;
}
.rec__act {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 30px;
  padding: 0 12px;
  border: 1px solid var(--gf-line-2);
  border-radius: 10px;
  background: var(--gf-surface);
  color: var(--gf-text-2);
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}
.rec__act:hover:not(:disabled) {
  border-color: var(--gf-purple);
  color: var(--gf-purple-active);
}
.rec__act:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.rec__act_danger:hover:not(:disabled) {
  border-color: var(--gf-red);
  color: var(--gf-red);
}
</style>
