<script setup>
// Minimal inline-SVG icon set so we don't pull an icon library.
// Stroke-based icons inherit currentColor.
const props = defineProps({
  name: { type: String, required: true },
  size: { type: [Number, String], default: 18 },
})

const paths = {
  sparkles:
    'M12 3l1.9 4.6L18.5 9.5 13.9 11.4 12 16l-1.9-4.6L5.5 9.5l4.6-1.9L12 3zM19 14l.9 2.1 2.1.9-2.1.9L19 20l-.9-2.1-2.1-.9 2.1-.9L19 14z',
  lock: 'M6 10V8a6 6 0 1 1 12 0v2M5 10h14v10H5z',
  history: 'M3 3v6h6M3.5 9a9 9 0 1 0 2.6-5M12 7v5l4 2',
  folder: 'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z',
  file: 'M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8l-5-5zM14 3v5h5',
  chevronDown: 'M6 9l6 6 6-6',
  chevronRight: 'M9 6l6 6-6 6',
  close: 'M6 6l12 12M18 6L6 18',
  check: 'M4 12l5 5L20 6',
  branch: 'M6 3v12M6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM6 6a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 9a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 6c0 6-12 3-12 9',
  trash: 'M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13',
  refresh: 'M3 12a9 9 0 0 1 15-6.7L21 8M21 3v5h-5M21 12a9 9 0 0 1-15 6.7L3 16M3 21v-5h5',
  alert: 'M12 9v4M12 17h.01M10.3 4.3 2.5 18a2 2 0 0 0 1.7 3h15.6a2 2 0 0 0 1.7-3L13.7 4.3a2 2 0 0 0-3.4 0z',
  shield: 'M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6l8-3z',
  copy: 'M9 9h10v10H9zM5 15V5h10',
  doc: 'M7 3h7l5 5v13H7zM14 3v5h5M9 13h8M9 17h8',
  external: 'M14 5h5v5M19 5l-8 8M12 5H6a2 2 0 0 0-2 2v11a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2v-6',
  search: 'M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16zM21 21l-4.3-4.3',
  star: 'M12 3l2.9 6 6.6.9-4.8 4.6 1.2 6.5L12 18.6 6.1 21.5l1.2-6.5L2.5 9.9 9.1 9 12 3z',
}

const filled = new Set(['sparkles', 'star'])
</script>

<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    :fill="filled.has(name) ? 'currentColor' : 'none'"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    class="gf-icon"
  >
    <path :d="paths[name]" />
  </svg>
</template>

<style scoped>
.gf-icon {
  display: inline-block;
  flex: none;
  vertical-align: -0.15em;
}
</style>
