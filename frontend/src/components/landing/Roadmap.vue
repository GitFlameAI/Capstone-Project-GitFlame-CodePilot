<script setup>
// Landing "How it works" roadmap.
//
// Two tracks — Autogeneration and Recommendations — shown one at a time via a
// sliding toggle. Steps auto-advance; a progress bar fills along the connector
// toward the next circle. Timing is driven by a SINGLE timer whose remaining time
// is tracked across pauses, and the CSS progress bar pauses/resumes on the same
// events, so the bar and the step change stay in sync. Auto-advance can be paused
// with the play/pause control (top-left) and also pauses while the cursor is over
// the block. When a track ends it auto-switches to the other, so a visitor who
// does nothing still sees the whole tour.
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import GfIcon from '../ui/GfIcon.vue'

const AUTO_MS = 4200

const tracks = {
  autogen: {
    label: 'Autogeneration',
    icon: 'sparkles',
    steps: [
      { icon: 'link', title: 'Connect your repository', text: 'Point CodePilot at your GitFlame repo with a URL and an access token. This is the only setup step.' },
      { icon: 'gear', title: 'Configure once', text: 'Save a small .ai.yml: choose recommendation categories and mark any files to skip. This unlocks the AI tabs.' },
      { icon: 'sparkles', title: 'Describe an issue', text: 'Pick an existing repository issue (fields auto-fill) or write a new one. CodePilot gathers the relevant files for you.' },
      { icon: 'doc', title: 'Review the plan', text: 'CodePilot drafts a Markdown plan. Edit it, then approve, request a correction, or reject — nothing runs without you.' },
      { icon: 'branch', title: 'Get code + a pull request', text: 'On approval you get structured file changes plus a branch / commit / PR contract that GitFlame applies on its side.' },
    ],
  },
  recommendations: {
    label: 'Recommendations',
    icon: 'shield',
    steps: [
      { icon: 'search', title: 'Analyse the repository', text: 'CodePilot scans the repo for the categories you enabled — security, performance, maintainability and more.' },
      { icon: 'star', title: 'Browse the grid', text: 'Each card shows severity, category, file and a confidence score. Filter by category or severity and sort by confidence.' },
      { icon: 'doc', title: 'Open a card', text: 'Click any card to see the full problem and the concrete suggested fix, and page through the rest.' },
      { icon: 'plus', title: 'Act on it', text: 'Dismiss a card, or turn it into an issue that flows straight into the Autogeneration tab.' },
    ],
  },
}
const trackKeys = ['autogen', 'recommendations']

const track = ref('autogen')
const active = ref(0)
const autoOn = ref(true) // manual play/pause intent (default: on)
const hovering = ref(false) // cursor is over the block
// One-shot override: set when the user presses Resume WHILE hovering, so auto-
// advance keeps running even with the cursor on the block. It is cleared the
// moment the cursor leaves, so re-entering pauses again by default.
const hoverOverride = ref(false)

const steps = computed(() => tracks[track.value].steps)
const current = computed(() => steps.value[active.value])

const reduceMotion =
  typeof window !== 'undefined' &&
  window.matchMedia &&
  window.matchMedia('(prefers-reduced-motion: reduce)').matches

// Effective running state. Auto-advance is on, motion is allowed, and either the
// cursor isn't hovering OR the user has explicitly resumed while hovering. The
// play/pause icon, the dim styling and the CSS progress bar all follow this, so
// hovering visibly shows the paused state on the control in the top-left corner.
const running = computed(
  () => autoOn.value && !reduceMotion && (!hovering.value || hoverOverride.value),
)

let timer = null
let startTs = 0
let remaining = AUTO_MS

function clearTimer() {
  if (timer) {
    clearTimeout(timer)
    timer = null
  }
}
function runTimer() {
  clearTimer()
  if (!running.value) return
  startTs = Date.now()
  timer = setTimeout(() => {
    advance()
    remaining = AUTO_MS
    if (running.value) runTimer()
  }, remaining)
}
function holdTimer() {
  clearTimer()
  if (startTs) remaining = Math.max(150, remaining - (Date.now() - startTs))
}
function resetStep() {
  remaining = AUTO_MS
}
function advance() {
  if (active.value < steps.value.length - 1) {
    active.value += 1
  } else {
    track.value = track.value === 'autogen' ? 'recommendations' : 'autogen'
    active.value = 0
  }
}

// Pause / resume the single timer whenever the effective running state flips.
watch(running, (on) => {
  if (on) runTimer()
  else holdTimer()
})

// The control reflects and drives the EFFECTIVE running state:
//   - running now -> pause (turn auto-advance off, drop any hover override);
//   - paused now  -> resume; if the cursor is on the block, override the hover-
//                    pause so it runs anyway (until the cursor leaves and returns).
function toggleAuto() {
  if (running.value) {
    autoOn.value = false
    hoverOverride.value = false
  } else {
    autoOn.value = true
    if (hovering.value) hoverOverride.value = true
  }
}
function onEnter() {
  hovering.value = true
}
function onLeave() {
  hovering.value = false
  hoverOverride.value = false // re-entering pauses again by default
}
function selectTrack(t) {
  if (track.value !== t) {
    track.value = t
    active.value = 0
  }
  resetStep()
  if (running.value) runTimer()
}
function selectStep(i) {
  active.value = i
  resetStep()
  if (running.value) runTimer()
}

onMounted(() => {
  if (running.value) runTimer()
})
onBeforeUnmount(clearTimer)
</script>

<template>
  <div class="rm" :class="{ rm_paused: !running }" @mouseenter="onEnter" @mouseleave="onLeave">
    <!-- top bar: play/pause + sliding track toggle -->
    <div class="rm__top">
      <button
        class="rm__pp"
        :class="{ rm__pp_off: !running }"
        :aria-label="running ? 'Pause auto-advance' : 'Resume auto-advance'"
        :title="running ? 'Pause auto-advance' : 'Resume auto-advance'"
        @click="toggleAuto"
      >
        <GfIcon :name="running ? 'pause' : 'play'" :size="13" />
      </button>

      <div class="rm__tabs" role="tablist" aria-label="Capabilities">
        <span class="rm__pill" :class="{ rm__pill_right: track === 'recommendations' }" aria-hidden="true" />
        <button
          v-for="k in trackKeys"
          :key="k"
          class="rm__tab"
          :class="{ rm__tab_on: track === k }"
          role="tab"
          :aria-selected="track === k"
          @click="selectTrack(k)"
        >
          <GfIcon :name="tracks[k].icon" :size="15" /> <span class="rm__tab-label">{{ tracks[k].label }}</span>
        </button>
      </div>

      <span class="rm__top-spacer" aria-hidden="true" />
    </div>

    <!-- numbered track with a flowing progress connector -->
    <ol class="rm__track">
      <li
        v-for="(s, i) in steps"
        :key="i"
        class="rm__node"
        :class="{ rm__node_active: i === active, rm__node_done: i < active }"
      >
        <span v-if="i === active + 1" :key="track + '-' + active" class="rm__flow" />
        <button class="rm__dot" :aria-label="s.title" @click="selectStep(i)">
          <GfIcon v-if="i < active" name="check" :size="15" />
          <span v-else>{{ i + 1 }}</span>
        </button>
        <span class="rm__caption">{{ s.title }}</span>
      </li>
    </ol>

    <!-- active step detail -->
    <transition name="rm-fade" mode="out-in">
      <div :key="track + '-' + active" class="rm__panel">
        <div class="rm__scene">
          <svg v-if="track === 'autogen' && active === 0" viewBox="0 0 220 130" class="scene">
            <rect x="26" y="34" width="76" height="62" rx="10" class="s-card" />
            <rect x="38" y="48" width="40" height="7" rx="3.5" class="s-line" />
            <rect x="38" y="62" width="52" height="6" rx="3" class="s-line-2" />
            <rect x="38" y="74" width="30" height="6" rx="3" class="s-line-2" />
            <path d="M106 65 h34" class="s-plug" />
            <circle cx="150" cy="65" r="12" class="s-plug-dot" />
            <rect x="150" y="40" width="44" height="50" rx="10" class="s-card-2" />
            <path d="M160 65 l7 7 l14 -16" class="s-check" />
          </svg>
          <svg v-else-if="track === 'autogen' && active === 1" viewBox="0 0 220 130" class="scene">
            <rect x="46" y="24" width="128" height="82" rx="12" class="s-card" />
            <rect x="60" y="38" width="30" height="7" rx="3.5" class="s-accent" />
            <rect x="60" y="54" width="90" height="6" rx="3" class="s-line-2" />
            <rect x="60" y="66" width="70" height="6" rx="3" class="s-line-2" />
            <rect x="60" y="82" width="26" height="14" rx="7" class="s-pill" />
            <rect x="92" y="82" width="26" height="14" rx="7" class="s-pill" />
            <rect x="124" y="82" width="26" height="14" rx="7" class="s-pill-off" />
          </svg>
          <svg v-else-if="track === 'autogen' && active === 2" viewBox="0 0 220 130" class="scene">
            <rect x="40" y="22" width="140" height="86" rx="12" class="s-card" />
            <circle cx="58" cy="40" r="6" class="s-accent-fill" />
            <rect x="72" y="36" width="80" height="8" rx="4" class="s-line" />
            <rect x="54" y="58" width="112" height="6" rx="3" class="s-line-2" />
            <rect x="54" y="70" width="96" height="6" rx="3" class="s-line-2" />
            <rect x="54" y="82" width="60" height="6" rx="3" class="s-line-2" />
            <rect x="120" y="90" width="46" height="12" rx="6" class="s-accent" />
          </svg>
          <svg v-else-if="track === 'autogen' && active === 3" viewBox="0 0 220 130" class="scene">
            <rect x="46" y="20" width="128" height="90" rx="12" class="s-card" />
            <rect x="60" y="32" width="54" height="9" rx="4.5" class="s-accent" />
            <rect x="60" y="50" width="98" height="6" rx="3" class="s-line-2" />
            <rect x="60" y="62" width="86" height="6" rx="3" class="s-line-2" />
            <rect x="60" y="74" width="98" height="6" rx="3" class="s-line-2" />
            <path d="M150 92 l6 -6 l10 10 l-6 6 h-10 z" class="s-pencil" />
          </svg>
          <svg v-else-if="track === 'autogen' && active === 4" viewBox="0 0 220 130" class="scene">
            <rect x="30" y="30" width="94" height="16" rx="8" class="s-file-create" />
            <rect x="30" y="54" width="94" height="16" rx="8" class="s-file-modify" />
            <rect x="30" y="78" width="94" height="16" rx="8" class="s-file-modify" />
            <path d="M150 34 v58" class="s-branch" />
            <circle cx="150" cy="34" r="7" class="s-branch-dot" />
            <circle cx="150" cy="92" r="7" class="s-branch-dot" />
            <path d="M150 58 q22 4 22 26 v8" class="s-branch" />
            <circle cx="172" cy="92" r="7" class="s-branch-dot s-branch-dot_accent" />
          </svg>
          <svg v-else-if="track === 'recommendations' && active === 0" viewBox="0 0 220 130" class="scene">
            <rect x="40" y="24" width="140" height="82" rx="12" class="s-card" />
            <rect x="54" y="40" width="70" height="6" rx="3" class="s-line-2" />
            <rect x="54" y="54" width="96" height="6" rx="3" class="s-line-2" />
            <rect x="54" y="68" width="60" height="6" rx="3" class="s-line-2" />
            <circle cx="150" cy="78" r="18" class="s-scan" />
            <path d="M163 91 l12 12" class="s-scan-handle" />
          </svg>
          <svg v-else-if="track === 'recommendations' && active === 1" viewBox="0 0 220 130" class="scene">
            <rect x="30" y="26" width="74" height="34" rx="8" class="s-card" />
            <rect x="116" y="26" width="74" height="34" rx="8" class="s-card" />
            <rect x="30" y="70" width="74" height="34" rx="8" class="s-card" />
            <rect x="116" y="70" width="74" height="34" rx="8" class="s-card" />
            <circle cx="42" cy="38" r="4" class="s-sev-high" />
            <circle cx="128" cy="38" r="4" class="s-sev-med" />
            <circle cx="42" cy="82" r="4" class="s-sev-med" />
            <circle cx="128" cy="82" r="4" class="s-sev-low" />
            <rect x="52" y="35" width="40" height="6" rx="3" class="s-line-2" />
            <rect x="138" y="35" width="40" height="6" rx="3" class="s-line-2" />
            <rect x="52" y="79" width="40" height="6" rx="3" class="s-line-2" />
            <rect x="138" y="79" width="40" height="6" rx="3" class="s-line-2" />
          </svg>
          <svg v-else-if="track === 'recommendations' && active === 2" viewBox="0 0 220 130" class="scene">
            <rect x="52" y="20" width="116" height="90" rx="12" class="s-card-2" />
            <circle cx="68" cy="36" r="5" class="s-sev-high" />
            <rect x="80" y="32" width="60" height="7" rx="3.5" class="s-accent" />
            <rect x="66" y="52" width="88" height="6" rx="3" class="s-line-2" />
            <rect x="66" y="64" width="72" height="6" rx="3" class="s-line-2" />
            <rect x="66" y="82" width="60" height="9" rx="4.5" class="s-conf" />
            <rect x="66" y="82" width="40" height="9" rx="4.5" class="s-conf-fill" />
          </svg>
          <svg v-else viewBox="0 0 220 130" class="scene">
            <rect x="24" y="42" width="76" height="46" rx="10" class="s-card" />
            <circle cx="40" cy="56" r="4" class="s-sev-high" />
            <rect x="52" y="53" width="34" height="6" rx="3" class="s-line-2" />
            <rect x="38" y="70" width="48" height="6" rx="3" class="s-line-2" />
            <path d="M104 65 h30 M126 58 l9 7 l-9 7" class="s-arrow" />
            <rect x="140" y="42" width="56" height="46" rx="10" class="s-card-2" />
            <path d="M150 65 h20 M160 55 v20" class="s-plus" />
          </svg>
        </div>
        <div class="rm__text">
          <span class="rm__step">Step {{ active + 1 }} of {{ steps.length }} · {{ tracks[track].label }}</span>
          <h3 class="rm__title"><GfIcon :name="current.icon" :size="17" /> {{ current.title }}</h3>
          <p class="rm__desc">{{ current.text }}</p>
        </div>
      </div>
    </transition>
  </div>
</template>

<style scoped>
.rm {
  position: relative;
  border: 1px solid var(--gf-line);
  border-radius: var(--gf-radius-lg);
  background: var(--gf-surface);
  box-shadow: var(--gf-shadow-sm);
  padding: 16px 22px 20px;
}

/* top bar */
.rm__top {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 20px;
}
.rm__pp {
  flex: none;
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border: 1px solid var(--gf-line-2);
  border-radius: 8px;
  background: var(--gf-surface-2);
  color: #9a90b3; /* dim grey-purple */
  cursor: pointer;
  transition: color 0.15s ease, border-color 0.15s ease, background-color 0.15s ease;
}
.rm__pp:hover {
  color: var(--gf-accent);
  border-color: var(--gf-purple);
}
.rm__pp_off {
  background: var(--gf-purple-soft);
  color: var(--gf-purple);
}
.rm__top-spacer {
  flex: none;
  width: 28px;
}

/* sliding track toggle */
.rm__tabs {
  position: relative;
  display: flex;
  gap: 0;
  margin: 0 auto;
  width: 100%;
  max-width: 420px;
  padding: 4px;
  border: 1px solid var(--gf-line-2);
  border-radius: 999px;
  background: var(--gf-surface-2);
}
.rm__pill {
  position: absolute;
  top: 4px;
  left: 4px;
  width: calc(50% - 4px);
  height: calc(100% - 8px);
  border-radius: 999px;
  background: var(--gf-surface);
  box-shadow: var(--gf-shadow-sm);
  transition: transform 0.28s cubic-bezier(0.4, 0, 0.2, 1);
}
.rm__pill_right {
  transform: translateX(100%);
}
.rm__tab {
  position: relative;
  z-index: 1;
  flex: 1 1 0;
  min-width: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  height: 36px;
  border: 0;
  border-radius: 999px;
  background: transparent;
  color: var(--gf-text-2);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: color 0.18s ease, box-shadow 0.15s ease;
}
.rm__tab-label {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.rm__tab_on {
  color: var(--gf-accent);
}
.rm__tab:not(.rm__tab_on):hover {
  color: var(--gf-accent);
  box-shadow: inset 0 0 0 1.5px var(--gf-purple);
}
.rm__tab :deep(.gf-icon) {
  flex: none;
  color: currentColor;
}

/* numbered track */
.rm__track {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 4px;
  margin: 0 0 20px;
  padding: 0;
  list-style: none;
}
.rm__node {
  position: relative;
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  text-align: center;
}
.rm__node::before {
  content: '';
  position: absolute;
  top: 15px;
  left: -50%;
  width: 100%;
  height: 2px;
  background: var(--gf-line-2);
  z-index: 0;
}
.rm__node:first-child::before {
  display: none;
}
.rm__node_active::before,
.rm__node_done::before {
  background: var(--gf-purple);
}
.rm__flow {
  position: absolute;
  top: 15px;
  left: -50%;
  width: 100%;
  height: 2px;
  background: var(--gf-purple);
  transform-origin: left center;
  transform: scaleX(0);
  z-index: 1;
  animation: rm-flow 4200ms linear forwards;
}
.rm_paused .rm__flow {
  animation-play-state: paused;
}
@keyframes rm-flow {
  from { transform: scaleX(0); }
  to { transform: scaleX(1); }
}
.rm__dot {
  position: relative;
  z-index: 2;
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: 2px solid var(--gf-line-2);
  background: var(--gf-surface);
  color: var(--gf-text-3);
  font: inherit;
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  transition: all 0.18s ease;
}
.rm__dot:hover {
  border-color: var(--gf-purple);
  color: var(--gf-accent);
}
.rm__node_done .rm__dot {
  border-color: var(--gf-purple);
  background: var(--gf-purple);
  color: #fff;
}
.rm__node_active .rm__dot {
  border-color: var(--gf-purple);
  background: var(--gf-purple);
  color: #fff;
  box-shadow: 0 0 0 5px var(--gf-purple-soft);
  transform: scale(1.06);
}
.rm__caption {
  font-size: 11px;
  font-weight: 600;
  line-height: 1.3;
  color: var(--gf-text-3);
  max-width: 92px;
}
.rm__node_active .rm__caption {
  color: var(--gf-accent);
}

/* detail panel */
.rm__panel {
  display: grid;
  grid-template-columns: 220px 1fr;
  align-items: center;
  gap: 22px;
  padding: 6px 6px 4px;
}
.rm__scene {
  display: grid;
  place-items: center;
  height: 130px;
  border-radius: var(--gf-radius);
  background: var(--gf-hero-soft);
  border: 1px solid var(--gf-line);
}
.scene {
  width: 100%;
  height: 100%;
}
.rm__step {
  display: inline-block;
  margin-bottom: 8px;
  padding: 3px 10px;
  border-radius: 999px;
  background: var(--gf-purple-soft);
  color: var(--gf-accent);
  font-size: 11px;
  font-weight: 700;
}
.rm__title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 8px;
  font-size: 17px;
}
.rm__title :deep(.gf-icon) {
  color: var(--gf-purple);
}
.rm__desc {
  margin: 0;
  font-size: 13.5px;
  line-height: 1.6;
  color: var(--gf-text-2);
}

/* scene primitives */
.s-card { fill: var(--gf-surface); stroke: var(--gf-line-2); stroke-width: 2; }
.s-card-2 { fill: var(--gf-purple-soft); stroke: var(--gf-purple); stroke-width: 2; }
.s-line { fill: var(--gf-text-3); opacity: 0.55; }
.s-line-2 { fill: var(--gf-line-2); }
.s-accent { fill: var(--gf-purple); }
.s-accent-fill { fill: var(--gf-purple); }
.s-pill { fill: var(--gf-purple-soft); stroke: var(--gf-purple); stroke-width: 1.5; }
.s-pill-off { fill: none; stroke: var(--gf-line-2); stroke-width: 1.5; }
.s-plug { stroke: var(--gf-purple); stroke-width: 2.5; stroke-dasharray: 4 4; fill: none; }
.s-plug-dot { fill: var(--gf-purple); }
.s-check { fill: none; stroke: var(--gf-purple); stroke-width: 3; stroke-linecap: round; stroke-linejoin: round; }
.s-pencil { fill: var(--gf-purple); }
.s-file-create { fill: var(--gf-green-bg); stroke: var(--gf-green); stroke-width: 1.5; }
.s-file-modify { fill: var(--gf-blue-bg); stroke: var(--gf-blue); stroke-width: 1.5; }
.s-branch { fill: none; stroke: var(--gf-purple); stroke-width: 2.5; }
.s-branch-dot { fill: var(--gf-surface); stroke: var(--gf-purple); stroke-width: 2.5; }
.s-branch-dot_accent { fill: var(--gf-purple); }
.s-scan { fill: none; stroke: var(--gf-purple); stroke-width: 2.5; }
.s-scan-handle { stroke: var(--gf-purple); stroke-width: 3; stroke-linecap: round; }
.s-sev-high { fill: #d64545; }
.s-sev-med { fill: var(--gf-amber); }
.s-sev-low { fill: var(--gf-blue); }
.s-conf { fill: var(--gf-line-2); }
.s-conf-fill { fill: var(--gf-purple); }
.s-arrow { fill: none; stroke: var(--gf-purple); stroke-width: 2.5; stroke-linecap: round; stroke-linejoin: round; }
.s-plus { stroke: var(--gf-purple); stroke-width: 2.5; stroke-linecap: round; }

.rm-fade-enter-active,
.rm-fade-leave-active {
  transition: opacity 0.25s ease, transform 0.25s ease;
}
.rm-fade-enter-from {
  opacity: 0;
  transform: translateY(6px);
}
.rm-fade-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

@media (max-width: 620px) {
  .rm__caption {
    display: none;
  }
  .rm__panel {
    grid-template-columns: 1fr;
    gap: 16px;
  }
  .rm__scene {
    height: 120px;
  }
}

/* very narrow / high zoom: stack the toggle so long labels never overflow */
@media (max-width: 440px) {
  .rm__top {
    flex-wrap: wrap;
  }
  .rm__top-spacer {
    display: none;
  }
  .rm__tabs {
    flex-direction: column;
    gap: 4px;
    border-radius: 14px;
  }
  .rm__pill {
    display: none;
  }
  .rm__tab {
    justify-content: flex-start;
    padding: 0 12px;
  }
  .rm__tab_on {
    background: var(--gf-surface);
    box-shadow: var(--gf-shadow-sm);
  }
}
</style>
