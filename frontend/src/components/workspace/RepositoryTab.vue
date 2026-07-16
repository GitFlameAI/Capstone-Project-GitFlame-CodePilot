<script setup>
// Repository tab. Three equal-width blocks stacked top to bottom:
//   Connection      — details from the landing screen, editable (switch repo/branch/token),
//                     the copyable webhook URL, and a live "GitFlame events" indicator.
//   Files           — a clickable file tree; each file/folder can be excluded from
//                     analysis, which keeps the .ai.yml analysis.exclude list in sync.
//   Recommendations — a short analysis summary, or a prompt to configure / analyse.
import { reactive, ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { session, updateConnection, clearConnection, updateDraftExcludePaths, applyMockPush, configDirty } from '../../store/session.js'
import { api, USING_MOCK } from '../../api/index.js'
import { describeError } from '../../api/errors.js'
import { copyText } from '../../utils/clipboard.js'
import { flattenFiles, computeExcludedSet, toggleFileExclude, toggleFolderExclude } from '../../utils/excludePaths.js'
import GfIcon from '../ui/GfIcon.vue'
import GfButton from '../ui/GfButton.vue'
import GfModal from '../ui/GfModal.vue'
import GfTooltip from '../ui/GfTooltip.vue'
import FileTree from '../FileTree.vue'

const emit = defineEmits(['go', 'reload-repository'])
const router = useRouter()

// --- edit-connection modal (reconnect: replace the token / repo / branch) ---
const editing = ref(false)
const showToken = ref(false)
const savingEdit = ref(false)
const editError = ref(null)
const form = reactive({ url: '', defaultBranch: '', token: '' })

function openEdit() {
  form.url = session.repo.url
  form.defaultBranch = session.repo.defaultBranch
  form.token = ''
  editError.value = null
  editing.value = true
}

// Reconnect always re-validates the token with GitFlame, so a token is required.
async function saveEdit() {
  editError.value = null
  if (!form.url.trim()) { editError.value = { message: 'A repository URL is required.' }; return }
  if (!form.token.trim()) { editError.value = { message: 'An access token is required to re-verify the connection.' }; return }
  savingEdit.value = true
  try {
    const opts = { token: form.token, repoUrl: form.url.trim(), defaultBranch: form.defaultBranch.trim() || 'main' }
    const conn = session.connectionId
      ? await api.reconnectConnection(session.connectionId, opts)
      : await api.createConnection(opts)
    updateConnection(conn)
    emit('reload-repository')
    form.token = ''
    editing.value = false
    loadRecSummary()
  } catch (e) {
    editError.value = describeError(e)
  } finally {
    savingEdit.value = false
  }
}

// --- disconnect (revoke) ---
const disconnecting = ref(false)
async function disconnect() {
  disconnecting.value = true
  try {
    if (session.connectionId) await api.revokeConnection(session.connectionId)
    try { await api.logout() } catch { /* best-effort */ }
  } catch {
    // Even if the backend call fails, clear the local session so the user is not stuck.
  } finally {
    disconnecting.value = false
    clearConnection()
    router.push('/codepilot')
  }
}

// Demo-only helper: simulate an expired session so the reconnect gate is
// demonstrable in mock mode (in live mode this happens when the backend returns
// 401/403). Available only in mock mode.
function simulateExpiry() {
  // markTokenInvalid lives in the store; import lazily to keep this demo-only.
  session.tokenStatus = 'invalid'
  session.tokenError = 'Your session expired (demo).'
}

// --- webhook copy ---
const copiedWebhook = ref(false)
async function copyWebhook() {
  const ok = await copyText(session.repo.webhookUrl)
  if (ok) {
    copiedWebhook.value = true
    setTimeout(() => (copiedWebhook.value = false), 1500)
  }
}

// --- live GitFlame events (mock webhook simulation) ---
const pushToast = ref('')
function simulatePush() {
  const ev = applyMockPush()
  pushToast.value = `Repository updated from GitFlame webhook · commit ${ev.commit}`
  setTimeout(() => (pushToast.value = ''), 3600)
  // A push changes code, so refresh the recommendations summary state.
  loadRecSummary()
}

// --- file tree exclude wiring (edits the config DRAFT; applied on Save in Config) ---
const allFiles = computed(() => flattenFiles(session.fileTree))
const excludedSet = computed(() =>
  computeExcludedSet(allFiles.value, session.configDraft.excludePaths || []),
)
const excludedCount = computed(() => excludedSet.value.size)
const dirty = computed(() => configDirty())

function onTreeToggle({ type, path }) {
  const current = session.configDraft.excludePaths || []
  const next =
    type === 'dir'
      ? toggleFolderExclude(session.fileTree, current, path)
      : toggleFileExclude(session.fileTree, current, path)
  updateDraftExcludePaths(next)
}

// --- recommendations summary block ---
const recSummary = ref('')
const recState = ref('idle') // idle | loading | ready | empty | no_categories
async function loadRecSummary() {
  if (!session.configExists) { recState.value = 'idle'; return }
  // Match the Recommendations tab: with no categories enabled, nothing is analysed.
  if (!(session.configForm.categories || []).length) { recState.value = 'no_categories'; return }
  recState.value = 'loading'
  try {
    const res = await api.getRecommendationSummary(session.repo.id)
    recSummary.value = res.summary
    recState.value = 'ready'
  } catch {
    recState.value = 'empty'
  }
}
onMounted(loadRecSummary)
</script>

<template>
  <div class="repo">
    <!-- Connection -->
    <section class="card gf-card">
      <div class="card__head">
        <h3 class="card__title"><GfIcon name="link" :size="16" /> Connection</h3>
        <GfButton variant="secondary" size="s" @click="openEdit">
          <GfIcon name="pencil" :size="14" /> Change
        </GfButton>
      </div>
      <dl class="info">
        <div><dt>Repository</dt><dd>{{ session.repo.owner }}/{{ session.repo.name }}</dd></div>
        <div><dt>URL</dt><dd class="mono"><a :href="session.repo.url" target="_blank" rel="noopener">{{ session.repo.url }}</a></dd></div>
        <div><dt>Default branch</dt><dd class="mono">{{ session.repo.defaultBranch }}</dd></div>
        <div>
          <dt>Access token</dt>
          <dd class="tokline">
            <span class="mono">{{ session.repo.tokenMasked || '—' }}</span>
            <span
              class="gf-chip tokstatus"
              :class="session.tokenStatus === 'invalid' ? 'tokstatus_bad' : 'tokstatus_ok'"
            >
              {{ session.tokenStatus === 'invalid' ? 'needs reconnect' : 'stored securely' }}
            </span>
          </dd>
        </div>
        <div v-if="session.repo.webhookUrl">
          <dt>Webhook <GfTooltip text="A URL you register in GitFlame. GitFlame calls it when an issue or branch changes, so CodePilot is notified and pulls the updated repository data. It is inbound (GitFlame → CodePilot), separate from your access token." /></dt>
          <dd class="webhookrow">
            <span class="mono webhook">{{ session.repo.webhookUrl }}</span>
            <button class="copybtn" :title="copiedWebhook ? 'Copied' : 'Copy webhook URL'" @click="copyWebhook">
              <GfIcon :name="copiedWebhook ? 'check' : 'copy'" :size="14" />
            </button>
          </dd>
        </div>
      </dl>

      <!-- live webhook indicator + demo trigger -->
      <div class="events">
        <span class="events__live"><span class="events__dot" /> Listening for GitFlame events</span>
        <span v-if="session.lastEvent" class="gf-chip events__last">
          <GfIcon name="branch" :size="12" /> last push {{ session.lastEvent.when }}
        </span>
        <button v-if="USING_MOCK" class="events__sim" title="Demo: simulate a GitFlame push so the tree, issues and recommendations refresh in place" @click="simulatePush">
          <GfIcon name="refresh" :size="13" /> Simulate a push (demo)
        </button>
      </div>

      <!-- connection actions -->
      <div class="connactions">
        <button
          v-if="USING_MOCK"
          class="connactions__demo"
          title="Demo: simulate an expired session so the reconnect prompt appears"
          @click="simulateExpiry"
        >
          <GfIcon name="key" :size="13" /> Simulate expired session (demo)
        </button>
        <button class="connactions__revoke" :disabled="disconnecting" @click="disconnect">
          <GfIcon name="close" :size="13" /> Disconnect repository
        </button>
      </div>
    </section>

    <!-- Files -->
    <section class="card gf-card">
      <div class="card__head">
        <h3 class="card__title"><GfIcon name="folder" :size="16" /> Files</h3>
        <div class="files__head-actions">
          <span v-if="excludedCount" class="gf-chip files__count">
            <GfIcon name="eyeOff" :size="12" /> {{ excludedCount }} excluded
          </span>
          <button v-if="dirty" class="files__unsaved" title="Exclude changes are staged in your draft — review and Save them in the Config tab" @click="emit('go', 'config')">
            <GfIcon name="alert" :size="12" /> Unsaved · review in Config
          </button>
        </div>
      </div>
      <p class="card__sub gf-muted">
        Click <strong>Exclude</strong> on any file or folder to keep CodePilot from analysing it —
        it updates your <span class="mono">.ai.yml</span> instantly. CodePilot reads file
        contents only when it generates a plan.
      </p>
      <p v-if="session.repositoryDataStatus === 'loading'" class="gf-muted">Loading repository files...</p>
      <div v-else-if="session.repositoryDataStatus === 'error'" class="notice">
        <GfIcon name="alert" :size="18" />
        <p>{{ session.repositoryDataError }}</p>
        <GfButton variant="secondary" size="s" @click="emit('reload-repository')">Retry</GfButton>
      </div>
      <p v-else-if="!session.fileTree.length" class="gf-muted">No files were returned for this branch.</p>
      <FileTree
        v-else
        :nodes="session.fileTree"
        interactive
        :excluded-set="excludedSet"
        :all-files="allFiles"
        @toggle="onTreeToggle"
      />
    </section>

    <!-- Recommendations summary -->
    <section class="card gf-card">
      <div class="card__head">
        <h3 class="card__title"><GfIcon name="shield" :size="16" /> Recommendations</h3>
        <!-- Only show the header "Open" when the summary is shown (no body CTA);
             the empty / no-categories states already have their own button. -->
        <GfButton v-if="session.configExists && recState === 'ready'" variant="secondary" size="s" @click="emit('go', 'recommendations')">
          Open
        </GfButton>
      </div>
      <template v-if="session.configExists">
        <p v-if="recState === 'loading'" class="gf-muted">Loading summary…</p>
        <p v-else-if="recState === 'ready'" class="recsum">
          <span class="recsum__kw">Summary: </span>{{ recSummary }}
        </p>
        <div v-else-if="recState === 'no_categories'" class="notice">
          <GfIcon name="info" :size="18" />
          <p>No recommendation categories are enabled in the configuration, so nothing is analysed.</p>
          <GfButton variant="primary" size="s" @click="emit('go', 'config')">Enable categories in Config</GfButton>
        </div>
        <div v-else class="locked locked_soft">
          <p class="gf-muted">No analysis stored yet — CodePilot will run it when you open the tab.</p>
          <GfButton variant="primary" size="s" @click="emit('go', 'recommendations')">Open Recommendations</GfButton>
        </div>
      </template>
      <div v-else class="locked">
        <GfIcon name="lock" :size="18" />
        <p>Save a configuration to unlock recommendations and autogeneration.</p>
        <GfButton variant="primary" size="s" @click="emit('go', 'config')">Go to Config</GfButton>
      </div>
    </section>

    <!-- Edit-connection modal (reconnect) -->
    <GfModal v-if="editing" title="Change connection" subtitle="Switch repository, branch, or token" @close="editing = false">
      <p class="modalnote gf-muted">
        Re-verifying re-checks the access token with GitFlame and replaces the stored token.
        Changing to a different repository clears the saved <span class="mono">.ai.yml</span>
        (configuration is per-repository), so you will set it up again.
      </p>
      <label class="mfield">
        <span class="mfield__label">Repository URL</span>
        <input v-model="form.url" class="minput" placeholder="https://gitflame.ru/owner/repository" />
      </label>
      <label class="mfield">
        <span class="mfield__label">Default branch</span>
        <input v-model="form.defaultBranch" class="minput mono" placeholder="main" />
      </label>
      <label class="mfield">
        <span class="mfield__label">Access token <span class="mfield__opt">required to re-verify</span></span>
        <div class="minput minput_group">
          <GfIcon name="key" :size="15" class="minput__lead" />
          <input v-model="form.token" :type="showToken ? 'text' : 'password'" class="minput__field" placeholder="xxxxxxxxxxxxxxxxxxxx" @keyup.enter="saveEdit" />
          <button type="button" class="minput__toggle" @click="showToken = !showToken">
            <GfIcon :name="showToken ? 'eyeOff' : 'eye'" :size="15" />
          </button>
        </div>
      </label>
      <p v-if="editError" class="medit-err"><GfIcon name="alert" :size="14" /> {{ editError.message }}</p>
      <template #footer>
        <GfButton variant="ghost" :disabled="savingEdit" @click="editing = false">Cancel</GfButton>
        <GfButton variant="primary" :loading="savingEdit" :disabled="!form.url.trim() || !form.token.trim()" @click="saveEdit">Save connection</GfButton>
      </template>
    </GfModal>

    <!-- webhook update toast -->
    <transition name="toastfade">
      <div v-if="pushToast" class="toast">
        <GfIcon name="check" :size="15" /> {{ pushToast }}
      </div>
    </transition>
  </div>
</template>

<style scoped>
.repo {
  display: flex;
  flex-direction: column;
  gap: 18px;
  width: 100%;
  max-width: var(--ws-content);
  margin: 0 auto;
}
.card {
  width: 100%;
  padding: 20px 22px;
}
.card__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 14px;
}
.card__title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 14px;
  font-size: 15px;
}
.card__head .card__title {
  margin: 0;
}
.card__title :deep(.gf-icon) {
  color: var(--gf-purple);
}
.card__sub {
  margin: -6px 0 14px;
  font-size: 12.5px;
  line-height: 1.5;
}
.card__sub strong {
  color: var(--gf-accent);
}
.info {
  display: grid;
  gap: 10px;
  margin: 0;
}
.info > div {
  display: grid;
  grid-template-columns: 130px 1fr;
  gap: 12px;
  align-items: baseline;
}
.info dt {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--gf-text-2);
}
.info dd {
  margin: 0;
  font-size: 13.5px;
  word-break: break-all;
  min-width: 0;
}
.tokline {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.tokstatus {
  height: 20px;
  font-size: 10.5px;
  font-weight: 700;
  border-color: transparent;
}
.tokstatus_ok {
  color: var(--gf-green);
  background: var(--gf-green-bg);
}
.tokstatus_bad {
  color: var(--gf-amber);
  background: var(--gf-amber-bg);
}
.connactions {
  display: flex;
  align-items: center;
  gap: 8px 16px;
  flex-wrap: wrap;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--gf-line);
}
.connactions__demo,
.connactions__revoke {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border: 0;
  background: transparent;
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}
.connactions__demo {
  color: var(--gf-text-3);
}
.connactions__demo:hover {
  color: var(--gf-text);
  text-decoration: underline;
}
.connactions__revoke {
  margin-left: auto;
  color: var(--gf-red);
}
.connactions__revoke:hover {
  text-decoration: underline;
}
.connactions__revoke:disabled {
  opacity: 0.5;
  cursor: default;
}
.medit-err {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 4px 0 0;
  font-size: 12.5px;
  color: var(--gf-red);
}
.medit-err :deep(.gf-icon) {
  flex: none;
}
.webhookrow {
  display: flex;
  align-items: center;
  gap: 8px;
}
.webhook {
  font-size: 12px;
  color: var(--gf-text-2);
  word-break: break-all;
}
.copybtn {
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border: 1px solid var(--gf-line-2);
  border-radius: 7px;
  background: var(--gf-surface);
  color: var(--gf-text-3);
  cursor: pointer;
  flex: none;
}
.copybtn:hover {
  border-color: var(--gf-purple);
  color: var(--gf-accent);
}
.events {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px 14px;
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px solid var(--gf-line);
}
.events__live {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 12px;
  font-weight: 600;
  color: var(--gf-text-2);
}
.events__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--gf-green);
  box-shadow: 0 0 0 0 rgba(0, 177, 78, 0.5);
  animation: pulse 2s infinite;
}
@keyframes pulse {
  0% { box-shadow: 0 0 0 0 rgba(0, 177, 78, 0.45); }
  70% { box-shadow: 0 0 0 6px rgba(0, 177, 78, 0); }
  100% { box-shadow: 0 0 0 0 rgba(0, 177, 78, 0); }
}
.events__last {
  height: 22px;
  font-size: 11px;
  color: var(--gf-text-2);
}
.events__sim {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-left: auto;
  border: 0;
  background: transparent;
  color: var(--gf-accent);
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}
.events__sim:hover {
  text-decoration: underline;
}
.files__head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.files__unsaved {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 22px;
  padding: 0 9px;
  border: 0;
  border-radius: 999px;
  background: var(--gf-amber-bg);
  color: var(--gf-amber);
  font: inherit;
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
}
.files__unsaved:hover {
  text-decoration: underline;
}
.files__count {
  height: 22px;
  font-size: 11px;
  color: var(--gf-accent);
  background: var(--gf-purple-soft);
  border-color: transparent;
}
.recsum {
  margin: 0;
  font-size: 13.5px;
  line-height: 1.6;
  color: var(--gf-text);
}
.recsum__kw {
  color: var(--gf-accent);
  font-weight: 700;
}
.locked {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  text-align: center;
  color: var(--gf-text-2);
  font-size: 13.5px;
  padding: 6px 0;
}
.locked_soft {
  padding: 2px 0;
}
.notice {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  text-align: center;
  color: var(--gf-text-2);
  font-size: 13.5px;
  padding: 2px 0;
}
.notice :deep(.gf-icon) {
  color: var(--gf-purple);
}
.locked :deep(.gf-icon) {
  color: var(--gf-locked);
}

/* modal fields */
.modalnote {
  margin: 0 0 16px;
  font-size: 12.5px;
  line-height: 1.5;
}
.mfield {
  display: block;
  margin-bottom: 14px;
}
.mfield__label {
  display: block;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--gf-text-2);
  margin-bottom: 7px;
}
.mfield__opt {
  margin-left: 6px;
  font-weight: 500;
  color: var(--gf-text-3);
}
.minput {
  width: 100%;
  height: 40px;
  padding: 0 13px;
  border: 1px solid var(--gf-line-2);
  border-radius: 10px;
  font: inherit;
  font-size: 13.5px;
  color: var(--gf-text);
  background: var(--gf-surface);
}
.minput:focus {
  outline: none;
  border-color: var(--gf-purple);
}
.minput_group {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
}
.minput__lead {
  color: var(--gf-text-3);
  flex: none;
}
.minput__field {
  flex: 1;
  height: 100%;
  border: 0;
  outline: 0;
  background: transparent;
  font: inherit;
  font-size: 13.5px;
  color: var(--gf-text);
}
.minput__field::-ms-reveal,
.minput__field::-ms-clear {
  display: none;
}
.minput__toggle {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--gf-text-3);
  cursor: pointer;
  flex: none;
}
.minput__toggle:hover {
  background: var(--gf-surface-3);
  color: var(--gf-text);
}

/* toast */
.toast {
  position: fixed;
  left: 50%;
  bottom: 24px;
  transform: translateX(-50%);
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 11px 16px;
  border-radius: 12px;
  background: var(--gf-text);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  box-shadow: var(--gf-shadow-pop);
  z-index: 1200;
  max-width: calc(100vw - 32px);
}

@media (max-width: 480px) {
  .info > div {
    grid-template-columns: 1fr;
    gap: 3px;
  }
  .card {
    padding: 18px 16px;
  }
}
.toast :deep(.gf-icon) {
  color: #7ee6a6;
}
.toastfade-enter-active,
.toastfade-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.toastfade-enter-from,
.toastfade-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(8px);
}
</style>
