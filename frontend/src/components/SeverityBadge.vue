<script setup>
const props = defineProps({
  severity: { type: String, default: '' },
  category: { type: String, default: '' },
})

const sevLabels = { high: 'High', medium: 'Medium', low: 'Low' }
const catLabels = {
  code_duplication: 'Duplication',
  security: 'Security',
  maintainability: 'Maintainability',
  performance: 'Performance',
  architecture: 'Architecture',
}
</script>

<template>
  <span v-if="severity" class="badge" :class="`badge_sev_${severity}`">
    {{ sevLabels[severity] || severity }}
  </span>
  <span v-if="category" class="badge badge_cat">
    {{ catLabels[category] || category }}
  </span>
</template>

<style scoped>
.badge {
  display: inline-flex;
  align-items: center;
  height: 22px;
  padding: 0 9px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.01em;
}
.badge_sev_high {
  color: var(--gf-red);
  background: var(--gf-red-bg);
}
.badge_sev_medium {
  color: var(--gf-amber);
  background: var(--gf-amber-bg);
}
.badge_sev_low {
  color: var(--gf-blue);
  background: var(--gf-blue-bg);
}
.badge_cat {
  color: var(--gf-accent);
  background: var(--gf-purple-soft);
}
</style>
