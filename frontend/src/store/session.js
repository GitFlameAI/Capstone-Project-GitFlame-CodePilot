// Shared session state for the CodePilot workspace.
//
// Plain `reactive()` singleton — no Pinia, in line with the "boring and reliable"
// rule. It holds what must survive navigation between the landing screen and the
// workspace tabs, plus a lightweight snapshot in sessionStorage so a browser
// refresh restores the workspace.
//
// Sprint 5 — secure connection model:
//   The frontend no longer owns the GitFlame access token. The user enters it
//   once on the landing screen; the backend validates it, stores it encrypted,
//   and returns only connection METADATA plus an HttpOnly `codepilot_session`
//   cookie. From then on the frontend authenticates with that cookie and keeps
//   only the metadata (connection id, repository, token_last4, token_status).
//   The raw token is never held in JS state or storage — not even in memory.
//
// Two configuration objects are kept on purpose:
//   - configForm  : the LAST SAVED configuration (drives the real `.ai.yml`,
//                   the tab gating and the recommendation analysis);
//   - configDraft : the WORKING copy edited by the Config form AND the Repository
//                   file-tree Exclude toggles.
//
//   import { session, connect, saveConfig } from '@/store/session'

import { reactive } from 'vue'
import {
  defaultConfigForm,
  buildYaml,
  parseYamlToForm,
  demoFileTree,
  pushedFileNode,
  pushedIssue,
} from '../data/demo.js'

const STORAGE_KEY = 'gfcp.session.v2'

// Parse a GitFlame repository URL into { owner, name, id, url }.
//   https://gitflametest.ru/owner/name        -> owner/name
//   https://gitflametest.ru/owner/name/code    -> owner/name  (/code stripped)
// The `id` here (owner/name) is only a FALLBACK sent to the backend; the
// authoritative repository id always comes back in the connection response.
export function parseRepoUrl(url) {
  const fallback = { owner: '', name: '', id: '', url: url || '' }
  if (!url) return fallback
  let path = url.trim()
  try {
    path = new URL(url).pathname
  } catch {
    path = url.replace(/^https?:\/\/[^/]+/i, '')
  }
  const parts = path.split('/').map((p) => p.trim()).filter(Boolean)
  // Drop a trailing GitFlame sub-page segment (e.g. /code, /tree/main, /issues).
  const SUBPAGES = new Set(['code', 'tree', 'issues', 'pulls', 'wiki', 'settings', 'blob'])
  while (parts.length > 2 && SUBPAGES.has(parts[2])) parts.splice(2)
  if (parts.length < 2) return fallback
  const owner = parts[0]
  const name = parts[1]
  return { owner, name, id: `${owner}/${name}`, url: url.trim() }
}

export const session = reactive({
  // --- connection ---
  connected: false,
  connectionId: '', // GitFlame connection id (for PUT reconnect / DELETE revoke)
  intent: 'autogen', // autogen | recommendations — where the workspace opens first
  repo: {
    id: '', // backend repository id (authoritative), e.g. "owner/name"
    owner: '',
    name: '',
    url: '',
    defaultBranch: 'main',
    webhookUrl: '',
    tokenLast4: '', // last 4 chars of the token, for display only
    tokenMasked: '', // "••••••978f" derived from tokenLast4
  },
  fileTree: [],
  issues: [],
  repositoryDataStatus: 'idle', // idle | loading | ready | error
  repositoryDataError: '',

  // --- configuration ---
  configExists: false, // a `.ai.yml` has been saved for this repo (gates AI tabs)
  configYaml: '',
  configForm: defaultConfigForm(), // last SAVED config
  configDraft: defaultConfigForm(), // working draft (Config form + tree edits)
  configSavedAt: '',

  // --- session / token status (point: explicit reconnect prompts) ---
  // 'active'  : the connection is usable (cookie valid, token stored on backend)
  // 'invalid' : a call reported 401/403 or a gitflame_* connection problem
  tokenStatus: 'active',
  tokenError: '',

  // --- cross-tab handoff ---
  pendingIssue: null,

  // --- optional GitFlame webhook ---
  webhook: { id: '', url: '', status: 'disabled', secret: '' },
  webhookEvents: {},
  lastEvent: null,
  issuePrompt: null,
  recommendationsStale: false,
  recommendationsRevision: 0,
})

// ---------------------------------------------------------------------------
// Persistence (sessionStorage). Only connection METADATA is stored — never the
// token (there is none on the frontend) and never the session cookie (it is
// HttpOnly and owned by the browser). The file tree / issues are re-derived.
// ---------------------------------------------------------------------------
function persist() {
  try {
    const snapshot = {
      connected: session.connected,
      connectionId: session.connectionId,
      intent: session.intent,
      repo: { ...session.repo },
      webhook: { id: session.webhook.id, url: session.webhook.url, status: session.webhook.status },
      webhookEvents: { ...session.webhookEvents },
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
  session.connectionId = snapshot.connectionId || ''
  session.intent = snapshot.intent || 'autogen'
  Object.assign(session.repo, snapshot.repo || {})
  Object.assign(session.webhook, snapshot.webhook || {})
  session.webhookEvents = snapshot.webhookEvents || {}
  session.repo.tokenMasked = maskLast4(session.repo.tokenLast4)
  session.configExists = !!snapshot.configExists
  session.configYaml = snapshot.configYaml || ''
  session.configForm = { ...defaultConfigForm(), ...(snapshot.configForm || {}) }
  session.configDraft = { ...defaultConfigForm(), ...(snapshot.configDraft || snapshot.configForm || {}) }
  session.configSavedAt = snapshot.configSavedAt || ''
  session.fileTree = []
  session.issues = []
  session.connected = true
  // The HttpOnly session cookie survives a refresh, so we assume the connection
  // is still usable. If a subsequent call reports 401/403, markTokenInvalid()
  // flips this to 'invalid' and the workspace shows the reconnect gate.
  session.tokenStatus = 'active'
  session.tokenError = ''
}

// ---------------------------------------------------------------------------
// Connection — driven by the backend connection response.
// ---------------------------------------------------------------------------
// Map a backend GitFlameConnection response onto the session repo metadata.
// Works identically in mock and live mode (the mock returns the same shape).
function applyConnectionResponse(conn) {
  const repository = conn.repository || {}
  const url = conn.repo_url || repository.web_url || ''
  const parsed = parseRepoUrl(url)
  session.connectionId = conn.id || session.connectionId || ''
  session.repo.id = repository.id || parsed.id || session.repo.id
  // Prefer a display owner/name parsed from the URL; fall back to the repo id.
  session.repo.owner = parsed.owner || ownerFromId(session.repo.id)
  session.repo.name = repository.name || parsed.name || nameFromId(session.repo.id)
  session.repo.url = url || session.repo.url
  session.repo.defaultBranch = conn.default_branch || repository.default_branch || session.repo.defaultBranch || 'main'
  session.repo.webhookUrl = session.webhook.url || ''
  session.repo.tokenLast4 = conn.token_last4 || ''
  session.repo.tokenMasked = maskLast4(session.repo.tokenLast4)
  session.tokenStatus = conn.token_status && conn.token_status !== 'active' ? 'invalid' : 'active'
  session.tokenError = ''
}

function ownerFromId(id) {
  return id && id.includes('/') ? id.split('/')[0] : id || ''
}
function nameFromId(id) {
  return id && id.includes('/') ? id.split('/').slice(1).join('/') : id || ''
}

// First-time connect (from the landing screen), after api.createConnection().
// `conn` is the backend connection response; `intent` picks the first AI tab.
export function connect(conn, { intent } = {}) {
  applyConnectionResponse(conn)
  session.intent = intent || 'autogen'
  session.connected = true
  session.lastEvent = null
  session.issuePrompt = null
  session.recommendationsStale = false
  session.recommendationsRevision = 0
  session.fileTree = []
  session.issues = []
  session.repositoryDataStatus = 'idle'
  session.repositoryDataError = ''
  session.configForm.defaultBranch = session.repo.defaultBranch
  session.configDraft.defaultBranch = session.repo.defaultBranch
  persist()
}

// Apply a reconnect / repository-change response (from the Repository tab or the
// reconnect gate). If the repository id changed, the per-repository .ai.yml is
// cleared so the user re-configures the new repository.
export function updateConnection(conn) {
  const previousId = session.repo.id
  applyConnectionResponse(conn)
  const idChanged = !!session.repo.id && session.repo.id !== previousId
  if (idChanged) {
    session.configExists = false
    session.configYaml = ''
    session.configSavedAt = ''
    session.configForm = defaultConfigForm()
    session.configDraft = defaultConfigForm()
    session.configForm.defaultBranch = session.repo.defaultBranch
    session.configDraft.defaultBranch = session.repo.defaultBranch
    session.pendingIssue = null
    resetWebhookState()
    session.issues = []
    session.recommendationsStale = false
    session.recommendationsRevision = 0
  }
  session.fileTree = []
  session.repositoryDataStatus = 'idle'
  session.repositoryDataError = ''
  persist()
  return { idChanged }
}

// Fully clear the connection (logout / revoke). Returns the user to a clean slate.
export function clearConnection() {
  session.connected = false
  session.connectionId = ''
  session.repo = {
    id: '', owner: '', name: '', url: '', defaultBranch: 'main',
    webhookUrl: '', tokenLast4: '', tokenMasked: '',
  }
  session.configExists = false
  session.configYaml = ''
  session.configForm = defaultConfigForm()
  session.configDraft = defaultConfigForm()
  session.configSavedAt = ''
  session.tokenStatus = 'active'
  session.tokenError = ''
  session.pendingIssue = null
  resetWebhookState()
  session.recommendationsStale = false
  session.recommendationsRevision = 0
  session.fileTree = []
  session.issues = []
  session.repositoryDataStatus = 'idle'
  session.repositoryDataError = ''
  try {
    sessionStorage.removeItem(STORAGE_KEY)
  } catch {
    // ignore
  }
}

export function setWebhookRegistration(registration) {
  session.webhook.id = registration?.id || ''
  session.webhook.url = registration?.webhook_url || ''
  session.webhook.status = registration?.status || 'disabled'
  session.webhook.secret = registration?.secret || ''
  session.repo.webhookUrl = session.webhook.url
  persist()
}

export function resetWebhookState() {
  session.webhook = { id: '', url: '', status: 'disabled', secret: '' }
  session.webhookEvents = {}
  session.lastEvent = null
  session.issuePrompt = null
  if (session.repo) session.repo.webhookUrl = ''
}

export function rememberWebhookEvent(event) {
  const previous = session.webhookEvents[event.id]
  session.webhookEvents[event.id] = event.status
  const rememberedIDs = Object.keys(session.webhookEvents)
  for (const id of rememberedIDs.slice(0, Math.max(0, rememberedIDs.length - 100))) {
    delete session.webhookEvents[id]
  }
  session.lastEvent = {
    id: event.id,
    kind: event.event_type,
    action: event.action,
    status: event.status,
    when: new Date(event.received_at || Date.now()).toLocaleTimeString(),
  }
  if (!previous || previous !== event.status) persist()
  return { isNew: !previous, statusChanged: !!previous && previous !== event.status }
}

// ---------------------------------------------------------------------------
// Session / token status
// ---------------------------------------------------------------------------
// Called when a live backend call reports the session/token is expired/invalid
// (401/403 or a gitflame_* connection code). Triggers the reconnect gate.
export function markTokenInvalid(message) {
  session.tokenStatus = 'invalid'
  session.tokenError = message || 'Your session or access token is invalid or has expired.'
}
// Called after a successful reconnect.
export function markConnected() {
  session.tokenStatus = 'active'
  session.tokenError = ''
}
export function hasConnection() {
  return session.connected && !!session.connectionId
}

export function beginRepositoryDataLoad() {
  session.repositoryDataStatus = 'loading'
  session.repositoryDataError = ''
}

export function setRepositoryData(fileTree, issues) {
  session.fileTree = fileTree
  session.issues = issues
  session.repositoryDataStatus = 'ready'
  session.repositoryDataError = ''
}

export function failRepositoryDataLoad(message) {
  session.repositoryDataStatus = 'error'
  session.repositoryDataError = message || 'Repository data could not be loaded.'
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
  ensureConfigFile()
  persist()
}

// Persist the current draft/session snapshot (called after in-place draft edits).
export function persistDraft() {
  persist()
}

// Discard all unsaved edits: reset the working draft back to the saved config.
export function resetDraft() {
  const saved = session.configForm
  session.configDraft = {
    ...saved,
    excludePaths: [...(saved.excludePaths || [])],
    categories: [...(saved.categories || [])],
  }
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
  ensureConfigFile()
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

function ensureConfigFile() {
  if (!session.fileTree.some((node) => node.type === 'file' && node.name === '.ai.yml')) {
    session.fileTree.push({ type: 'file', name: '.ai.yml', badge: 'CodePilot' })
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
    const issue = pushedIssue()
    session.issues = [issue, ...session.issues]
    session.issuePrompt = issue
  }
  session.recommendationsStale = true
  session.lastEvent = { kind: 'push', commit, when: new Date().toLocaleTimeString() }
  return session.lastEvent
}

function maskLast4(last4) {
  if (!last4) return ''
  return `••••••${last4}`
}

// Restore any persisted session as soon as the module loads, before the router
// evaluates the workspace guard — so a refresh stays in the workspace.
rehydrate()
