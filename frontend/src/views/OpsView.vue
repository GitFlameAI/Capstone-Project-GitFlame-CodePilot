<script setup>
// Operations screen (route /ops).
//
// A read-only window onto the running deployment: which dependencies answer,
// how deep the queue is, what the recent agent tasks did, and whether anything
// ended up in the dead-letter stream. It exists so that whoever maintains this
// service after the handover can answer "is it healthy, and if not, what broke"
// from a browser, without Prometheus, Grafana or shell access to the VM.
//
// All logic stays in the backend: this component renders /ops/* verbatim.
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../api/index.js'
import { describeError } from '../api/errors.js'
import GfButton from '../components/ui/GfButton.vue'

const REFRESH_INTERVAL_MS = 10000

const status = ref(null)
const tasks = ref([])
const deadLetter = ref({ count: 0, entries: [], detail: '' })
const error = ref(null)
const loading = ref(true)
const lastUpdated = ref(null)
let timer = null

async function refresh() {
  try {
    const [statusResponse, tasksResponse, deadLetterResponse] = await Promise.all([
      api.getOpsStatus(),
      api.getOpsTasks(25),
      api.getOpsDeadLetter(),
    ])
    status.value = statusResponse
    tasks.value = tasksResponse.tasks || []
    deadLetter.value = deadLetterResponse
    lastUpdated.value = new Date()
    error.value = null
  } catch (e) {
    error.value = describeError(e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  refresh()
  timer = setInterval(refresh, REFRESH_INTERVAL_MS)
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})

function formatDuration(milliseconds) {
  if (!milliseconds || milliseconds < 0) return '—'
  if (milliseconds < 1000) return `${milliseconds} ms`
  const seconds = milliseconds / 1000
  if (seconds < 60) return `${seconds.toFixed(1)} s`
  const minutes = Math.floor(seconds / 60)
  return `${minutes} m ${Math.round(seconds % 60)} s`
}

function formatTime(value) {
  if (!value) return '—'
  return new Date(value).toLocaleString()
}

function statusTone(value) {
  if (['ok', 'completed'].includes(value)) return 'ok'
  if (['down', 'failed'].includes(value)) return 'bad'
  if (['degraded', 'processing', 'queued'].includes(value)) return 'warn'
  return 'muted'
}
</script>

<template>
  <main class="ops">
    <header class="ops__header">
      <div>
        <h1>Operations</h1>
        <p class="ops__subtitle">
          Live state of this deployment. Refreshes every 10 seconds.
          <span v-if="lastUpdated"> Last update: {{ lastUpdated.toLocaleTimeString() }}.</span>
        </p>
      </div>
      <GfButton variant="secondary" @click="refresh">Refresh now</GfButton>
    </header>

    <p v-if="loading" class="ops__note">Loading…</p>

    <div v-if="error" class="ops__error">
      <strong>{{ error.title }}</strong>
      <span>{{ error.message }}</span>
    </div>

    <template v-if="status">
      <section class="ops__section">
        <h2>Service</h2>
        <div class="ops__grid">
          <div class="ops__card">
            <span class="ops__label">Overall</span>
            <span class="ops__value" :class="`is-${statusTone(status.status)}`">{{ status.status }}</span>
          </div>
          <div class="ops__card">
            <span class="ops__label">Version</span>
            <span class="ops__value">{{ status.build?.version }}</span>
          </div>
          <div class="ops__card">
            <span class="ops__label">Commit</span>
            <span class="ops__value ops__value--mono">{{ status.build?.commit }}</span>
          </div>
          <div class="ops__card">
            <span class="ops__label">Indexing jobs running</span>
            <span class="ops__value">{{ status.indexing?.running ?? 0 }}</span>
          </div>
        </div>
        <ul v-if="status.warnings?.length" class="ops__warnings">
          <li v-for="warning in status.warnings" :key="warning">{{ warning }}</li>
        </ul>
      </section>

      <section class="ops__section">
        <h2>Dependencies</h2>
        <table class="ops__table">
          <thead>
            <tr><th>Component</th><th>Status</th><th>Detail</th></tr>
          </thead>
          <tbody>
            <tr v-for="dependency in status.dependencies" :key="dependency.component">
              <td>{{ dependency.component }}</td>
              <td><span class="ops__pill" :class="`is-${statusTone(dependency.status)}`">{{ dependency.status }}</span></td>
              <td class="ops__detail">{{ dependency.error || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </section>

      <section v-if="status.queue" class="ops__section">
        <h2>Queue</h2>
        <div class="ops__grid">
          <div class="ops__card">
            <span class="ops__label">Waiting</span>
            <span class="ops__value">{{ status.queue.stream }}</span>
          </div>
          <div class="ops__card">
            <span class="ops__label">In flight</span>
            <span class="ops__value">{{ status.queue.pending }}</span>
          </div>
          <div class="ops__card">
            <span class="ops__label">Dead-letter</span>
            <span class="ops__value" :class="status.queue.dead_letter > 0 ? 'is-bad' : ''">
              {{ status.queue.dead_letter }}
            </span>
          </div>
        </div>
      </section>

      <section class="ops__section">
        <h2>Tasks (last 24 hours)</h2>
        <div class="ops__grid">
          <div v-for="(count, taskStatus) in status.tasks_last_24h" :key="taskStatus" class="ops__card">
            <span class="ops__label">{{ taskStatus }}</span>
            <span class="ops__value" :class="`is-${statusTone(taskStatus)}`">{{ count }}</span>
          </div>
          <p v-if="!Object.keys(status.tasks_last_24h || {}).length" class="ops__note">No tasks in the last 24 hours.</p>
        </div>
      </section>

      <section class="ops__section">
        <h2>GitFlame connections</h2>
        <div class="ops__grid">
          <div v-for="(count, tokenStatus) in status.connections_by_token_status" :key="tokenStatus" class="ops__card">
            <span class="ops__label">token: {{ tokenStatus }}</span>
            <span class="ops__value" :class="tokenStatus === 'active' ? 'is-ok' : 'is-warn'">{{ count }}</span>
          </div>
          <p v-if="!Object.keys(status.connections_by_token_status || {}).length" class="ops__note">No connections stored.</p>
        </div>
      </section>
    </template>

    <section class="ops__section">
      <h2>Recent agent tasks</h2>
      <p v-if="!tasks.length" class="ops__note">No tasks recorded yet.</p>
      <table v-else class="ops__table">
        <thead>
          <tr><th>Created</th><th>Type</th><th>Status</th><th>Attempt</th><th>Duration</th><th>Error</th></tr>
        </thead>
        <tbody>
          <tr v-for="task in tasks" :key="task.id">
            <td>{{ formatTime(task.created_at) }}</td>
            <td>{{ task.task_type }}</td>
            <td><span class="ops__pill" :class="`is-${statusTone(task.status)}`">{{ task.status }}</span></td>
            <td>{{ task.attempt }}</td>
            <td>{{ formatDuration(task.duration_ms) }}</td>
            <td class="ops__detail">{{ task.error_code || '—' }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <section class="ops__section">
      <h2>Dead-letter</h2>
      <p v-if="deadLetter.detail" class="ops__note">{{ deadLetter.detail }}</p>
      <p v-else-if="!deadLetter.entries?.length" class="ops__note">Empty — no task has exhausted its retries.</p>
      <table v-else class="ops__table">
        <thead>
          <tr><th>Task</th><th>Type</th><th>Cause</th></tr>
        </thead>
        <tbody>
          <tr v-for="entry in deadLetter.entries" :key="entry.id">
            <td class="ops__value--mono">{{ entry.task_id || entry.id }}</td>
            <td>{{ entry.task_type || '—' }}</td>
            <td class="ops__detail">{{ entry.error || '—' }}</td>
          </tr>
        </tbody>
      </table>
    </section>
  </main>
</template>

<style scoped>
.ops {
  max-width: var(--ws-content, 860px);
  margin: 0 auto;
  padding: 32px 20px 64px;
  color: var(--gf-text);
}

.ops__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 24px;
}

.ops__header h1 {
  margin: 0 0 4px;
  font-size: 26px;
}

.ops__subtitle {
  margin: 0;
  color: var(--gf-text-3);
  font-size: 13px;
}

.ops__section {
  margin-bottom: 28px;
}

.ops__section h2 {
  margin: 0 0 10px;
  font-size: 15px;
  color: var(--gf-text-2);
}

.ops__grid {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.ops__card {
  min-width: 150px;
  padding: 12px 14px;
  background: var(--gf-surface);
  border: 1px solid var(--gf-line);
  border-radius: var(--gf-radius-sm, 10px);
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.ops__label {
  font-size: 12px;
  color: var(--gf-text-3);
}

.ops__value {
  font-size: 18px;
  font-weight: 600;
}

.ops__value--mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 13px;
}

.is-ok { color: var(--gf-green); }
.is-bad { color: var(--gf-red); }
.is-warn { color: var(--gf-amber); }
.is-muted { color: var(--gf-text-3); }

.ops__table {
  width: 100%;
  border-collapse: collapse;
  background: var(--gf-surface);
  border: 1px solid var(--gf-line);
  border-radius: var(--gf-radius-sm, 10px);
  overflow: hidden;
  font-size: 13px;
}

.ops__table th,
.ops__table td {
  padding: 8px 12px;
  text-align: left;
  border-bottom: 1px solid var(--gf-line);
}

.ops__table th {
  background: var(--gf-surface-3);
  color: var(--gf-text-2);
  font-weight: 600;
}

.ops__table tr:last-child td {
  border-bottom: none;
}

.ops__pill {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--gf-surface-3);
  font-size: 12px;
  font-weight: 600;
}

.ops__pill.is-ok { background: var(--gf-green-bg); color: var(--gf-green); }
.ops__pill.is-bad { background: var(--gf-red-bg); color: var(--gf-red); }
.ops__pill.is-warn { background: var(--gf-amber-bg); color: var(--gf-amber); }

.ops__detail {
  color: var(--gf-text-3);
  word-break: break-word;
}

.ops__note {
  color: var(--gf-text-3);
  font-size: 13px;
  margin: 4px 0;
}

.ops__warnings {
  margin: 10px 0 0;
  padding-left: 18px;
  color: var(--gf-amber);
  font-size: 13px;
}

.ops__error {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px 14px;
  margin-bottom: 20px;
  background: var(--gf-red-bg);
  border-radius: var(--gf-radius-sm, 10px);
  color: var(--gf-red);
  font-size: 13px;
}
</style>
