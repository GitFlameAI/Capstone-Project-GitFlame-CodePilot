<script setup>
// Workspace shell (route /workspace). Four tabs, left to right:
//   Repository · Config · Autogeneration · Recommendations
//
// An AI disclaimer banner sits above the tab strip on every tab (dim but
// readable). Autogeneration and Recommendations are LOCKED (dimmed + lock icon)
// until a configuration has been saved, because both flows depend on the
// repository's .ai.yml. Clicking a locked tab nudges the user to the Config tab.
import { ref, computed, onMounted, watch, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { session, setToken } from '../store/session.js'
import { USING_MOCK } from '../api/index.js'
import GfIcon from '../components/ui/GfIcon.vue'
import GfButton from '../components/ui/GfButton.vue'
import RepositoryTab from '../components/workspace/RepositoryTab.vue'
import ConfigTab from '../components/workspace/ConfigTab.vue'
import AutogenTab from '../components/workspace/AutogenTab.vue'
import RecommendationsTab from '../components/workspace/RecommendationsTab.vue'

const router = useRouter()
const active = ref('repository')
const lockHint = ref(false)

// Always show a new tab from the top of the page (header first). Scrolling the
// whole window to the top keeps the sticky top bar — with the repo name and
// config status — in view, instead of landing on the AI disclaimer.
watch(active, async () => {
  await nextTick()
  window.scrollTo({ top: 0, behavior: 'auto' })
})

// --- access token gate (the token is never persisted, so a refresh needs it) ---
const tokenInput = ref('')
const showToken = ref(false)
// In mock mode there is no real auth, so a restored session doesn't need a token.
const tokenRequired = computed(() => !USING_MOCK && session.tokenStatus !== 'ok')

function submitToken() {
  const t = tokenInput.value.trim()
  if (!t) return
  setToken(t)
  tokenInput.value = ''
}

onMounted(() => {
  if (!session.connected) {
    router.replace('/codepilot')
    return
  }
  // Where to land: no saved config yet → Config (you must set it up first);
  // otherwise open the capability chosen on the landing screen.
  if (!session.configExists) active.value = 'config'
  else active.value = session.intent === 'recommendations' ? 'recommendations' : 'autogen'
})

const tabs = computed(() => [
  { id: 'repository', label: 'Repository', icon: 'folder', locked: false },
  { id: 'config', label: 'Config', icon: 'gear', locked: false },
  { id: 'autogen', label: 'Autogeneration', icon: 'sparkles', locked: !session.configExists },
  { id: 'recommendations', label: 'Recommendations', icon: 'shield', locked: !session.configExists },
])

function select(tab) {
  if (tab.locked) {
    lockHint.value = true
    setTimeout(() => (lockHint.value = false), 2600)
    return
  }
  active.value = tab.id
}

// Used by child tabs to jump elsewhere (e.g. Repository → Recommendations,
// Config "saved" → Autogeneration).
function goTo(id) {
  const tab = tabs.value.find((t) => t.id === id)
  if (tab && !tab.locked) active.value = id
}
</script>

<template>
  <div class="ws">
    <!-- top bar -->
    <div class="topbar">
      <div class="topbar__inner">
        <button class="brand" @click="router.push('/codepilot')">
          <span class="brand__mark"><GfIcon name="sparkles" :size="16" /></span>
          CodePilot
        </button>
        <span class="repo">
          <GfIcon name="folder" :size="15" />
          <span class="repo__name">{{ session.repo.owner }}/{{ session.repo.name }}</span>
          <span class="gf-chip repo__branch mono">{{ session.repo.defaultBranch }}</span>
        </span>
        <div class="topbar__spacer" />
        <span v-if="session.configExists" class="gf-chip status_ok">
          <GfIcon name="check" :size="13" /> .ai.yml active
        </span>
        <span v-else class="gf-chip status_warn">
          <GfIcon name="lock" :size="13" /> no config yet
        </span>
      </div>
    </div>

    <div class="shell">
      <!-- AI disclaimer banner (shown above tabs on every tab) -->
      <div class="disclaimer">
        <GfIcon name="info" :size="15" />
        <span>
          CodePilot uses AI. Plans, generated code and advices may contain mistakes —
          <strong>trust, but verify</strong> before you apply them.
        </span>
      </div>

      <!-- Tab strip -->
      <nav class="tabs" aria-label="Workspace sections">
        <button
          v-for="t in tabs"
          :key="t.id"
          class="tab"
          :class="{ tab_active: active === t.id, tab_locked: t.locked }"
          @click="select(t)"
        >
          <GfIcon :name="t.locked ? 'lock' : t.icon" :size="16" />
          <span>{{ t.label }}</span>
        </button>
      </nav>

      <transition name="lockfade">
        <p v-if="lockHint" class="lockmsg">
          <GfIcon name="lock" :size="14" />
          Save a configuration in the <strong>Config</strong> tab to unlock this.
        </p>
      </transition>

      <!-- Tab content -->
      <div class="content">
        <RepositoryTab v-if="active === 'repository'" @go="goTo" />
        <ConfigTab v-else-if="active === 'config'" @saved="goTo('autogen')" />
        <AutogenTab v-else-if="active === 'autogen'" />
        <RecommendationsTab v-else-if="active === 'recommendations'" @go="goTo" />
      </div>
    </div>

    <!-- Mandatory access-token gate. The token is never persisted, so after a
         refresh (or when the backend reports it expired/invalid) the user must
         re-enter it. The overlay cannot be dismissed until a token is provided. -->
    <div v-if="tokenRequired" class="tokgate">
      <div class="tokgate__card">
        <div class="tokgate__icon" :class="{ 'tokgate__icon_err': session.tokenStatus === 'invalid' }">
          <GfIcon :name="session.tokenStatus === 'invalid' ? 'alert' : 'key'" :size="22" />
        </div>
        <h2 class="tokgate__title">
          {{ session.tokenStatus === 'invalid' ? 'Access token problem' : 'Enter your access token' }}
        </h2>
        <p v-if="session.tokenStatus === 'invalid'" class="tokgate__err">
          {{ session.tokenError || 'The access token is invalid or has expired.' }} Please enter a valid token to continue.
        </p>
        <p v-else class="tokgate__sub">
          Your workspace for <strong>{{ session.repo.owner }}/{{ session.repo.name }}</strong> was restored,
          but the access token isn’t kept after a refresh. Re-enter it to keep using CodePilot.
        </p>

        <label class="tokgate__field">
          <span class="tokgate__label">Access token</span>
          <div class="tokgate__input">
            <GfIcon name="key" :size="16" class="tokgate__lead" />
            <input
              v-model="tokenInput"
              :type="showToken ? 'text' : 'password'"
              class="tokgate__inp"
              placeholder="xxxxxxxxxxxxxxxxxxxx"
              autofocus
              @keyup.enter="submitToken"
            />
            <button type="button" class="tokgate__toggle" @click="showToken = !showToken">
              <GfIcon :name="showToken ? 'eyeOff' : 'eye'" :size="16" />
            </button>
          </div>
        </label>

        <GfButton variant="primary" size="l" class="tokgate__btn" :disabled="!tokenInput.trim()" @click="submitToken">
          Continue
        </GfButton>
        <button class="tokgate__exit" @click="router.push('/codepilot')">Back to connect screen</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ws {
  min-height: 100vh;
}
.topbar {
  position: sticky;
  top: 0;
  z-index: 50;
  background: var(--gf-surface);
  border-bottom: 1px solid var(--gf-line);
}
.topbar__inner {
  max-width: 900px;
  margin: 0 auto;
  padding: 10px 24px;
  min-height: 56px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px 14px;
}
.brand {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: 0;
  background: transparent;
  font: inherit;
  font-weight: 700;
  font-size: 15px;
  color: var(--gf-text);
  cursor: pointer;
}
.brand__mark {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 9px;
  color: #fff;
  background: var(--gf-hero);
}
.repo {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  max-width: 100%;
  color: var(--gf-text-2);
  font-size: 13px;
}
.repo__name {
  font-weight: 600;
  color: var(--gf-text);
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.repo__branch {
  height: 22px;
  font-size: 11px;
}
.topbar__spacer {
  flex: 1;
}
.status_ok {
  color: var(--gf-green);
  background: var(--gf-green-bg);
  border-color: transparent;
}
.status_warn {
  color: var(--gf-amber);
  background: var(--gf-amber-bg);
  border-color: transparent;
}

.shell {
  max-width: var(--ws-content);
  margin: 0 auto;
  padding: 18px 20px 72px;
}
.disclaimer {
  display: flex;
  align-items: center;
  gap: 9px;
  max-width: 800px;
  margin: 0 auto 14px;
  padding: 10px 16px;
  border: 1px solid var(--gf-line-2);
  border-radius: 10px;
  background: var(--gf-purple-soft);
  color: var(--gf-text-2);
  font-size: 12.5px;
  line-height: 1.45;
}
.disclaimer :deep(.gf-icon) {
  color: var(--gf-purple);
  flex: none;
}
.disclaimer strong {
  color: var(--gf-accent);
}

.tabs {
  display: flex;
  gap: 4px;
  justify-content: center;
  max-width: var(--ws-narrow);
  margin: 0 auto;
  border-bottom: 1px solid var(--gf-line);
  overflow-x: auto;
}
.tab {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 11px 16px;
  border: 0;
  background: transparent;
  color: var(--gf-text-2);
  font: inherit;
  font-size: 13.5px;
  font-weight: 600;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  white-space: nowrap;
}
.tab:hover:not(.tab_locked) {
  color: var(--gf-text);
}
.tab_active {
  color: var(--gf-accent);
  border-bottom-color: var(--gf-purple);
}
.tab_locked {
  color: var(--gf-locked);
  cursor: not-allowed;
}
.lockmsg {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  max-width: var(--ws-narrow);
  margin: 12px auto 0;
  padding: 8px 12px;
  border-radius: 10px;
  background: var(--gf-amber-bg);
  color: var(--gf-amber);
  font-size: 12.5px;
}
.lockfade-enter-active,
.lockfade-leave-active {
  transition: opacity 0.2s ease;
}
.lockfade-enter-from,
.lockfade-leave-to {
  opacity: 0;
}
.content {
  padding-top: 22px;
}

/* mandatory access-token gate */
.tokgate {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  overflow-y: auto;
  padding: 40px 20px;
  background: rgba(24, 22, 34, 0.55);
  backdrop-filter: blur(3px);
}
.tokgate__card {
  width: 100%;
  max-width: 420px;
  margin: auto;
  padding: 28px 26px 22px;
  border-radius: var(--gf-radius-lg);
  background: var(--gf-surface);
  box-shadow: var(--gf-shadow-pop);
  text-align: center;
  border-top: 3px solid var(--gf-purple);
}
.tokgate__icon {
  display: grid;
  place-items: center;
  width: 52px;
  height: 52px;
  margin: 0 auto 14px;
  border-radius: 50%;
  background: var(--gf-purple-soft);
  color: var(--gf-purple);
}
.tokgate__icon_err {
  background: #fdecec;
  color: #c0392b;
}
.tokgate__title {
  margin: 0 0 8px;
  font-size: 19px;
}
.tokgate__sub,
.tokgate__err {
  margin: 0 0 18px;
  font-size: 13px;
  line-height: 1.55;
  color: var(--gf-text-2);
}
.tokgate__err {
  color: #c0392b;
}
.tokgate__sub strong {
  color: var(--gf-text);
}
.tokgate__field {
  display: block;
  text-align: left;
  margin-bottom: 16px;
}
.tokgate__label {
  display: block;
  margin-bottom: 7px;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--gf-text-2);
}
.tokgate__input {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 44px;
  padding: 0 12px;
  border: 2px solid var(--gf-purple);
  border-radius: 10px;
  background: var(--gf-surface);
  box-shadow: 0 0 0 4px var(--gf-purple-soft);
}
.tokgate__lead {
  color: var(--gf-purple);
  flex: none;
}
.tokgate__inp {
  flex: 1;
  min-width: 0;
  height: 100%;
  border: 0;
  outline: 0;
  background: transparent;
  font: inherit;
  font-size: 14px;
  color: var(--gf-text);
}
.tokgate__inp::-ms-reveal {
  display: none;
}
.tokgate__toggle {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--gf-text-3);
  cursor: pointer;
  flex: none;
}
.tokgate__toggle:hover {
  background: var(--gf-surface-3);
  color: var(--gf-text);
}
.tokgate__btn {
  width: 100%;
}
.tokgate__exit {
  margin-top: 12px;
  border: 0;
  background: transparent;
  font: inherit;
  font-size: 12.5px;
  color: var(--gf-text-3);
  cursor: pointer;
}
.tokgate__exit:hover {
  color: var(--gf-text);
  text-decoration: underline;
}
</style>
