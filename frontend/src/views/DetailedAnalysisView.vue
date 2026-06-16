<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api, ApiError, USING_MOCK } from '../api/index.js'
import { demoRepo } from '../data/demo.js'
import GfIcon from '../components/ui/GfIcon.vue'
import GfButton from '../components/ui/GfButton.vue'
import GfSpinner from '../components/ui/GfSpinner.vue'
import RecommendationCard from '../components/RecommendationCard.vue'

const router = useRouter()

const state = ref('loading') // loading | empty | ready | error
const errorMessage = ref('')
const summary = ref('')
const status = ref(null)
const cards = ref([])
const busyId = ref(null)

// Filters: severity + whether to show already-resolved cards.
const severityFilter = ref('all') // all | high | medium | low
const showResolved = ref(false)

const severityOptions = [
  { id: 'all', label: 'All' },
  { id: 'high', label: 'High' },
  { id: 'medium', label: 'Medium' },
  { id: 'low', label: 'Low' },
]

const visibleCards = computed(() =>
  cards.value.filter((c) => {
    if (!showResolved.value && c.state === 'closed') return false
    if (severityFilter.value !== 'all' && c.severity !== severityFilter.value) return false
    return true
  })
)

const severityCounts = computed(() => {
  const open = cards.value.filter((c) => c.state !== 'closed')
  return {
    high: open.filter((c) => c.severity === 'high').length,
    medium: open.filter((c) => c.severity === 'medium').length,
    low: open.filter((c) => c.severity === 'low').length,
  }
})

async function load() {
  state.value = 'loading'
  errorMessage.value = ''
  try {
    const [statusRes, summaryRes, listRes] = await Promise.all([
      api.getRecommendationStatus(demoRepo.id),
      api.getRecommendationSummary(demoRepo.id),
      api.listRecommendations(demoRepo.id),
    ])
    status.value = statusRes
    summary.value = summaryRes.summary
    cards.value = listRes.recommendations
    state.value = cards.value.length ? 'ready' : 'empty'
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) {
      state.value = 'empty'
    } else {
      errorMessage.value = e.message || 'Failed to load analysis.'
      state.value = 'error'
    }
  }
}

async function onClose(card) {
  busyId.value = card.id
  try {
    const updated = await api.closeRecommendation(card.id)
    const idx = cards.value.findIndex((c) => c.id === card.id)
    if (idx !== -1) cards.value[idx] = updated
    await refreshStatus()
  } catch (e) {
    errorMessage.value = e.message
  } finally {
    busyId.value = null
  }
}

async function onDelete(card) {
  busyId.value = card.id
  try {
    await api.deleteRecommendation(card.id)
    cards.value = cards.value.filter((c) => c.id !== card.id)
    await refreshStatus()
  } catch (e) {
    errorMessage.value = e.message
  } finally {
    busyId.value = null
  }
}

async function refreshStatus() {
  try {
    status.value = await api.getRecommendationStatus(demoRepo.id)
  } catch {
    /* non-critical */
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page__inner">
      <!-- Header -->
      <header class="head">
        <button class="back" @click="router.push('/')">
          <GfIcon name="chevronRight" :size="16" class="back__icon" />
          Back to repository
        </button>
        <div class="head__title">
          <span class="head__mark"><GfIcon name="sparkles" :size="18" /></span>
          <div>
            <h1>Detailed AI analysis</h1>
            <p class="head__sub gf-muted">
              {{ demoRepo.owner }}/{{ demoRepo.name }} · code optimization recommendations
            </p>
          </div>
        </div>
      </header>

      <!-- Loading -->
      <div v-if="state === 'loading'" class="center gf-card">
        <GfSpinner label="Loading detailed analysis…" />
      </div>

      <!-- Empty -->
      <div v-else-if="state === 'empty'" class="center gf-card">
        <GfIcon name="sparkles" :size="28" />
        <p>No recommendations are stored for this repository yet.</p>
        <GfButton variant="primary" @click="router.push('/')">
          Go back and run an analysis
        </GfButton>
      </div>

      <!-- Error -->
      <div v-else-if="state === 'error'" class="center gf-card center_error">
        <GfIcon name="alert" :size="24" />
        <p>{{ errorMessage }}</p>
        <GfButton variant="secondary" size="s" @click="load">Try again</GfButton>
      </div>

      <!-- Ready -->
      <template v-else>
        <!-- Summary + counters -->
        <section class="summary gf-card">
          <p class="summary__text">{{ summary }}</p>
          <div class="summary__stats">
            <div class="stat">
              <span class="stat__num">{{ status?.total ?? cards.length }}</span>
              <span class="stat__label">total</span>
            </div>
            <div class="stat stat_open">
              <span class="stat__num">{{ status?.open ?? 0 }}</span>
              <span class="stat__label">open</span>
            </div>
            <div class="stat stat_closed">
              <span class="stat__num">{{ status?.closed ?? 0 }}</span>
              <span class="stat__label">resolved</span>
            </div>
          </div>
        </section>

        <!-- Filter bar -->
        <div class="filters">
          <div class="filters__sev">
            <button
              v-for="opt in severityOptions"
              :key="opt.id"
              class="chip"
              :class="{ chip_active: severityFilter === opt.id }"
              @click="severityFilter = opt.id"
            >
              {{ opt.label }}
              <span
                v-if="opt.id !== 'all'"
                class="chip__count"
              >{{ severityCounts[opt.id] }}</span>
            </button>
          </div>
          <label class="toggle">
            <input type="checkbox" v-model="showResolved" />
            Show resolved
          </label>
        </div>

        <!-- Cards -->
        <div v-if="visibleCards.length" class="list">
          <RecommendationCard
            v-for="c in visibleCards"
            :key="c.id"
            :card="c"
            :busy="busyId === c.id"
            @close="onClose"
            @delete="onDelete"
          />
        </div>
        <div v-else class="center gf-card">
          <GfIcon name="check" :size="22" />
          <p>No recommendations match the current filters.</p>
        </div>

        <p v-if="USING_MOCK" class="mockflag">
          Demo data (mock mode) · set <code>VITE_API_BASE</code> to use the live backend
        </p>
      </template>
    </div>
  </div>
</template>

<style scoped>
.page {
  min-height: 100vh;
}
.page__inner {
  max-width: 920px;
  margin: 0 auto;
  padding: 28px 24px 72px;
}
.head {
  margin-bottom: 22px;
}
.back {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 16px;
  border: 0;
  background: transparent;
  color: var(--gf-accent);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.back__icon {
  transform: rotate(180deg);
}
.head__title {
  display: flex;
  align-items: center;
  gap: 14px;
}
.head__mark {
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  border-radius: 12px;
  color: #fff;
  background: linear-gradient(135deg, var(--gf-purple), var(--gf-accent));
}
.head__title h1 {
  margin: 0;
  font-size: 21px;
  font-weight: 700;
}
.head__sub {
  margin: 3px 0 0;
  font-size: 13px;
}

.center {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 40px 20px;
  text-align: center;
  color: var(--gf-text-2);
}
.center :deep(.gf-icon) {
  color: var(--gf-purple);
}
.center_error :deep(.gf-icon) {
  color: var(--gf-red);
}

.summary {
  padding: 20px;
  margin-bottom: 18px;
}
.summary__text {
  margin: 0 0 18px;
  font-size: 14px;
  line-height: 1.6;
  color: var(--gf-text);
}
.summary__stats {
  display: flex;
  gap: 12px;
}
.stat {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 12px;
  border-radius: 12px;
  background: var(--gf-surface-2);
  border: 1px solid var(--gf-line);
}
.stat__num {
  font-size: 22px;
  font-weight: 700;
}
.stat__label {
  font-size: 12px;
  color: var(--gf-text-3);
}
.stat_open .stat__num {
  color: var(--gf-accent);
}
.stat_closed .stat__num {
  color: var(--gf-green);
}

.filters {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.filters__sev {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 14px;
  border: 1px solid var(--gf-line-2);
  border-radius: 999px;
  background: var(--gf-surface);
  color: var(--gf-text-2);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.chip:hover {
  border-color: var(--gf-purple);
}
.chip_active {
  border-color: var(--gf-purple);
  background: var(--gf-purple-soft);
  color: var(--gf-accent);
}
.chip__count {
  display: inline-grid;
  place-items: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 999px;
  background: var(--gf-surface-3);
  color: var(--gf-text-2);
  font-size: 11px;
}
.chip_active .chip__count {
  background: #fff;
}
.toggle {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 13px;
  color: var(--gf-text-2);
  cursor: pointer;
}

.list {
  display: grid;
  gap: 12px;
}
.mockflag {
  margin: 20px 0 0;
  font-size: 11px;
  color: var(--gf-text-3);
  text-align: center;
}
.mockflag code {
  color: var(--gf-accent);
}
</style>
