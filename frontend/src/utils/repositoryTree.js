export function buildRepositoryTree(entries = []) {
  const root = []

  for (const entry of entries) {
    const path = String(entry?.path || '').replace(/^\/+|\/+$/g, '')
    if (!path) continue
    const parts = path.split('/').filter(Boolean)
    let level = root

    parts.forEach((name, index) => {
      const isLast = index === parts.length - 1
      const type = isLast && entry.type !== 'dir' ? 'file' : 'dir'
      let node = level.find((candidate) => candidate.name === name)
      if (!node) {
        node = type === 'dir' ? { type, name, children: [] } : { type, name }
        level.push(node)
      }
      if (node.type === 'dir') level = node.children
    })
  }

  sortNodes(root)
  return root
}

function sortNodes(nodes) {
  nodes.sort((left, right) => {
    if (left.type !== right.type) return left.type === 'dir' ? -1 : 1
    return left.name.localeCompare(right.name)
  })
  nodes.forEach((node) => {
    if (node.children) sortNodes(node.children)
  })
}
