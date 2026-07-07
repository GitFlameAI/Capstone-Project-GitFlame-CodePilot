// Shared session state for the CodePilot workspace.
//
// Plain `reactive()` singleton — no Pinia, in line with the "boring and reliable"
// rule. It holds what must survive navigation between the landing screen and the
// workspace tabs, and (new in this pass) a lightweight snapshot in sessionStorage
// so a browser refresh restores the workspace instead of dropping to the landing.
//
// Two configuration objects are kept on purpose:
//   - configForm  : the LAST SAVED configuration (drives the real `.ai.yml`,
//                   the tab gating and the recommendation analysis);
//   - configDraft : the WORKING copy edited by the Config form AND the Repository
//                   file-tree Exclude toggles. Edits persist across tab switches
//                   and only touch the saved `.ai.yml` when the user presses Save.
//
//   import { session, connect, saveConfig } from '@/store/session'

import { reactive } from 'vue'
import {
  defaultConfigForm,
  buildYaml,
  parseYamlToForm,
  demoFileTree,
  demoIssues,
  pushedFileNode,
  pushedIssue,
} from '../data/demo.js'

// Where CodePilot is deployed. GitFlame registers the webhook below so branch /
// issue events reach the service; the backend receiver is Arthur's Sprint 4
// endpoint `POST /integrations/gitflame/webhooks/issues`, proxied under /api on
// the VM (see frontend/nginx.conf). Override via VITE_DEPLOY_BASE if needed.
export const DEPLOY_BASE = (import.meta.env.VITE_DEPLOY_BASE || 'http://10.93.27.34').replace(/\/$/, '')
export const WEBHOOK_PATH = '/api/integrations/gitflame/webhooks/issues'

const STORAGE_KEY = 'gfcp.session.v1'
// The raw access token is kept ONLY in memory and is never persisted anywhere.
let liveToken = ''

// Derive a slug repository id (no slashes, safe for the `/repositories/{id}` route
// on the real backend) and a display owner/name from a GitFlame repository URL.
export function parseRepoUrl(url) {
  const fallback = { owner: '', name: '', id: '' }
  if (!url) return fallback
  let path = url.trim()
  try {
    path = new URL(url).pathname
  } catch {
    path = url.replace(/^https?:\/\/[^/]+/i, '')
  }
  const parts = path.split('/').map((p) => p.trim()).filter(Boolean)
  if (parts.length < 2) return fallback
  const owner = parts[0]
  const name = parts[1]
  const id = `${owner}-${name}`.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/(^-|-$)/g, '')
  return { owner, name, id }
}

// The webhook endpoint CodePilot exposes for GitFlame to register. A single
// receiver handles every repository (GitFlame includes the repo in the event
// payload), so it is NOT repo-scoped — matching the backend route exactly.
export function webhookFor(/* id */) {
  return `${DEPLOY_BASE}${WEBHOOK_PATH}`
}

export const session = reactive({
  // --- connection ---
  connected: false,
  intent: 'autogen', // autogen | recommendations — where the workspace opens first
  repo: {
    id: '',
    owner: '',
    name: '',
    url: '',
    defaultBranch: 'main',
    webhookUrl: '',
    tokenMasked: '', // masked hint only; the raw token lives in `liveToken`
  },
  fileTree: [],
  issues: [],

  // --- configuration ---
  configExists: false, // a `.ai.yml` has been saved for this repo (gates AI tabs)
  configYaml: '',
  configForm: defaultConfigForm(), // last SAVED config
  configDraft: defaultConfigForm(), // working draft (Config form + tree edits)
  configSavedAt: '',

  // --- access token status (point: explicit token problems) ---
  tokenStatus: 'ok', // ok | missing | invalid
  tokenError: '',

  // --- cross-tab handoff ---
  pendingIssue: null,

  // --- live GitFlame webhook events (mock simulation) ---
  lastEvent: null,
  recommendationsStale: false,
})

// ---------------------------------------------------------------------------
// Persistence (sessionStorage). The token is deliberately excluded, and the file
// tree / issues are re-derived on rehydrate rather than stored.
// ---------------------------------------------------------------------------
function persist() {
  try {
    const snapshot = {
      connected: session.connected,
      intent: session.intent,
      repo: { ...session.repo, tokenMasked: '' },
      configExists: session.configExists,
      configYaml: session.configYaml,
      configForm: session.configForm,
      configDraft: session.configDraft,
      configSavedAt: session.configSavedAt,
    }
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(snapshot))
  } catch {
    // sessionStorage may be unavailable (private mode); non-critical.
  }
}

function rehydrate() {
  let snapshot = null
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    if (raw) snapshot = JSON.parse(raw)
  } catch {
    snapshot = null
  }
  if (!snapshot || !snapshot.connected) return
  session.intent = snapshot.intent || 'autogen'
  Object.assign(session.repo, snapshot.repo || {})
  session.repo.tokenMasked = ''
  session.configExists = !!snapshot.configExists
  session.configYaml = snapshot.configYaml || ''
  session.configForm = { ...defaultConfigForm(), ...(snapshot.configForm || {}) }
  session.configDraft = { ...defaultConfigForm(), ...(snapshot.configDraft || snapshot.configForm || {}) }
  session.configSavedAt = snapshot.configSavedAt || ''
  // Re-derive demo data (not persisted).
  session.fileTree = demoFileTree(session.configExists)
  session.issues = demoIssues.map((i) => ({ ...i }))
  session.connected = true
  // The raw token is gone after a refresh — require the user to re-enter it.
  liveToken = ''
  session.tokenStatus = 'missing'
  session.tokenError = ''
}

// ---------------------------------------------------------------------------
// Connection
// ---------------------------------------------------------------------------
export function connect({ url, owner, name, id, defaultBranch, token, webhookUrl, intent }) {
  session.repo.url = url
  session.repo.owner = owner
  session.repo.name = name
  session.repo.id = id
  session.repo.defaultBranch = defaultBranch || 'main'
  session.repo.webhookUrl = webhookUrl || webhookFor(id)
  session.intent = intent || 'autogen'
  session.fileTree = demoFileTree(session.configExists)
  session.issues = demoIssues.map((i) => ({ ...i }))
  session.connected = true
  session.lastEvent = null
  session.recommendationsStale = false
  session.configForm.defaultBranch = session.repo.defaultBranch
  session.configDraft.defaultBranch = session.repo.defaultBranch
  setToken(token)
  persist()
}

// Change the connected repository (or its branch / token) from the Repository tab.
export function updateConnection({ url, defaultBranch, token }) {
  const r = parseRepoUrl(url)
  const idChanged = !!r.id && r.id !== session.repo.id
  session.repo.url = url
  if (r.owner) session.repo.owner = r.owner
  if (r.name) session.repo.name = r.name
  if (r.id) session.repo.id = r.id
  session.repo.defaultBranch = defaultBranch || session.repo.defaultBranch || 'main'
  session.repo.webhookUrl = webhookFor(session.repo.id)
  session.configForm.defaultBranch = session.repo.defaultBranch
  session.configDraft.defaultBranch = session.repo.defaultBranch
  if (token) setToken(token)
  if (idChanged) {
    session.configExists = false
    session.configYaml = ''
    session.configSavedAt = ''
    session.configForm = defaultConfigForm()
    session.configDraft = defaultConfigForm()
    session.configForm.defaultBranch = session.repo.defaultBranch
    session.configDraft.defaultBranch = session.repo.defaultBranch
    session.pendingIssue = null
    session.issues = demoIssues.map((i) => ({ ...i }))
    session.recommendationsStale = false
  }
  session.fileTree = demoFileTree(session.configExists)
  persist()
  return { idChanged }
}

// ---------------------------------------------------------------------------
// Access token
// ---------------------------------------------------------------------------
export function setToken(token) {
  liveToken = token || ''
  session.repo.tokenMasked = maskToken(liveToken)
  session.tokenStatus = liveToken ? 'ok' : 'missing'
  session.tokenError = ''
}
export function getToken() {
  return liveToken
}
export function hasToken() {
  return !!liveToken
}
// Called when a live backend call reports the GitFlame token is expired/invalid.
export function markTokenInvalid(message) {
  session.tokenStatus = 'invalid'
  session.tokenError = message || 'The access token is invalid or has expired.'
}

// ---------------------------------------------------------------------------
// Configuration draft / save
// ---------------------------------------------------------------------------
// True when the working draft differs from the last saved configuration.
export function configDirty() {
  return JSON.stringify(normaliseConfig(session.configDraft)) !== JSON.stringify(normaliseConfig(session.configForm))
}

// Commit the working draft: this is the only place the saved `.ai.yml` changes.
export function saveConfig(form) {
  const clean = normaliseConfig(form || session.configDraft)
  session.configForm = clean
  session.configDraft = { ...clean, excludePaths: [...clean.excludePaths], categories: [...clean.categories] }
  session.configYaml = buildYaml(clean)
  session.configExists = true
  session.configSavedAt = new Date().toLocaleString()
  session.fileTree = demoFileTree(true) // `.ai.yml` now appears in the tree
  persist()
}

// Persist the current draft/session snapshot (called after in-place draft edits).
export function persistDraft() {
  persist()
}

// Update only the draft exclude paths (from the Repository file tree). Does NOT
// touch the saved `.ai.yml`; the user applies changes with Save in the Config tab.
export function updateDraftExcludePaths(paths) {
  session.configDraft.excludePaths = [...paths]
  persist()
}

// Load an existing configuration (e.g. if GitFlame reports one already in the repo).
export function loadExistingConfig(yaml) {
  const form = parseYamlToForm(yaml)
  session.configYaml = yaml
  session.configForm = form
  session.configDraft = { ...form, excludePaths: [...form.excludePaths], categories: [...form.categories] }
  session.configExists = true
  session.fileTree = demoFileTree(true)
  persist()
}

function normaliseConfig(form) {
  return {
    defaultBranch: form.defaultBranch || 'main',
    excludePaths: [...(form.excludePaths || [])],
    categories: [...(form.categories || [])],
    retentionDays: form.retentionDays || 30,
  }
}

// ---------------------------------------------------------------------------
// Mock GitFlame webhook / push simulation
// ---------------------------------------------------------------------------
export function applyMockPush() {
  const commit = Math.random().toString(16).slice(2, 9)
  const tree = demoFileTree(session.configExists)
  const backend = tree.find((n) => n.name === 'backend')
  const internal = backend?.children?.find((n) => n.name === 'internal')
  if (internal) internal.children.push(pushedFileNode(commit))
  session.fileTree = tree
  if (!session.issues.some((i) => i.id === 'ISSUE-204')) {
    session.issues = [pushedIssue(), ...session.issues]
  }
  session.recommendationsStale = true
  session.lastEvent = { kind: 'push', commit, when: new Date().toLocaleTimeString() }
  return session.lastEvent
}

function maskToken(token) {
  if (!token) return ''
  const tail = token.slice(-4)
  return `••••••${tail}`
}

// Restore any persisted session as soon as the module loads, before the router
// evaluates the workspace guard — so a refresh stays in the workspace.
rehydrate()
