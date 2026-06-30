// Shared session state for the CodePilot workspace.
//
// This is a plain `reactive()` object exported as a singleton — no Pinia, no new
// framework, in line with the project's "boring and reliable" rule. It holds the
// state that has to survive navigation between the landing screen and the
// workspace tabs:
//   - the repository connection the user entered on the landing screen;
//   - whether a `.ai.yml` exists for the repository (this gates the
//     Autogeneration and Recommendations tabs);
//   - the configuration form values and the YAML that was last saved.
//
// Components import the singleton and read/update it directly:
//   import { session, connect, saveConfig } from '@/store/session'

import { reactive } from 'vue'
import { defaultConfigForm, buildYaml, parseYamlToForm, demoFileTree } from '../data/demo.js'

// Derive a slug repository id (no slashes, safe for the `/repositories/{id}` route
// on the real backend) and a display owner/name from a GitFlame repository URL.
export function parseRepoUrl(url) {
  const fallback = { owner: '', name: '', id: '' }
  if (!url) return fallback
  let path = url.trim()
  try {
    path = new URL(url).pathname
  } catch {
    // Not a full URL — treat the whole string as a path.
    path = url.replace(/^https?:\/\/[^/]+/i, '')
  }
  const parts = path.split('/').map((p) => p.trim()).filter(Boolean)
  if (parts.length < 2) return fallback
  const owner = parts[0]
  const name = parts[1]
  const id = `${owner}-${name}`.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/(^-|-$)/g, '')
  return { owner, name, id }
}

// The webhook endpoint CodePilot exposes for a repository. GitFlame registers this
// URL so issue / issue-comment events (approve / correct / reject) reach the service.
export function webhookFor(id) {
  return `https://codepilot.gitflame.ru/api/v1/gitflame/events/${id || 'your-repo'}`
}

export const session = reactive({
  // --- connection (filled on the landing screen) ---
  connected: false,
  intent: 'autogen', // autogen | recommendations — chosen on the landing screen
  repo: {
    id: '',
    owner: '',
    name: '',
    url: '',
    defaultBranch: 'main',
    webhookUrl: '',
    tokenMasked: '', // never store the real token; only a masked hint for display
  },
  fileTree: [],

  // --- configuration ---
  configExists: false,
  configYaml: '',
  configForm: defaultConfigForm(),
  configSavedAt: '',

  // --- cross-tab handoff ---
  // Set by the Recommendations tab when the user turns a recommendation into an
  // issue; the Autogeneration tab reads it on mount to pre-fill the issue form.
  pendingIssue: null,
})

// Called from the landing screen once the connect form validates.
export function connect({ url, owner, name, id, defaultBranch, token, webhookUrl, intent }) {
  session.repo.url = url
  session.repo.owner = owner
  session.repo.name = name
  session.repo.id = id
  session.repo.defaultBranch = defaultBranch || 'main'
  session.repo.webhookUrl = webhookUrl || webhookFor(id)
  session.repo.tokenMasked = maskToken(token)
  session.intent = intent || 'autogen'
  session.fileTree = demoFileTree(session.configExists)
  session.connected = true
  // The configuration form follows the connected default branch.
  session.configForm.defaultBranch = session.repo.defaultBranch
}

// Change the connected repository (or its branch / token) from the Repository tab.
// A `.ai.yml` is repository-specific, so if the repository id changes the saved
// configuration is cleared and the AI tabs re-lock until a new one is saved.
export function updateConnection({ url, defaultBranch, token }) {
  const r = parseRepoUrl(url)
  const idChanged = !!r.id && r.id !== session.repo.id
  session.repo.url = url
  if (r.owner) session.repo.owner = r.owner
  if (r.name) session.repo.name = r.name
  if (r.id) session.repo.id = r.id
  session.repo.defaultBranch = defaultBranch || session.repo.defaultBranch || 'main'
  session.repo.webhookUrl = webhookFor(session.repo.id)
  if (token) session.repo.tokenMasked = maskToken(token)
  session.configForm.defaultBranch = session.repo.defaultBranch
  if (idChanged) {
    session.configExists = false
    session.configYaml = ''
    session.configSavedAt = ''
    session.pendingIssue = null
  }
  session.fileTree = demoFileTree(session.configExists)
  return { idChanged }
}

// Persisting the configuration "saves .ai.yml to the default branch" (mocked) and
// unlocks the Autogeneration and Recommendations tabs.
export function saveConfig(form) {
  session.configForm = { ...form }
  session.configYaml = buildYaml(form)
  session.configExists = true
  session.configSavedAt = new Date().toLocaleString()
  session.fileTree = demoFileTree(true) // .ai.yml now appears in the tree
}

// Load an existing configuration (e.g. if GitFlame reports one already in the repo).
export function loadExistingConfig(yaml) {
  session.configYaml = yaml
  session.configForm = parseYamlToForm(yaml)
  session.configExists = true
  session.fileTree = demoFileTree(true)
}

function maskToken(token) {
  if (!token) return ''
  const tail = token.slice(-4)
  return `••••••${tail}`
}
