<script setup>
import GfIcon from './ui/GfIcon.vue'

defineProps({
  repo: { type: Object, required: true },
  files: { type: Array, required: true },
})
</script>

<template>
  <div class="files gf-card">
    <!-- Last commit row -->
    <div class="files__commit">
      <span class="files__avatar">{{ repo.lastCommit.author.charAt(0).toUpperCase() }}</span>
      <span class="files__author">{{ repo.lastCommit.author }}</span>
      <span class="files__msg">{{ repo.lastCommit.message }}</span>
      <span class="files__spacer" />
      <span class="files__hash mono">{{ repo.lastCommit.hash }}</span>
      <span class="files__when gf-muted">{{ repo.lastCommit.when }}</span>
    </div>

    <!-- File list -->
    <ul class="files__list">
      <li v-for="f in files" :key="f.name" class="files__row">
        <span class="files__icon" :class="f.type === 'dir' ? 'is-dir' : 'is-file'">
          <GfIcon :name="f.type === 'dir' ? 'folder' : 'file'" :size="16" />
        </span>
        <a href="#" class="files__name">{{ f.name }}</a>
        <span class="files__filemsg gf-muted">{{ f.message }}</span>
        <span class="files__when gf-muted">{{ f.when }}</span>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.files {
  overflow: hidden;
}
.files__commit {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  background: var(--gf-surface-2);
  border-bottom: 1px solid var(--gf-line);
  font-size: 13px;
}
.files__avatar {
  display: grid;
  place-items: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--gf-purple-soft);
  color: var(--gf-accent);
  font-size: 11px;
  font-weight: 700;
}
.files__author {
  font-weight: 600;
}
.files__msg {
  color: var(--gf-text-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.files__spacer {
  flex: 1;
}
.files__hash {
  font-size: 12px;
  color: var(--gf-text-2);
}
.files__list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.files__row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 11px 16px;
  border-bottom: 1px solid var(--gf-line);
  font-size: 13px;
}
.files__row:last-child {
  border-bottom: 0;
}
.files__row:hover {
  background: var(--gf-surface-2);
}
.files__icon.is-dir {
  color: var(--gf-purple);
}
.files__icon.is-file {
  color: var(--gf-text-3);
}
.files__name {
  font-weight: 600;
  color: var(--gf-text);
  min-width: 150px;
}
.files__name:hover {
  color: var(--gf-accent);
  text-decoration: underline;
}
.files__filemsg {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.files__when {
  font-size: 12px;
  white-space: nowrap;
}
@media (max-width: 720px) {
  .files__filemsg,
  .files__msg {
    display: none;
  }
}
</style>
