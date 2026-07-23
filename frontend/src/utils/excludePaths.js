// Exclude-path logic shared by the Repository file tree and the Config form.
//
// The Config form stores `analysis.exclude` as a list of glob patterns. The
// Repository tab lets the user click files/folders to exclude/include them, and
// both views must stay in sync:
//   - a file is struck through when it is covered by an exclude pattern
//     (its exact path, a parent folder like `dir/**`, or a `*.ext` glob);
//   - a folder is struck through only when EVERY file under it is excluded;
//   - excluding a whole folder collapses its files into a single `folder/**`
//     pattern (optimisation the user asked for), and the reverse expands it.
//
// The `.ai.yml` configuration file is protected: it can never be excluded.
//
// Design: the source of truth for editing is the SET of concrete excluded file
// paths (derived from the patterns + the tree). Toggles mutate that set; then we
// serialise it back to a minimal pattern list, collapsing fully-excluded folders
// to `folder/**` and preserving any custom globs the user typed that do not map
// onto real tree files (e.g. `node_modules/**`, `*.min.js`).

export const PROTECTED_PATHS = ['.ai.yml']

// Return true if `path` cannot be excluded (the config file itself).
export function isProtected(path) {
  return PROTECTED_PATHS.includes(path)
}

// Match a single file path against one glob pattern. Supported forms:
//   - exact:        `backend/internal/httpapi/server.go`
//   - folder glob:  `backend/**`  or  `backend/internal/`   (trailing / or /**)
//   - extension:    `*.min.js`, `*.lock`  (matches by basename)
//   - name glob:    `*.go` (basename), `**/*.go`
export function matchPattern(filePath, pattern) {
  if (!filePath || !pattern) return false
  const p = pattern.trim()
  if (!p) return false
  if (p === filePath) return true

  // Folder patterns: `dir/**`, `dir/`, `dir/*`
  const folder = p.replace(/\/(\*\*|\*)?$/, '')
  if (p.endsWith('/**') || p.endsWith('/') || p.endsWith('/*')) {
    return filePath === folder || filePath.startsWith(folder + '/')
  }

  // Basename globs like `*.min.js`, `*.lock`, `*.go`
  if (p.includes('*')) {
    const base = filePath.split('/').pop()
    const target = p.startsWith('**/') ? p.slice(3) : p
    const re = new RegExp(
      '^' + target.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*') + '$',
    )
    return re.test(base) || re.test(filePath)
  }
  return false
}

// True if `filePath` is excluded by ANY of the patterns.
export function isExcluded(filePath, patterns) {
  return (patterns || []).some((p) => matchPattern(filePath, p))
}

// Build the "Exclude paths" picker suggestions from the REAL repository tree, so
// the Config dropdown never shows hard-coded placeholder patterns. It offers:
//   - `folder/**` for every directory in the tree (top-level and nested);
//   - a few basename globs (`*.min.js`, `*.lock`, `*.map`) ONLY when the repo
//     actually contains such files;
//   - top-level files as exact paths.
// Returns [] for an empty tree — the picker still lets the user type a custom
// pattern and press Enter, so nothing is lost before the tree has loaded.
export function excludePathOptionsFromTree(tree) {
  const dirs = []
  const files = []
  const walk = (nodes, base) => {
    for (const node of nodes || []) {
      const full = base ? `${base}/${node.name}` : node.name
      if (isProtected(full)) continue // never suggest excluding .ai.yml
      if (node.type === 'dir') {
        dirs.push(`${full}/**`)
        walk(node.children || [], full)
      } else {
        files.push(full)
      }
    }
  }
  walk(tree || [], '')

  // Basename globs are suggested only when the repository really has matching files.
  const basenames = files.map((f) => f.split('/').pop())
  const globs = []
  const hasBasename = (re) => basenames.some((b) => re.test(b))
  if (hasBasename(/\.min\.js$/)) globs.push('*.min.js')
  if (hasBasename(/\.lock$/) || basenames.includes('package-lock.json')) globs.push('*.lock')
  if (hasBasename(/\.map$/)) globs.push('*.map')

  const topLevelFiles = files.filter((f) => !f.includes('/'))
  // De-duplicate while preserving order: folders, then derived globs, then files.
  return [...new Set([...dirs, ...globs, ...topLevelFiles])]
}

// Flatten a nested file tree into a list of { path } for every file (dirs skipped).
export function flattenFiles(nodes, base = '') {
  const out = []
  for (const node of nodes || []) {
    const full = base ? `${base}/${node.name}` : node.name
    if (node.type === 'dir') out.push(...flattenFiles(node.children || [], full))
    else out.push(full)
  }
  return out
}

// The set of concrete file paths currently excluded (patterns expanded over the tree).
export function computeExcludedSet(allFiles, patterns) {
  const set = new Set()
  for (const f of allFiles) if (isExcluded(f, patterns)) set.add(f)
  return set
}

// Is every file under `folderPath` in the excluded set? (folder strike-through)
export function isFolderFullyExcluded(folderPath, allFiles, excludedSet) {
  const children = allFiles.filter(
    (f) => f === folderPath || f.startsWith(folderPath + '/'),
  )
  const editable = children.filter((f) => !isProtected(f))
  if (!editable.length) return false
  return editable.every((f) => excludedSet.has(f))
}

// Does the folder contain at least one excluded (but not all) file? (indeterminate)
export function isFolderPartiallyExcluded(folderPath, allFiles, excludedSet) {
  const children = allFiles
    .filter((f) => f === folderPath || f.startsWith(folderPath + '/'))
    .filter((f) => !isProtected(f))
  if (!children.length) return false
  const excluded = children.filter((f) => excludedSet.has(f)).length
  return excluded > 0 && excluded < children.length
}

// Serialise an excluded-file set back to a minimal pattern list: collapse any
// folder whose every file is excluded into `folder/**`, keep remaining files as
// exact paths, and preserve custom patterns that match no real tree file.
export function serialiseExcludes(tree, allFiles, excludedSet, originalPatterns = []) {
  const covered = new Set()
  const patterns = []

  function walk(nodes, base) {
    for (const node of nodes || []) {
      const full = base ? `${base}/${node.name}` : node.name
      if (node.type === 'dir') {
        if (isFolderFullyExcluded(full, allFiles, excludedSet)) {
          patterns.push(`${full}/**`)
          // mark all descendant files covered so we don't also emit them
          for (const f of allFiles) if (f === full || f.startsWith(full + '/')) covered.add(f)
        } else {
          walk(node.children || [], full)
        }
      } else if (excludedSet.has(full) && !covered.has(full) && !isProtected(full)) {
        patterns.push(full)
        covered.add(full)
      }
    }
  }
  walk(tree, '')

  // Preserve original custom globs that don't correspond to any tree file
  // (e.g. node_modules/**, *.min.js) so we never silently drop user input.
  for (const p of originalPatterns) {
    const matchesTree = allFiles.some((f) => matchPattern(f, p))
    if (!matchesTree && !patterns.includes(p)) patterns.push(p)
  }
  return patterns
}

// Toggle a single file's exclusion and return the new pattern list.
export function toggleFileExclude(tree, patterns, filePath) {
  if (isProtected(filePath)) return patterns
  const allFiles = flattenFiles(tree)
  const set = computeExcludedSet(allFiles, patterns)
  if (set.has(filePath)) set.delete(filePath)
  else set.add(filePath)
  return serialiseExcludes(tree, allFiles, set, patterns)
}

// Toggle a whole folder (all its files) and return the new pattern list.
export function toggleFolderExclude(tree, patterns, folderPath) {
  const allFiles = flattenFiles(tree)
  const set = computeExcludedSet(allFiles, patterns)
  const children = allFiles
    .filter((f) => f === folderPath || f.startsWith(folderPath + '/'))
    .filter((f) => !isProtected(f))
  if (!children.length) return patterns
  const fullyExcluded = children.every((f) => set.has(f))
  if (fullyExcluded) children.forEach((f) => set.delete(f))
  else children.forEach((f) => set.add(f))
  return serialiseExcludes(tree, allFiles, set, patterns)
}
