// Demo data and pure helpers used by the CodePilot demo.
//
// In a real integration GitFlame supplies the repository metadata, issues and file
// tree; here it is static so the demo runs without any backend. The configuration
// helpers (buildYaml / parseYamlToForm / defaultConfigForm) are also used by the
// Config tab and the session store, so they live here as a single source of truth.
//
// The configuration follows the agreed contract in
//   docs/config/ai_config_spec.md  (branch: sprint-3/danil-codegen-contracts)
// which is intentionally small: repository.default_branch, analysis.exclude,
// recommendations.categories and storage.recommendation_ttl_days.

// The repository id is a slug (no slashes) so it is also safe for the real
// backend's `/repositories/{id}` route. Owner/name are shown for display only.
export const DEMO_REPO = {
  id: 'tiroro-20-10-test',
  owner: 'tiroro-20-10',
  name: 'test',
  url: 'https://gitflametest.ru/tiroro-20-10/test',
  defaultBranch: 'main',
}

// Last-commit line shown on the demo GitFlame page.
export const demoLastCommit = {
  message: 'Add async agent tasks and Redis-backed plan generation',
  author: 'artur',
  hash: 'a1b2c3d',
  when: '2 days ago',
}

// Flat file list for the demo GitFlame entry page (Code tab look).
export const demoFiles = [
  { type: 'dir', name: 'backend', message: 'Go API service and integration contracts', when: '2 days ago' },
  { type: 'dir', name: 'frontend', message: 'Vue demo UI (Work with AI + recommendations)', when: 'just now' },
  { type: 'dir', name: 'recommendations', message: 'Agent Engine and recommendation model service', when: '4 days ago' },
  { type: 'dir', name: 'docs', message: 'Architecture, API contracts and report sections', when: '3 days ago' },
  { type: 'dir', name: 'infra', message: 'Docker and deployment notes', when: '5 days ago' },
  { type: 'file', name: 'docker-compose.yml', message: 'Backend, ML service and database services', when: '5 days ago' },
  { type: 'file', name: 'README.md', message: 'Project overview and quick start', when: '2 days ago' },
]

// Nested file tree for the Repository tab. `hasConfig` controls whether the
// `.ai.yml` file is present (it appears only after the user saves a config).
export function demoFileTree(hasConfig) {
  const tree = [
    {
      type: 'dir', name: 'backend', children: [
        {
          type: 'dir', name: 'internal', children: [
            { type: 'dir', name: 'httpapi', children: [
              { type: 'file', name: 'server.go' },
              { type: 'file', name: 'openapi.json' },
            ] },
            { type: 'dir', name: 'service', children: [
              { type: 'file', name: 'workflow.go' },
              { type: 'file', name: 'config.go' },
            ] },
            { type: 'dir', name: 'repository', children: [
              { type: 'file', name: 'postgres.go' },
              { type: 'file', name: 'memory.go' },
            ] },
            { type: 'dir', name: 'domain', children: [{ type: 'file', name: 'domain.go' }] },
          ],
        },
        { type: 'dir', name: 'cmd', children: [{ type: 'file', name: 'server/main.go' }] },
        { type: 'file', name: 'go.mod' },
      ],
    },
    {
      type: 'dir', name: 'frontend', children: [
        { type: 'dir', name: 'src', children: [
          { type: 'file', name: 'main.js' },
          { type: 'file', name: 'router.js' },
        ] },
        { type: 'file', name: 'package.json' },
      ],
    },
    {
      type: 'dir', name: 'recommendations', children: [
        { type: 'dir', name: 'src/agent_engine', children: [
          { type: 'file', name: 'service.py' },
          { type: 'file', name: 'loop.py' },
        ] },
        { type: 'file', name: 'recommendation_schema.json' },
      ],
    },
    { type: 'file', name: 'docker-compose.yml' },
    { type: 'file', name: 'README.md' },
  ]
  if (hasConfig) {
    tree.push({ type: 'file', name: '.ai.yml', badge: 'CodePilot' })
  }
  return tree
}

// Issues that already exist in the demo repository. In a real integration GitFlame
// supplies these; the Autogeneration tab lets the user pick one of these (fields are
// auto-filled) or create a new issue from scratch.
export const demoIssues = [
  {
    id: 'ISSUE-101',
    title: 'Add pagination to the repository list endpoint',
    body: 'The /repositories endpoint returns every record at once. We need offset/limit pagination and a total count so the UI can render pages.',
    author: 'tiroro-20-10',
    labels: ['enhancement', 'backend'],
  },
  {
    id: 'ISSUE-102',
    title: 'Graceful shutdown for the API server',
    body: 'On restart, in-flight requests can be dropped. Add signal handling and call Shutdown(ctx) on SIGTERM/SIGINT so the server drains connections cleanly.',
    author: 'artur',
    labels: ['reliability', 'backend'],
  },
  {
    id: 'ISSUE-103',
    title: 'Return a clear validation error for invalid .ai.yml',
    body: 'Saving an invalid configuration currently fails with a 500. It should return a structured validation error describing which field is wrong.',
    author: 'roma',
    labels: ['backend', 'validation'],
  },
]

// A file node that a simulated GitFlame push adds to the tree (see
// store/session.js::applyMockPush). The commit hash keeps each push distinct.
export function pushedFileNode(commit) {
  return { type: 'file', name: 'ratelimit.go', badge: 'new', pushedCommit: commit }
}

// An issue a simulated GitFlame push opens in the repository.
export function pushedIssue() {
  return {
    id: 'ISSUE-204',
    title: 'Add request rate limiting to the public API',
    body: 'Unauthenticated endpoints have no rate limiting. Add a small token-bucket limiter and return 429 with a Retry-After header when the limit is exceeded.',
    author: 'ruslan',
    labels: ['security', 'backend'],
  }
}

// NOTE: the Config "exclude paths" picker no longer uses a hard-coded list. Its
// suggestions are derived from the real connected repository tree — see
// excludePathOptionsFromTree() in src/utils/excludePaths.js.

export const RECOMMENDATION_CATEGORIES = [
  { id: 'code_duplication', label: 'Code duplication' },
  { id: 'security', label: 'Security' },
  { id: 'maintainability', label: 'Maintainability' },
  { id: 'performance', label: 'Performance' },
  { id: 'architecture', label: 'Architecture' },
]

// ---------------------------------------------------------------------------
// Configuration form <-> YAML  (docs/config/ai_config_spec.md)
// ---------------------------------------------------------------------------
// The agreed contract is small:
//   repository.default_branch
//   analysis.enabled / analysis.exclude
//   recommendations.enabled / recommendations.categories
//   storage.recommendation_ttl_days
// No analysis.include, no rag.*, no code_generation.*, no severity_threshold —
// those were dropped in the Sprint 3 contract, so the form does not expose them.

export function defaultConfigForm() {
  return {
    defaultBranch: 'main',
    excludePaths: [],
    categories: [],
    retentionDays: 30,
  }
}

export function buildYaml(form) {
  const exclude = Array.isArray(form.excludePaths) ? form.excludePaths : []
  const categories = Array.isArray(form.categories) ? form.categories : []
  const lines = []
  lines.push('repository:')
  lines.push(`  default_branch: ${form.defaultBranch || 'main'}`)
  lines.push('')
  lines.push('analysis:')
  lines.push('  enabled: true')
  lines.push('  exclude:')
  if (exclude.length) exclude.forEach((p) => lines.push(`    - ${p}`))
  else lines.push('    []')
  lines.push('')
  lines.push('recommendations:')
  lines.push(`  enabled: ${categories.length ? 'true' : 'false'}`)
  lines.push('  categories:')
  if (categories.length) categories.forEach((c) => lines.push(`    - ${c}`))
  else lines.push('    []')
  lines.push('')
  lines.push('storage:')
  lines.push(`  recommendation_ttl_days: ${form.retentionDays || 30}`)
  return lines.join('\n') + '\n'
}

// Lightweight reverse parse, only good enough to pre-fill the form from a saved
// YAML (the authoritative parser is the Go backend). Reads the keys we write.
export function parseYamlToForm(yaml) {
  const form = defaultConfigForm()
  const text = String(yaml || '')
  const scalar = (key) => {
    const m = text.match(new RegExp(`${key}:\\s*([^\\n]+)`))
    return m ? m[1].trim().replace(/^["']|["']$/g, '') : null
  }
  const listUnder = (header) => {
    const re = new RegExp(`${header}:\\s*\\n((?:\\s*-\\s*[^\\n]+\\n?)+)`)
    const m = text.match(re)
    if (!m) return []
    return m[1].split('\n').map((l) => l.replace(/^\s*-\s*/, '').trim()).filter(Boolean)
  }
  const db = scalar('default_branch')
  if (db) form.defaultBranch = db
  const exc = listUnder('exclude')
  if (exc.length) form.excludePaths = exc
  const cats = listUnder('categories')
  if (cats.length) form.categories = cats
  const ttl = scalar('recommendation_ttl_days')
  if (ttl) form.retentionDays = Number(ttl) || form.retentionDays
  return form
}

// Convenience: the YAML for the default form (used as a starting point).
export const defaultYaml = buildYaml(defaultConfigForm())
