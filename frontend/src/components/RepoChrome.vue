<script setup>
import { computed } from 'vue'
import GfIcon from './ui/GfIcon.vue'

const props = defineProps({
  repo: { type: Object, required: true },
  activeTab: { type: String, default: 'code' },
})
const emit = defineEmits(['tab'])

const tabs = computed(() => [
  { id: 'code', label: 'Code', icon: 'file' },
  { id: 'issues', label: 'Issues', icon: 'alert', count: 8 },
  { id: 'pulls', label: 'Pull Requests', icon: 'branch', count: 2 },
  { id: 'wiki', label: 'Wiki', icon: 'doc' },
  { id: 'settings', label: 'Settings', icon: 'shield' },
])
</script>

<template>
  <div class="chrome">
    <!-- Global top bar -->
    <div class="topbar">
      <div class="topbar__inner">
        <div class="brand">
          <span class="brand__mark"><GfIcon name="sparkles" :size="16" /></span>
          <span class="brand__name">GitFlame</span>
        </div>
        <label class="topsearch">
          <GfIcon name="search" :size="16" />
          <input type="search" placeholder="Search or jump to…" />
        </label>
        <div class="topbar__spacer" />
        <span class="gf-chip topbar__demo">CodePilot demo</span>
      </div>
    </div>

    <!-- Repository header -->
    <header class="repohead">
      <div class="repohead__inner">
        <div class="repohead__title">
          <a href="#" class="repohead__owner">{{ repo.owner }}</a>
          <span class="repohead__sep">/</span>
          <a href="#" class="repohead__name">{{ repo.name }}</a>
          <span class="gf-chip repohead__vis">Public</span>
        </div>
      </div>

      <!-- Tab strip -->
      <nav class="tabs" aria-label="Repository sections">
        <div class="tabs__inner">
          <button
            v-for="t in tabs"
            :key="t.id"
            class="tab"
            :class="{ tab_active: t.id === activeTab }"
            @click="emit('tab', t.id)"
          >
            <GfIcon :name="t.icon" :size="16" />
            <span>{{ t.label }}</span>
            <span v-if="t.count" class="tab__count">{{ t.count }}</span>
          </button>
        </div>
      </nav>
    </header>

    <main class="repobody">
      <div class="repobody__inner">
        <slot />
      </div>
    </main>
  </div>
</template>

<style scoped>
.topbar {
  background: var(--gf-surface);
  border-bottom: 1px solid var(--gf-line);
}
.topbar__inner,
.repohead__inner,
.tabs__inner,
.repobody__inner {
  max-width: 1180px;
  margin: 0 auto;
  padding: 0 24px;
}
.topbar__inner {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  min-height: 56px;
  padding: 8px 0;
}
.brand {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 700;
}
.brand__mark {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 9px;
  color: #fff;
  background: linear-gradient(135deg, var(--gf-purple), var(--gf-accent));
}
.brand__name {
  font-size: 15px;
}
.topsearch {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 34px;
  padding: 0 12px;
  min-width: 280px;
  border: 1px solid var(--gf-line-2);
  border-radius: 999px;
  color: var(--gf-text-3);
  background: var(--gf-surface-2);
}
.topsearch input {
  border: 0;
  outline: 0;
  background: transparent;
  font: inherit;
  font-size: 13px;
  color: var(--gf-text);
  width: 100%;
}
.topbar__spacer {
  flex: 1;
}
.topbar__demo {
  color: var(--gf-accent);
  border-color: var(--gf-line-2);
  background: var(--gf-purple-soft);
}

.repohead {
  background: var(--gf-surface);
  border-bottom: 1px solid var(--gf-line);
}
.repohead__inner {
  padding-top: 18px;
}
.repohead__title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 18px;
}
.repohead__owner {
  color: var(--gf-accent);
  font-weight: 600;
}
.repohead__name {
  color: var(--gf-accent);
  font-weight: 700;
}
.repohead__sep {
  color: var(--gf-text-3);
}
.repohead__vis {
  height: 22px;
  font-size: 11px;
  color: var(--gf-text-2);
}

.tabs {
  margin-top: 14px;
}
.tabs__inner {
  display: flex;
  gap: 4px;
  overflow-x: auto;
}
.tab {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 10px 14px;
  border: 0;
  background: transparent;
  color: var(--gf-text-2);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  white-space: nowrap;
}
.tab:hover {
  color: var(--gf-text);
}
.tab_active {
  color: var(--gf-text);
  border-bottom-color: var(--gf-purple);
}
.tab__count {
  display: inline-grid;
  place-items: center;
  min-width: 20px;
  height: 18px;
  padding: 0 6px;
  border-radius: 999px;
  background: var(--gf-surface-3);
  color: var(--gf-text-2);
  font-size: 11px;
}

.repobody__inner {
  padding-top: 22px;
  padding-bottom: 64px;
}
</style>
