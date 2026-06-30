<script setup>
// Repository tab. Three blocks stacked vertically, top to bottom:
//   Connection      — details from the landing screen, editable (switch repo/branch/token).
//   Files           — a clickable file tree (names + paths only, no content).
//   Recommendations — a short analysis summary, or a prompt to configure / analyse.
import { reactive, ref, onMounted } from 'vue'
import { session, updateConnection } from '../../store/session.js'
import { api, ApiError } from '../../api/index.js'
import GfIcon from '../ui/GfIcon.vue'
import GfButton from '../ui/GfButton.vue'
import GfModal from '../ui/GfModal.vue'
import FileTree from '../FileTree.vue'

const emit = defineEmits(['go'])

// --- edit-connection modal ---
const editing = ref(false)
const showToken = ref(false)
const form = reactive({ url: '', defaultBranch: '', token: '' })

function openEdit() {
  form.url = session.repo.url
  form.defaultBranch = session.repo.defaultBranch
  form.token = ''
  editing.value = true
}
function saveEdit() {
  updateConnection({
    url: form.url.trim(),
    defaultBranch: form.defaultBranch.trim(),
    token: form.token,
  })
  editing.value = false
  loadRecSummary()
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
  } catch (e) {
    recState.value = e instanceof ApiError && e.status === 404 ? 'empty' : 'empty'
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
        <div><dt>Access token</dt><dd class="mono">{{ session.repo.tokenMasked || '—' }}</dd></div>
        <div v-if="session.repo.webhookUrl"><dt>Webhook</dt><dd class="mono webhook">{{ session.repo.webhookUrl }}</dd></div>
      </dl>
    </section>

    <!-- Files -->
    <section class="card gf-card">
      <h3 class="card__title"><GfIcon name="folder" :size="16" /> Files</h3>
      <p class="card__sub gf-muted">
        Names and paths only — hover a file to see its full path. CodePilot reads file
        contents only when it generates a plan.
      </p>
      <FileTree :nodes="session.fileTree" />
    </section>

    <!-- Recommendations summary -->
    <section class="card gf-card">
      <div class="card__head">
        <h3 class="card__title"><GfIcon name="shield" :size="16" /> Recommendations</h3>
        <GfButton v-if="session.configExists" variant="secondary" size="s" @click="emit('go', 'recommendations')">
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
          <p class="gf-muted">No analysis stored yet for this repository.</p>
          <GfButton variant="primary" size="s" @click="emit('go', 'recommendations')">Open Recommendations</GfButton>
        </div>
      </template>
      <div v-else class="locked">
        <GfIcon name="lock" :size="18" />
        <p>Save a configuration to unlock recommendations and autogeneration.</p>
        <GfButton variant="primary" size="s" @click="emit('go', 'config')">Go to Config</GfButton>
      </div>
    </section>

    <!-- Edit-connection modal -->
    <GfModal v-if="editing" title="Change connection" subtitle="Switch repository, branch, or token" @close="editing = false">
      <p class="modalnote gf-muted">
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
        <span class="mfield__label">Access token <span class="mfield__opt">leave blank to keep current</span></span>
        <div class="minput minput_group">
          <GfIcon name="key" :size="15" class="minput__lead" />
          <input v-model="form.token" :type="showToken ? 'text' : 'password'" class="minput__field" placeholder="xxxxxxxxxxxxxxxxxxxx" />
          <button type="button" class="minput__toggle" @click="showToken = !showToken">
            <GfIcon :name="showToken ? 'eyeOff' : 'eye'" :size="15" />
          </button>
        </div>
      </label>
      <template #footer>
        <GfButton variant="ghost" @click="editing = false">Cancel</GfButton>
        <GfButton variant="primary" :disabled="!form.url.trim()" @click="saveEdit">Save connection</GfButton>
      </template>
    </GfModal>
  </div>
</template>

<style scoped>
.repo {
  display: flex;
  flex-direction: column;
  gap: 18px;
  max-width: 820px;
}
.card {
  padding: 20px 22px;
}
.card__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
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
}
.webhook {
  font-size: 12px;
  color: var(--gf-text-2);
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
</style>
