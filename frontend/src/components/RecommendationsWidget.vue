<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api, ApiError, USING_MOCK } from '../api/index.js'
import { demoRepo, defaultYaml } from '../data/demo.js'
import GfIcon from './ui/GfIcon.vue'
import GfButton from './ui/GfButton.vue'
import GfSpinner from './ui/GfSpinner.vue'
import RecommendationCard from './RecommendationCard.vue'

const router = useRouter()

const state = ref('loading') // loading | empty | ready | error
const errorMessage = ref('')
const summary = ref('')
const status = ref(null)
const cards = ref([])
const analyzing = ref(false)
const busyId = ref(null)

const PREVIEW_COUNT = 3
const openCards = computed(() => cards.value.filter((c) => c.state !== 'closed'))
const previewCards = computed(() => openCards.value.slice(0, PREVIEW_COUNT))
const moreCount = computed(() => Math.max(0, openCards.value.length - PREVIEW_COUNT))

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
    state.value = 'ready'
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) {
      state.value = 'empty' // no analysis has been run for this repo yet
    } else {
      errorMessage.value = e.message || 'Failed to load recommendations.'
      state.value = 'error'
    }
  }
}

async function runAnalysis() {
  analyzing.value = true
  errorMessage.value = ''
  try {
    await api.analyzeRepository(demoRepo.id, {
      repository: {
        id: demoRepo.id,
        name: demoRepo.name,
        default_branch: demoRepo.defaultBranch,
        web_url: demoRepo.webUrl,
      },
      yaml_config: defaultYaml,
      repository_context: demoRepo.branches,
    })
    await load()
  } catch (e) {
    errorMessage.value = e.message || 'Analysis failed.'
    state.value = 'error'
  } finally {
    analyzing.value = false
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
  <section class="widget gf-card" aria-label="AI recommendations">
    <header class="widget__head">
      <div class="widget__title">
        <span class="widget__mark"><GfIcon name="sparkles" :size="16" /></span>
        <div>
          <h3>AI Recommendations</h3>
          <p class="widget__sub gf-muted">Repository analysis for code optimization</p>
        </div>
      </div>
      <div class="widget__head-actions">
        <span v-if="status && state === 'ready'" class="widget__counts">
          <span class="count count_open">{{ status.open }} open</span>
          <span class="count">{{ status.closed }} resolved</span>
        </span>
        <GfButton
          v-if="state === 'ready'"
          variant="secondary"
          size="s"
          :loading="analyzing"
          @click="runAnalysis"
        >
          <GfIcon name="refresh" :size="14" /> Re-run
        </GfButton>
      </div>
    </header>

    <div class="widget__body">
      <!-- Loading -->
      <div v-if="state === 'loading'" class="widget__center">
        <GfSpinner label="Loading recommendations…" />
      </div>

      <!-- Empty: no report yet (real backend) -->
      <div v-else-if="state === 'empty'" class="widget__center widget__empty">
        <GfIcon name="sparkles" :size="26" />
        <p>No analysis has been run for this repository yet.</p>
        <GfButton variant="primary" :loading="analyzing" @click="runAnalysis">
          Run analysis
        </GfButton>
      </div>

      <!-- Error -->
      <div v-else-if="state === 'error'" class="widget__center widget__error">
        <GfIcon name="alert" :size="22" />
        <p>{{ errorMessage }}</p>
        <GfButton variant="secondary" size="s" @click="load">Try again</GfButton>
      </div>

      <!-- Ready -->
      <template v-else>
        <p class="widget__summary">{{ summary }}</p>

        <div v-if="openCards.length" class="widget__cards">
          <RecommendationCard
            v-for="c in previewCards"
            :key="c.id"
            :card="c"
            :busy="busyId === c.id"
            @close="onClose"
            @delete="onDelete"
          />
        </div>
        <div v-else class="widget__allclear">
          <GfIcon name="check" :size="20" /> All recommendations have been handled.
        </div>

        <div class="widget__foot">
          <button class="widget__link" @click="router.push('/recommendations')">
            View detailed analysis
            <span v-if="moreCount"> (+{{ moreCount }} more)</span>
            <GfIcon name="chevronRight" :size="15" />
          </button>
        </div>
      </template>
    </div>

    <p v-if="USING_MOCK" class="widget__mockflag">
      Demo data (mock mode) · set <code>VITE_API_BASE</code> to use the live backend
    </p>
  </section>
</template>

<style scoped>
.widget {
  margin-top: 22px;
  overflow: hidden;
}
.widget__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 18px;
  border-bottom: 1px solid var(--gf-line);
  background: linear-gradient(180deg, var(--gf-purple-soft), var(--gf-surface));
}
.widget__title {
  display: flex;
  align-items: center;
  gap: 12px;
}
.widget__mark {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 10px;
  color: #fff;
  background: linear-gradient(135deg, var(--gf-purple), var(--gf-accent));
}
.widget__title h3 {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
}
.widget__sub {
  margin: 2px 0 0;
  font-size: 12px;
}
.widget__head-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}
.widget__counts {
  display: inline-flex;
  gap: 8px;
}
.count {
  font-size: 12px;
  font-weight: 600;
  color: var(--gf-text-2);
}
.count_open {
  color: var(--gf-accent);
}
.widget__body {
  padding: 18px;
}
.widget__center {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 28px 10px;
  text-align: center;
}
.widget__empty,
.widget__error {
  color: var(--gf-text-2);
}
.widget__empty :deep(.gf-icon),
.widget__error :deep(.gf-icon) {
  color: var(--gf-purple);
}
.widget__error :deep(.gf-icon) {
  color: var(--gf-red);
}
.widget__summary {
  margin: 0 0 16px;
  font-size: 14px;
  line-height: 1.55;
  color: var(--gf-text);
}
.widget__cards {
  display: grid;
  gap: 12px;
}
.widget__allclear {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 18px;
  border-radius: 12px;
  background: var(--gf-green-bg);
  color: var(--gf-green);
  font-size: 14px;
  font-weight: 600;
}
.widget__foot {
  margin-top: 16px;
  display: flex;
  justify-content: center;
}
.widget__link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 0;
  background: transparent;
  color: var(--gf-accent);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.widget__link:hover {
  text-decoration: underline;
}
.widget__mockflag {
  margin: 0;
  padding: 8px 18px;
  border-top: 1px dashed var(--gf-line-2);
  background: var(--gf-surface-2);
  font-size: 11px;
  color: var(--gf-text-3);
}
.widget__mockflag code {
  font-size: 11px;
  color: var(--gf-accent);
}
</style>
