<script setup>
// Recursive, expandable file tree (GitHub-like). It shows folder/file NAMES and,
// on hover, the full path — it never shows file contents.
//
// When `interactive` is true (the Repository tab), every file and folder has an
// Exclude / Include toggle that keeps the repository's `.ai.yml` analysis.exclude
// list in sync:
//   - excluding a file strikes its name through (purple);
//   - excluding a folder strikes the whole subtree and collapses to `folder/**`;
//   - a folder shows a "partial" state when only some of its files are excluded;
//   - the `.ai.yml` config file itself is protected and cannot be excluded.
// The parent owns the pattern list; this component only reports toggles and
// renders the excluded/partial state passed back down via `excludedSet`.
import { ref } from 'vue'
import GfIcon from './ui/GfIcon.vue'
import { isProtected, isFolderFullyExcluded, isFolderPartiallyExcluded } from '../utils/excludePaths.js'

const props = defineProps({
  nodes: { type: Array, required: true },
  base: { type: String, default: '' },
  depth: { type: Number, default: 0 },
  // interactive exclude controls (Repository tab only)
  interactive: { type: Boolean, default: false },
  // Set of concrete excluded file paths (shared, computed by the parent).
  excludedSet: { type: Object, default: () => new Set() },
  // Flat list of every file path in the whole tree (for folder-state checks).
  allFiles: { type: Array, default: () => [] },
})
const emit = defineEmits(['toggle'])

// Local open/closed state for folders. All folders start collapsed.
const openMap = ref({})

function isOpen(full) {
  return openMap.value[full] ?? false
}
function toggleOpen(full) {
  openMap.value[full] = !isOpen(full)
}
function fullPath(node) {
  return props.base ? `${props.base}/${node.name}` : node.name
}
function fileExcluded(full) {
  return props.excludedSet.has(full)
}
function folderState(full) {
  if (isFolderFullyExcluded(full, props.allFiles, props.excludedSet)) return 'full'
  if (isFolderPartiallyExcluded(full, props.allFiles, props.excludedSet)) return 'partial'
  return 'none'
}
function emitToggle(type, path) {
  emit('toggle', { type, path })
}
</script>

<template>
  <ul class="tree" :class="{ tree_root: depth === 0 }">
    <li v-for="node in nodes" :key="fullPath(node)" class="tree__item">
      <!-- Folder -->
      <template v-if="node.type === 'dir'">
        <div class="tree__row tree__row_dir" :class="{ tree__row_excluded: folderState(fullPath(node)) === 'full' }">
          <button
            class="tree__main"
            :style="{ paddingLeft: 8 + depth * 16 + 'px' }"
            :title="fullPath(node)"
            @click="toggleOpen(fullPath(node))"
          >
            <GfIcon name="chevronRight" :size="13" class="tree__caret" :class="{ tree__caret_open: isOpen(fullPath(node)) }" />
            <GfIcon name="folder" :size="15" class="tree__ic tree__ic_dir" />
            <span class="tree__name">{{ node.name }}</span>
          </button>
          <button
            v-if="interactive"
            class="tree__excl"
            :class="{
              tree__excl_on: folderState(fullPath(node)) === 'full',
              tree__excl_partial: folderState(fullPath(node)) === 'partial',
            }"
            :title="folderState(fullPath(node)) === 'full' ? 'Include this folder in analysis' : 'Exclude this whole folder from analysis'"
            @click.stop="emitToggle('dir', fullPath(node))"
          >
            <GfIcon :name="folderState(fullPath(node)) === 'full' ? 'eye' : 'eyeOff'" :size="13" />
            <span class="tree__excl-label">{{ folderState(fullPath(node)) === 'full' ? 'Include' : folderState(fullPath(node)) === 'partial' ? 'Exclude rest' : 'Exclude' }}</span>
          </button>
        </div>
        <FileTree
          v-if="isOpen(fullPath(node)) && node.children"
          :nodes="node.children"
          :base="fullPath(node)"
          :depth="depth + 1"
          :interactive="interactive"
          :excluded-set="excludedSet"
          :all-files="allFiles"
          @toggle="emit('toggle', $event)"
        />
      </template>

      <!-- File -->
      <div
        v-else
        class="tree__row tree__row_file"
        :class="{ tree__row_excluded: fileExcluded(fullPath(node)) }"
        :title="fullPath(node)"
      >
        <span class="tree__caret-spacer" />
        <GfIcon name="file" :size="15" class="tree__ic tree__ic_file" />
        <span class="tree__name">{{ node.name }}</span>
        <span v-if="node.badge" class="tree__badge">{{ node.badge }}</span>

        <!-- interactive: exclude toggle (or a protected chip for .ai.yml) -->
        <template v-if="interactive">
          <span v-if="isProtected(fullPath(node))" class="tree__protected" title="The configuration file is always analysed and cannot be excluded.">
            <GfIcon name="lock" :size="11" /> always analysed
          </span>
          <button
            v-else
            class="tree__excl"
            :class="{ tree__excl_on: fileExcluded(fullPath(node)) }"
            :title="fileExcluded(fullPath(node)) ? 'Include this file in analysis' : 'Exclude this file from analysis'"
            @click="emitToggle('file', fullPath(node))"
          >
            <GfIcon :name="fileExcluded(fullPath(node)) ? 'eye' : 'eyeOff'" :size="13" />
            <span class="tree__excl-label">{{ fileExcluded(fullPath(node)) ? 'Include' : 'Exclude' }}</span>
          </button>
        </template>

        <!-- non-interactive: reveal full path on hover -->
        <span v-else class="tree__path">{{ fullPath(node) }}</span>
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
  padding-right: 10px;
}
.tree__row:hover {
  background: var(--gf-surface-2);
}
.tree__main {
  display: flex;
  align-items: center;
  gap: 7px;
  flex: 1;
  min-width: 0;
  padding: 6px 4px 6px 8px;
  border: 0;
  background: transparent;
  font: inherit;
  font-size: 13px;
  color: var(--gf-text);
  cursor: pointer;
  text-align: left;
}
.tree__row_file {
  padding: 6px 10px 6px 0;
}
.tree__row_file .tree__caret-spacer,
.tree__row_file .tree__ic,
.tree__row_file .tree__name,
.tree__row_file .tree__badge {
  /* keep file rows aligned with folder rows (which use .tree__main padding) */
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
  margin-left: 8px;
  flex: none;
}
.tree__ic_dir {
  color: var(--gf-purple);
  flex: none;
}
.tree__ic_file {
  color: var(--gf-text-3);
  flex: none;
}
.tree__name {
  font-weight: 600;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tree__row_file .tree__name {
  font-weight: 500;
}
/* excluded: strike through + purple */
.tree__row_excluded .tree__name {
  text-decoration: line-through;
  text-decoration-color: var(--gf-purple);
  color: var(--gf-accent);
}
.tree__row_excluded .tree__ic {
  opacity: 0.6;
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
  flex: none;
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

/* exclude / include toggle */
.tree__excl {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 22px;
  margin-left: auto;
  padding: 0 8px;
  border: 1px solid var(--gf-line-2);
  border-radius: 999px;
  background: var(--gf-surface);
  color: var(--gf-text-3);
  font: inherit;
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
  flex: none;
  opacity: 0;
  transition: opacity 0.12s ease, color 0.12s ease, border-color 0.12s ease, background-color 0.12s ease;
}
/* Always show the toggle when excluded/partial; on hover for included rows;
   and always on touch devices (no hover). */
.tree__row:hover .tree__excl,
.tree__excl_on,
.tree__excl_partial {
  opacity: 1;
}
@media (hover: none) {
  .tree__excl {
    opacity: 1;
  }
}
.tree__excl:hover {
  border-color: var(--gf-purple);
  color: var(--gf-accent);
}
.tree__excl_on {
  background: var(--gf-purple-soft);
  border-color: var(--gf-purple);
  color: var(--gf-accent);
}
.tree__excl_partial {
  background: var(--gf-amber-bg);
  border-color: transparent;
  color: var(--gf-amber);
}
.tree__protected {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-left: auto;
  padding: 0 8px;
  height: 20px;
  border-radius: 999px;
  background: var(--gf-surface-3);
  color: var(--gf-text-3);
  font-size: 10.5px;
  font-weight: 600;
  flex: none;
}
</style>
