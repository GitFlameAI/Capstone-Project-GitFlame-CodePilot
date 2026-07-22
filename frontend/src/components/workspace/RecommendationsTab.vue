<script setup>
// Recommendations tab (Sprint 3 redesign).
//
// Recommendations are shown as a compact grid of small cards (category +
// confidence + severity + a short problem). Clicking a card opens a detail
// overlay (dim background) where the user can page through recommendations with
// ←/→, delete one, or turn it into an issue (handed off to the Autogeneration
// tab via session.pendingIssue). A category toggle bar filters the grid: all
// categories are on by default (everything shows); turning all off shows nothing.
//
// There is intentionally no "resolved" concept here — a recommendation is either
// acted on (turned into an issue) or dismissed (deleted). Severity is kept but
// explained in a small legend and in the detail overlay.
import { ref, computed, onMounted } from 'vue'
import { api, ApiError, USING_MOCK } from '../../api/index.js'
import { describeError } from '../../api/errors.js'
import { session } from '../../store/session.js'
import { RECOMMENDATION_CATEGORIES } from '../../data/demo.js'
import { flattenFiles, computeExcludedSet } from '../../utils/excludePaths.js'
import GfIcon from '../ui/GfIcon.vue'
import GfButton from '../ui/GfButton.vue'
import GfSpinner from '../ui/GfSpinner.vue'
import GfModal from '../ui/GfModal.vue'
import GfTooltip from '../ui/GfTooltip.vue'
import MultiSelect from '../MultiSelect.vue'

const emit = defineEmits(['go'])

const state = ref('loading') // loading | ready | analyzing | error | no_categories | clean | dismissed
const errorMessage = ref('')
const summary = ref('')
const cards = ref([])
const busy = ref(false)
const stale = ref(false)

// --- filters (one row: confidence sort · categories · severity) ---
const sortDir = ref('desc') // 'desc' = highest confidence first, 'asc' = lowest first
const selectedCats = ref([])
const selectedSevs = ref([])
const selectedIndex = ref(-1)

const CATEGORY_COLORS = {
  security: '#d03232',
  performance: '#b07400',
  code_duplication: '#414ee0',
  maintainability: '#0a8f6c',
  architecture: '#7822f9',
}
const SEVERITY_ORDER = ['high', 'medium', 'low']
const SEVERITY_COLORS = { high: '#d03232', medium: '#b07400', low: '#8a87a6' }

const categoryLabel = (id) => RECOMMENDATION_CATEGORIES.find((c) => c.id === id)?.label || id
const confidencePct = (c) => Math.round((c || 0) * 100)

// Configured categories decide whether the system analyses at all (empty => nothing),
// but once cards are stored we show ALL of them regardless of the current config —
// so unselecting a category in Config does not hide already-found cards. Only when
// the stored cards run out does the "no categories" state apply.
const configuredCategories = computed(() => session.configForm.categories || [])

// Every stored card is in scope; filters below narrow the visible set.
const inScopeCards = computed(() => cards.value)

const presentCategories = computed(() => {
  const set = new Set(inScopeCards.value.map((c) => c.category))
  return RECOMMENDATION_CATEGORIES.filter((c) => set.has(c.id))
})
const presentSeverities = computed(() => {
  const set = new Set(inScopeCards.value.map((c) => c.severity))
  return SEVERITY_ORDER.filter((s) => set.has(s))
})

// Options for the category / severity multi-selects.
const categoryOptions = computed(() =>
  presentCategories.value.map((c) => ({ id: c.id, label: c.label, color: CATEGORY_COLORS[c.id] })),
)
const severityOptions = computed(() =>
  presentSeverities.value.map((s) => ({ id: s, label: s, color: SEVERITY_COLORS[s] })),
)

const visibleCards = computed(() => {
  const list = inScopeCards.value.filter(
    (c) => selectedCats.value.includes(c.category) && selectedSevs.value.includes(c.severity),
  )
  const dir = sortDir.value === 'asc' ? 1 : -1
  return [...list].sort((a, b) => ((a.confidence || 0) - (b.confidence || 0)) * dir)
})

const selectedCard = computed(() =>
  selectedIndex.value >= 0 ? visibleCards.value[selectedIndex.value] : null,
)

// Default both filters to "everything present" whenever a report loads.
function ensureFilter() {
  selectedCats.value = presentCategories.value.map((c) => c.id)
  selectedSevs.value = presentSeverities.value.slice()
}
function toggleSort() {
  sortDir.value = sortDir.value === 'desc' ? 'asc' : 'desc'
}

async function load({ auto = false } = {}) {
  state.value = 'loading'
  errorMessage.value = ''
  try {
    const [summaryRes, listRes] = await Promise.all([
      api.getRecommendationSummary(session.repo.id),
      api.listRecommendations(session.repo.id),
    ]).catch((e) => {
      // A 404 (no report yet) is an "empty" signal, not an error.
      if (e instanceof ApiError && e.status === 404) return [null, { recommendations: [] }]
      throw e
    })
    cards.value = (listRes && listRes.recommendations) || []
    summary.value = (summaryRes && summaryRes.summary) || ''
    if (cards.value.length) {
      ensureFilter()
      stale.value = session.recommendationsStale
      state.value = 'ready'
      return
    }
    // No stored cards. Auto-run once if categories are configured (the user asked
    // not to have to press "Run analysis"); otherwise explain what to do.
    if (configuredCategories.value.length) {
      if (auto) state.value = 'clean' // already analysed, nothing found — avoid a loop
      else await runAnalysis({ auto: true })
    } else {
      state.value = 'no_categories'
    }
  } catch (e) {
    errorMessage.value = describeError(e).message
    state.value = 'error'
  }
}

// File paths CodePilot may analyse: everything in the known repository tree that
// is not excluded by the configuration. The backend's direct analyze endpoint
// requires at least one path (repository_context); the backend then reads the
// file contents through the GitFlame connection.
function analyzableContext() {
  const all = flattenFiles(session.fileTree)
  const excluded = computeExcludedSet(all, session.configForm.excludePaths || [])
  return all.filter((p) => !excluded.has(p))
}

async function runAnalysis({ auto = false } = {}) {
  if (!configuredCategories.value.length) { state.value = 'no_categories'; return }
  state.value = 'analyzing'
  errorMessage.value = ''
  try {
    await api.analyzeRepository(session.repo.id, {
      repository: {
        id: session.repo.id,
        name: session.repo.name,
        default_branch: session.repo.defaultBranch,
        web_url: session.repo.url,
      },
      yaml_config: session.configYaml,
      categories: configuredCategories.value,
      repository_context: analyzableContext(),
    })
    session.recommendationsStale = false
    await load({ auto })
  } catch (e) {
    errorMessage.value = describeError(e).message
    state.value = 'error'
  }
}

// --- detail overlay ---
function openCard(card) {
  selectedIndex.value = visibleCards.value.findIndex((c) => c.id === card.id)
}
function closeCard() {
  selectedIndex.value = -1
}
function prev() {
  if (!visibleCards.value.length) return
  selectedIndex.value = (selectedIndex.value - 1 + visibleCards.value.length) % visibleCards.value.length
}
function next() {
  if (!visibleCards.value.length) return
  selectedIndex.value = (selectedIndex.value + 1) % visibleCards.value.length
}

async function deleteSelected() {
  const card = selectedCard.value
  if (!card) return
  busy.value = true
  try {
    await api.deleteRecommendation(card.id)
    cards.value = cards.value.filter((c) => c.id !== card.id)
    if (!cards.value.length) {
      // The user dismissed the last card. Don't immediately re-run — the code and
      // config haven't changed, so a fresh run would surface the same findings.
      // The analysis re-runs automatically the next time this tab is opened.
      closeCard()
      summary.value = ''
      state.value = 'dismissed'
      return
    }
    if (!visibleCards.value.length) closeCard()
    else selectedIndex.value = Math.min(selectedIndex.value, visibleCards.value.length - 1)
  } catch (e) {
    errorMessage.value = e.message
  } finally {
    busy.value = false
  }
}

function basename(path) {
  return String(path || '').split('/').pop()
}

// Turn the selected recommendation into an issue and jump to Autogeneration.
function createIssue() {
  const card = selectedCard.value
  if (!card) return
  const title = `Fix ${categoryLabel(card.category).toLowerCase()} issue in ${basename(card.file)}`
  const body =
    `${card.problem}\n\n` +
    `Suggested fix:\n${card.suggestion}\n\n` +
    `Location: ${card.file}${card.line ? ':' + card.line : ''}\n\n` +
    `(Created from a CodePilot recommendation.)`
  session.pendingIssue = { title, body, author: session.repo.owner || 'roma' }
  closeCard()
  emit('go', 'autogen')
}

onMounted(load)
</script>

<template>
  <div class="rec">
    <!-- Loading -->
    <div v-if="state === 'loading'" class="center gf-card"><GfSpinner label="Loading recommendations…" /></div>

    <!-- Analyzing -->
    <div v-else-if="state === 'analyzing'" class="center gf-card">
      <GfSpinner :size="26" label="Analysing repository…" />
      <p class="gf-muted">CodePilot is reviewing the repository against your configuration.</p>
    </div>

    <!-- No categories configured -->
    <div v-else-if="state === 'no_categories'" class="center gf-card">
      <GfIcon name="info" :size="26" />
      <p>No recommendation categories are enabled in the configuration, so nothing is analysed.</p>
      <GfButton variant="primary" size="s" @click="emit('go', 'config')">Enable categories in Config</GfButton>
    </div>

    <!-- Analysed, nothing found -->
    <div v-else-if="state === 'clean'" class="center gf-card">
      <GfIcon name="check" :size="28" />
      <p>Analysis complete — no issues were found for your selected categories. 🎉</p>
      <GfButton variant="secondary" @click="runAnalysis({ auto: true })"><GfIcon name="refresh" :size="15" /> Re-run</GfButton>
    </div>

    <!-- All cards dismissed — re-run on demand (or automatically next visit) -->
    <div v-else-if="state === 'dismissed'" class="center gf-card">
      <GfIcon name="check" :size="28" />
      <p>All recommendations dismissed. The analysis will run again next time you open this tab, or re-run it now.</p>
      <GfButton variant="secondary" @click="runAnalysis({ auto: true })"><GfIcon name="refresh" :size="15" /> Re-run analysis</GfButton>
    </div>

    <!-- Error -->
    <div v-else-if="state === 'error'" class="center gf-card center_error">
      <GfIcon name="alert" :size="24" />
      <p>{{ errorMessage }}</p>
      <GfButton variant="secondary" size="s" @click="load">Try again</GfButton>
    </div>

    <!-- Ready -->
    <template v-else>
      <div v-if="stale" class="stalebar">
        <GfIcon name="alert" :size="15" />
        <span>The repository changed since this analysis. Use <strong>Re-run</strong> below to refresh the recommendations.</span>
      </div>
      <section class="summary gf-card">
        <p class="summary__text"><span class="summary__kw">Summary: </span>{{ summary }}</p>
        <GfButton variant="secondary" size="s" @click="runAnalysis({ auto: true })"><GfIcon name="refresh" :size="14" /> Re-run</GfButton>
      </section>

      <div class="bar">
        <button class="sortbtn" :title="sortDir === 'desc' ? 'Confidence: high to low' : 'Confidence: low to high'" @click="toggleSort">
          Confidence
          <GfIcon name="chevronDown" :size="14" class="sortbtn__arrow" :class="{ sortbtn__arrow_up: sortDir === 'asc' }" />
        </button>
        <MultiSelect v-model="selectedCats" label="Categories" :options="categoryOptions" />
        <MultiSelect v-model="selectedSevs" label="Severity" :options="severityOptions" />
        <GfTooltip text="Severity reflects estimated impact if left unaddressed: High = likely security/correctness/scaling risk; Medium = maintainability or moderate performance cost; Low = style or minor cleanup. Confidence is how sure the model is about the finding." />
      </div>

      <div v-if="visibleCards.length" class="grid">
        <button
          v-for="c in visibleCards"
          :key="c.id"
          class="mini gf-card"
          @click="openCard(c)"
        >
          <div class="mini__top">
            <span class="catbadge" :class="`catbg_${c.category}`">{{ categoryLabel(c.category) }}</span>
            <span class="sev" :class="`sev_${c.severity}`">{{ c.severity }}</span>
          </div>
          <p class="mini__problem">{{ c.problem }}</p>
          <div class="mini__foot">
            <span class="mini__file mono">{{ basename(c.file) }}</span>
            <span class="conf"><span class="conf__bar"><span class="conf__fill" :style="{ width: confidencePct(c.confidence) + '%' }"></span></span>{{ confidencePct(c.confidence) }}%</span>
          </div>
        </button>
      </div>
      <div v-else class="center gf-card">
        <GfIcon name="eye" :size="22" />
        <p>No recommendations match the current filters.</p>
        <GfButton variant="secondary" size="s" @click="ensureFilter">Reset filters</GfButton>
      </div>

      <p v-if="USING_MOCK" class="mockflag">Demo data (mock mode) · set <code>VITE_API_BASE</code> to use the live backend</p>
    </template>

    <!-- Detail overlay -->
    <GfModal v-if="selectedCard" :title="categoryLabel(selectedCard.category)" :subtitle="`${selectedIndex + 1} of ${visibleCards.length}`" wide @close="closeCard">
      <div class="detail">
        <div class="detail__badges">
          <span class="sev" :class="`sev_${selectedCard.severity}`">{{ selectedCard.severity }} severity</span>
          <span class="catbadge" :class="`catbg_${selectedCard.category}`">{{ categoryLabel(selectedCard.category) }}</span>
          <span class="conf conf_lg"><span class="conf__bar"><span class="conf__fill" :style="{ width: confidencePct(selectedCard.confidence) + '%' }"></span></span>{{ confidencePct(selectedCard.confidence) }}% confidence</span>
        </div>
        <p class="detail__loc mono"><GfIcon name="file" :size="14" /> {{ selectedCard.file }}<span v-if="selectedCard.line">:{{ selectedCard.line }}</span></p>

        <h4 class="detail__h">Problem</h4>
        <p class="detail__p">{{ selectedCard.problem }}</p>
        <h4 class="detail__h">Suggested fix</h4>
        <p class="detail__p">{{ selectedCard.suggestion }}</p>

        <div class="detail__nav">
          <button class="navbtn" :disabled="visibleCards.length < 2" @click="prev"><GfIcon name="chevronRight" :size="16" class="navbtn__l" /> Prev</button>
          <button class="navbtn" :disabled="visibleCards.length < 2" @click="next">Next <GfIcon name="chevronRight" :size="16" /></button>
        </div>
      </div>
      <template #footer>
        <GfButton variant="danger" :loading="busy" @click="deleteSelected"><GfIcon name="trash" :size="15" /> Delete</GfButton>
        <GfButton variant="primary" @click="createIssue"><GfIcon name="sparkles" :size="15" /> Create issue</GfButton>
      </template>
    </GfModal>
  </div>
</template>

<style scoped>
.rec { width: 100%; max-width: var(--ws-content); margin: 0 auto; }
.stalebar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
  padding: 10px 14px;
  border-radius: 10px;
  background: var(--gf-amber-bg);
  color: var(--gf-amber);
  font-size: 12.5px;
  line-height: 1.4;
}
.stalebar :deep(.gf-icon) { flex: none; }
.stalebar span { flex: 1; }
.center {
  display: flex; flex-direction: column; align-items: center; gap: 12px;
  padding: 40px 20px; text-align: center; color: var(--gf-text-2);
}
.center :deep(.gf-icon) { color: var(--gf-purple); }
.center_error :deep(.gf-icon) { color: var(--gf-red); }

.summary {
  display: flex; align-items: flex-start; gap: 14px; flex-wrap: wrap;
  padding: 18px 20px; margin-bottom: 14px;
}
.summary__text { flex: 1; min-width: 200px; margin: 0; font-size: 13.5px; line-height: 1.6; }
.summary__kw { color: var(--gf-accent); font-weight: 700; }

.bar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.sortbtn {
  display: inline-flex;
  align-items: center;
  gap: 7px;
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
.sortbtn:hover {
  border-color: var(--gf-purple);
}
.sortbtn__arrow {
  color: var(--gf-accent);
  transition: transform 0.12s ease;
}
.sortbtn__arrow_up {
  transform: rotate(180deg);
}

/* category colors used by the card badges */
.catbg_security { --c: #d03232; }
.catbg_performance { --c: #b07400; }
.catbg_code_duplication { --c: #414ee0; }
.catbg_maintainability { --c: #0a8f6c; }
.catbg_architecture { --c: #7822f9; }

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(248px, 100%), 1fr));
  gap: 12px;
}
.mini {
  display: flex; flex-direction: column; gap: 10px; min-height: 150px;
  padding: 14px; text-align: left; border: 1px solid var(--gf-line);
  background: var(--gf-surface); cursor: pointer; font: inherit;
  transition: border-color 0.12s ease, box-shadow 0.12s ease;
}
.mini:hover { border-color: var(--gf-purple); box-shadow: var(--gf-shadow-pop); }
.mini__top { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.catbadge {
  display: inline-flex; align-items: center; height: 22px; padding: 0 9px;
  border-radius: 999px; font-size: 11px; font-weight: 700;
  color: var(--c, var(--gf-accent));
  background: color-mix(in srgb, var(--c, var(--gf-accent)) 12%, transparent);
}
.sev {
  display: inline-flex; align-items: center; height: 22px; padding: 0 9px;
  border-radius: 999px; font-size: 11px; font-weight: 700; text-transform: capitalize;
}
.sev_high { color: var(--gf-red); background: var(--gf-red-bg); }
.sev_medium { color: var(--gf-amber); background: var(--gf-amber-bg); }
.sev_low { color: var(--gf-text-2); background: var(--gf-surface-3); }
.mini__problem {
  flex: 1; margin: 0; font-size: 12.5px; line-height: 1.45; color: var(--gf-text);
  display: -webkit-box; -webkit-line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden;
}
.mini__foot { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.mini__file { font-size: 11px; color: var(--gf-text-3); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.conf { display: inline-flex; align-items: center; gap: 6px; font-size: 11px; color: var(--gf-text-2); font-weight: 600; flex: none; }
.conf__bar { width: 38px; height: 5px; border-radius: 999px; background: var(--gf-surface-3); overflow: hidden; }
.conf__fill { display: block; height: 100%; background: var(--gf-purple); }
.conf_lg { font-size: 12.5px; }
.conf_lg .conf__bar { width: 64px; }

.mockflag { margin: 18px 0 0; font-size: 11px; color: var(--gf-text-3); text-align: center; }
.mockflag code { color: var(--gf-accent); }

/* detail overlay */
.detail__badges { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-bottom: 12px; }
.detail__loc { display: flex; align-items: center; gap: 6px; margin: 0 0 16px; font-size: 12.5px; color: var(--gf-text-2); word-break: break-all; }
.detail__loc :deep(.gf-icon) { color: var(--gf-text-3); flex: none; }
.detail__h { margin: 14px 0 5px; font-size: 12px; text-transform: uppercase; letter-spacing: 0.03em; color: var(--gf-text-3); }
.detail__p { margin: 0; font-size: 13.5px; line-height: 1.6; color: var(--gf-text); }
.detail__nav { display: flex; justify-content: space-between; gap: 10px; margin-top: 20px; }
.navbtn {
  display: inline-flex; align-items: center; gap: 5px; height: 32px; padding: 0 14px;
  border: 1px solid var(--gf-line-2); border-radius: 8px; background: var(--gf-surface);
  font: inherit; font-size: 12.5px; font-weight: 600; color: var(--gf-text-2); cursor: pointer;
}
.navbtn:hover:not(:disabled) { border-color: var(--gf-purple); color: var(--gf-accent); }
.navbtn:disabled { opacity: 0.4; cursor: default; }
.navbtn__l { transform: rotate(180deg); }
</style>
