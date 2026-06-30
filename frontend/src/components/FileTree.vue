<script setup>
// Recursive, expandable file tree (GitHub-like). It shows folder/file NAMES and,
// on hover, the full path — it never shows file contents, matching the brief:
// "only names and paths so the user can see where files live". Folders toggle
// open/closed; files are leaves.
import { ref } from 'vue'
import GfIcon from './ui/GfIcon.vue'

const props = defineProps({
  nodes: { type: Array, required: true },
  // path prefix accumulated from parent folders, used to build full paths
  base: { type: String, default: '' },
  depth: { type: Number, default: 0 },
})

// Track which folders are open. Top two levels start expanded for a helpful view.
const openMap = ref({})
function isOpen(node, full) {
  return openMap.value[full] ?? props.depth < 1
}
function toggle(node, full) {
  openMap.value[full] = !isOpen(node, full)
}
function fullPath(node) {
  return props.base ? `${props.base}/${node.name}` : node.name
}
</script>

<template>
  <ul class="tree" :class="{ tree_root: depth === 0 }">
    <li v-for="node in nodes" :key="fullPath(node)" class="tree__item">
      <!-- Folder -->
      <template v-if="node.type === 'dir'">
        <button
          class="tree__row tree__row_dir"
          :style="{ paddingLeft: 8 + depth * 16 + 'px' }"
          :title="fullPath(node)"
          @click="toggle(node, fullPath(node))"
        >
          <GfIcon
            name="chevronRight"
            :size="13"
            class="tree__caret"
            :class="{ 'tree__caret_open': isOpen(node, fullPath(node)) }"
          />
          <GfIcon name="folder" :size="15" class="tree__ic tree__ic_dir" />
          <span class="tree__name">{{ node.name }}</span>
        </button>
        <FileTree
          v-if="isOpen(node, fullPath(node)) && node.children"
          :nodes="node.children"
          :base="fullPath(node)"
          :depth="depth + 1"
        />
      </template>

      <!-- File -->
      <div
        v-else
        class="tree__row tree__row_file"
        :style="{ paddingLeft: 8 + depth * 16 + 'px' }"
        :title="fullPath(node)"
      >
        <span class="tree__caret-spacer" />
        <GfIcon name="file" :size="15" class="tree__ic tree__ic_file" />
        <span class="tree__name">{{ node.name }}</span>
        <span v-if="node.badge" class="tree__badge">{{ node.badge }}</span>
        <span class="tree__path">{{ fullPath(node) }}</span>
      </div>
    </li>
  </ul>
</template>

<style scoped>
.tree {
  list-style: none;
  margin: 0;
  padding: 0;
}
.tree_root {
  border: 1px solid var(--gf-line);
  border-radius: var(--gf-radius);
  background: var(--gf-surface);
  overflow: hidden;
  padding: 6px 0;
}
.tree__row {
  display: flex;
  align-items: center;
  gap: 7px;
  width: 100%;
  padding: 6px 14px 6px 8px;
  border: 0;
  background: transparent;
  font: inherit;
  font-size: 13px;
  color: var(--gf-text);
  cursor: pointer;
  text-align: left;
}
.tree__row_file {
  cursor: default;
}
.tree__row:hover {
  background: var(--gf-surface-2);
}
.tree__caret {
  color: var(--gf-text-3);
  transition: transform 0.12s ease;
  flex: none;
}
.tree__caret_open {
  transform: rotate(90deg);
}
.tree__caret-spacer {
  display: inline-block;
  width: 13px;
  flex: none;
}
.tree__ic_dir {
  color: var(--gf-purple);
}
.tree__ic_file {
  color: var(--gf-text-3);
}
.tree__name {
  font-weight: 600;
}
.tree__row_file .tree__name {
  font-weight: 500;
}
.tree__badge {
  display: inline-flex;
  align-items: center;
  height: 18px;
  padding: 0 7px;
  border-radius: 999px;
  background: var(--gf-purple-soft);
  color: var(--gf-accent);
  font-size: 10.5px;
  font-weight: 700;
}
.tree__path {
  margin-left: auto;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--gf-text-3);
  opacity: 0;
  transition: opacity 0.12s ease;
  white-space: nowrap;
}
.tree__row_file:hover .tree__path {
  opacity: 1;
}
@media (max-width: 620px) {
  .tree__path {
    display: none;
  }
}
</style>
