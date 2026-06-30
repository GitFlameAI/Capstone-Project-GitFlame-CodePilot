<script setup>
// Workspace shell (route /workspace). Four tabs, left to right:
//   Repository · Config · Autogeneration · Recommendations
//
// An AI disclaimer banner sits above the tab strip on every tab (dim but
// readable). Autogeneration and Recommendations are LOCKED (dimmed + lock icon)
// until a configuration has been saved, because both flows depend on the
// repository's .ai.yml. Clicking a locked tab nudges the user to the Config tab.
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { session } from '../store/session.js'
import GfIcon from '../components/ui/GfIcon.vue'
import RepositoryTab from '../components/workspace/RepositoryTab.vue'
import ConfigTab from '../components/workspace/ConfigTab.vue'
import AutogenTab from '../components/workspace/AutogenTab.vue'
import RecommendationsTab from '../components/workspace/RecommendationsTab.vue'

const router = useRouter()
const active = ref('repository')
const lockHint = ref(false)

// If the workspace is opened without a connected repository (e.g. a direct link
// or a refresh that cleared state), send the user back to the landing screen.
onMounted(() => {
  if (!session.connected) router.replace('/codepilot')
  // Open on the capability the user chose on the landing screen, but only if it
  // is unlocked; otherwise stay on Repository.
  if (session.configExists && session.intent === 'recommendations') active.value = 'recommendations'
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
          CodePilot uses AI. Plans, generated code and recommendations may contain mistakes —
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
  </div>
</template>

<style scoped>
.ws {
  min-height: 100vh;
}
.topbar {
  background: var(--gf-surface);
  border-bottom: 1px solid var(--gf-line);
}
.topbar__inner {
  max-width: 1080px;
  margin: 0 auto;
  padding: 0 24px;
  height: 56px;
  display: flex;
  align-items: center;
  gap: 14px;
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
  color: var(--gf-text-2);
  font-size: 13px;
}
.repo__name {
  font-weight: 600;
  color: var(--gf-text);
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
  max-width: 1080px;
  margin: 0 auto;
  padding: 18px 24px 72px;
}
.disclaimer {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 10px 14px;
  margin-bottom: 14px;
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
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 12px 0 0;
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
</style>
