<script setup>
// Service landing screen (route /codepilot).
//
// Layout follows the brief top-to-bottom:
//   1. Hero: big service name + short description + "Work with AI" button that
//      scrolls down to the connect form.
//   2. Intent toggle explaining the two capabilities (Autogeneration vs
//      Recommendations) — the same toggle style used elsewhere in the app.
//   3. Connect form: repository URL, default branch, access token, and an
//      advanced webhook URL. Gray placeholder examples, "i-in-circle" hints.
//   4. AI disclaimer + policy consent checkboxes.
//   5. Continue: validates; empty required fields / unchecked boxes get a red
//      underline and navigation is blocked. On success it connects and opens the
//      workspace.
import { reactive, ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { session, connect, parseRepoUrl, webhookFor } from '../store/session.js'
import { api } from '../api/index.js'
import { describeError } from '../api/errors.js'
import { copyText } from '../utils/clipboard.js'
import GfIcon from '../components/ui/GfIcon.vue'
import GfButton from '../components/ui/GfButton.vue'
import GfTooltip from '../components/ui/GfTooltip.vue'
import GfModal from '../components/ui/GfModal.vue'
import Roadmap from '../components/landing/Roadmap.vue'
import TryDemo from '../components/landing/TryDemo.vue'

const router = useRouter()
const formEl = ref(null)

// The connect form starts empty in every mode, as a real user would experience it.
// The token and default branch are always empty to exercise validation / avoid
// pre-filling assumptions.
const form = reactive({
  repoUrl: session.repo.url || '',
  defaultBranch: '',
  token: '',
})
const showAdvanced = ref(false)
const showToken = ref(false)
const showPolicy = ref(false)
const copied = ref(false)

// Connection request state. The token is submitted once to the backend and then
// cleared from the form — the frontend never keeps the raw GitFlame token.
const connecting = ref(false)
const connectError = ref(null) // { title, message } from describeError

// The webhook URL is something OUR service exposes for GitFlame to register; it
// is shown read-only so the user can copy it.
function webhookUrl() {
  return webhookFor(parseRepoUrl(form.repoUrl).id)
}

// A single consent: agreeing to the usage policy (which itself covers the
// "AI output may be wrong — trust, but verify" point and how the repo/token are used).
const consent = reactive({ policy: false })

// Per-field error flags drive the red underline.
const errors = reactive({ repoUrl: false, token: false, policy: false })

// Whether every required field is filled. Drives the dimmed look of the Continue
// button — it stays clickable (a click reveals the red validation), it just looks
// inactive until the form is complete.
const formComplete = computed(() => {
  const r = parseRepoUrl(form.repoUrl)
  return !!form.repoUrl.trim() && !!r.owner && !!r.name && !!form.token.trim() && consent.policy
})

function validate() {
  const r = parseRepoUrl(form.repoUrl)
  errors.repoUrl = !form.repoUrl.trim() || !r.owner || !r.name
  errors.token = !form.token.trim()
  errors.policy = !consent.policy
  return !errors.repoUrl && !errors.token && !errors.policy
}

function scrollToForm() {
  formEl.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

async function copyWebhook() {
  const ok = await copyText(webhookUrl())
  if (ok) {
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  }
}

async function submit() {
  connectError.value = null
  if (!validate()) {
    // Jump to the first offending field group so the red underline is visible.
    formEl.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    return
  }
  const r = parseRepoUrl(form.repoUrl)
  connecting.value = true
  try {
    // One-time secure handshake: the backend validates the token via GitFlame,
    // creates the app user + server session (HttpOnly codepilot_session cookie),
    // stores the token encrypted, and returns connection metadata only.
    const conn = await api.createConnection({
      token: form.token,
      repoUrl: form.repoUrl.trim(),
      repository: {
        id: r.id,
        name: r.name,
        default_branch: form.defaultBranch.trim() || 'main',
        web_url: form.repoUrl.trim(),
      },
      defaultBranch: form.defaultBranch.trim() || 'main',
    })
    // Store ONLY the returned metadata; never the token.
    connect(conn)
    // Wipe the token from the form so it does not linger in component state.
    form.token = ''
    router.push('/workspace')
  } catch (e) {
    const info = describeError(e)
    connectError.value = info
    // Highlight the token field for an auth problem, the URL field otherwise.
    if (info.tokenProblem) errors.token = true
    else if (info.kind === 'validation') errors.repoUrl = true
    formEl.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  } finally {
    connecting.value = false
  }
}
</script>

<template>
  <div class="land">
    <!-- top brand bar -->
    <div class="topbar">
      <div class="topbar__inner">
        <span class="brand">
          <span class="brand__mark"><GfIcon name="sparkles" :size="16" /></span>
          CodePilot
        </span>
        <a class="gf-chip topbar__link" href="https://gitflame.ru" target="_blank" rel="noopener">for GitFlame</a>
      </div>
    </div>

    <!-- 1. Hero -->
    <section class="hero">
      <a class="hero__eyebrow" href="https://gitflame.ru" target="_blank" rel="noopener">AI integration for GitFlame <GfIcon name="external" :size="12" /></a>
      <h1 class="hero__title">GitFlame CodePilot</h1>
      <p class="hero__desc">
        Turn issues into reviewable implementation plans and generated code, and get
        continuous optimization recommendations for your repository — all under your
        approval.
      </p>
      <GfButton variant="primary" size="l" @click="scrollToForm">
        <GfIcon name="sparkles" :size="17" /> Work with AI
      </GfButton>
    </section>

    <!-- 2. How it works: roadmap + interactive preview -->
    <section class="how">
      <div class="how__intro">
        <h2 class="how__title">How it works</h2>
        <p class="how__sub gf-muted">From a repository issue to reviewed, generated code — and continuous recommendations. All under your approval.</p>
      </div>
      <Roadmap />

      <div class="how__try">
        <h3 class="how__try-title">Try it yourself</h3>
        <p class="how__sub gf-muted">A quick, hands-on preview — click through the flow. Nothing is saved.</p>
        <TryDemo />
      </div>
    </section>

    <!-- 3 + 4 + 5. Connect form -->
    <section ref="formEl" class="connect">
      <header class="connect__head">
        <h2 class="connect__title">Connect a repository</h2>
        <p class="connect__sub gf-muted">Fill these in so CodePilot can reach your repository.</p>
      </header>
      <div class="connect__card gf-card">
        <label class="field" :class="{ field_error: errors.repoUrl }">
          <span class="field__label">
            Repository URL
            <GfTooltip text="The GitFlame repository CodePilot should work with, e.g. https://gitflame.ru/owner/name" />
          </span>
          <input
            v-model="form.repoUrl"
            class="input"
            placeholder="https://gitflame.ru/owner/repository"
            @input="errors.repoUrl = false"
          />
          <span v-if="errors.repoUrl" class="field__msg">Enter a valid repository URL (owner/name).</span>
        </label>

        <label class="field">
          <span class="field__label">
            Default branch
            <span class="field__opt">optional</span>
            <GfTooltip text="Branch CodePilot reads from and saves the .ai.yml to. Defaults to main." />
          </span>
          <input v-model="form.defaultBranch" class="input" placeholder="main" />
        </label>

        <label class="field" :class="{ field_error: errors.token }">
          <span class="field__label">
            Access token
            <GfTooltip text="A GitFlame access token so CodePilot can read files and open a branch / pull request on your behalf. It is sent once to CodePilot, stored encrypted on the server, and never kept in your browser." />
          </span>
          <div class="input input_group">
            <GfIcon name="key" :size="15" class="input__lead" />
            <input
              v-model="form.token"
              :type="showToken ? 'text' : 'password'"
              class="input__field"
              placeholder="xxxxxxxxxxxxxxxxxxxx"
              @input="errors.token = false"
            />
            <button type="button" class="input__toggle" :aria-label="showToken ? 'Hide token' : 'Show token'" @click="showToken = !showToken">
              <GfIcon :name="showToken ? 'eyeOff' : 'eye'" :size="15" />
            </button>
          </div>
          <span v-if="errors.token" class="field__msg">An access token is required.</span>
        </label>

        <!-- Advanced: the webhook our service exposes for GitFlame to register -->
        <button class="advanced" @click="showAdvanced = !showAdvanced">
          <GfIcon name="chevronRight" :size="14" :class="{ advanced__caret_open: showAdvanced }" class="advanced__caret" />
          Advanced
        </button>
        <div v-if="showAdvanced" class="field">
          <span class="field__label">
            Webhook URL (register this in GitFlame)
            <GfTooltip text="In GitFlame open the repository → Settings → Webhooks and add this URL. Subscribe it to the Issues and Issue comment events so approve / correct / reject reach CodePilot. The access token above needs repository read (to analyse code) and pull-request write (to open PRs)." />
          </span>
          <div class="input input_group">
            <GfIcon name="link" :size="15" class="input__lead" />
            <input :value="webhookUrl()" class="input__field mono" readonly />
            <button type="button" class="input__toggle" :title="copied ? 'Copied' : 'Copy'" @click="copyWebhook">
              <GfIcon :name="copied ? 'check' : 'copy'" :size="15" />
            </button>
          </div>
        </div>

        <!-- Usage policy consent (a single checkbox; the policy itself covers the
             "AI output may be wrong — trust, but verify" point) -->
        <div class="consent">
          <label class="check" :class="{ check_error: errors.policy }">
            <input type="checkbox" v-model="consent.policy" @change="errors.policy = false" />
            <span>
              I agree to the
              <button type="button" class="link-btn" @click.stop.prevent="showPolicy = true">service usage policy</button>,
              understand CodePilot’s output is AI-generated and needs review
              (<strong>trust, but verify</strong>), and allow it to access the connected
              repository for analysis and code generation.
            </span>
          </label>
        </div>

        <!-- Connection error (invalid/expired token, GitFlame or backend down) -->
        <div v-if="connectError" class="connect__err">
          <GfIcon name="alert" :size="16" />
          <div>
            <strong>{{ connectError.title }}</strong>
            <p>{{ connectError.message }}</p>
          </div>
        </div>

        <div class="connect__cta">
          <GfButton variant="primary" size="l" :loading="connecting" :class="{ 'connect__go_dim': !formComplete }" @click="submit">
            {{ connecting ? 'Connecting…' : 'Continue to workspace' }}
            <GfIcon v-if="!connecting" name="chevronRight" :size="16" />
          </GfButton>
          <p class="connect__foot gf-muted">
            You can change every setting later in the workspace.
          </p>
        </div>
      </div>
    </section>

    <!-- Service usage policy -->
    <GfModal v-if="showPolicy" title="Service usage policy" subtitle="GitFlame CodePilot" @close="showPolicy = false">
      <div class="policy">
        <p>
          GitFlame CodePilot is an AI assistant that produces implementation plans, code
          changes and repository recommendations. By connecting a repository you agree to
          the following.
        </p>
        <h4>1. AI-generated output</h4>
        <p>
          Plans, generated code and recommendations are produced by AI models and may be
          incomplete or incorrect. You are responsible for reviewing every change before it
          is merged — <strong>trust, but verify</strong>. CodePilot never merges code on its
          own and always requires your approval of a plan before generating code.
        </p>
        <h4>2. Repository access</h4>
        <p>
          CodePilot reads repository content only to analyse it and to generate the changes
          you request. It uses the access token you provide solely to read code and to open
          branches and pull requests for your review. It never force-pushes, deletes
          branches, or writes to your default branch directly.
        </p>
        <h4>3. Credentials and data handling</h4>
        <p>
          The access token you provide is a credential: it is kept only for your active
          session, is never shown back in full, and is used solely to read code and open
          branches / pull requests. CodePilot does not collect personal data about you.
          Repository snippets are sent to the configured model provider only for the
          duration of a request. Recommendation reports are stored for the retention period
          you set in the configuration and are removed afterwards.
        </p>
        <h4>4. Scope of analysis</h4>
        <p>
          Analysis respects the exclude paths and recommendation categories in your
          configuration. If no category is enabled, no recommendations are produced.
        </p>
        <h4>5. No warranty</h4>
        <p>
          This is a student capstone project provided “as is”, without warranty. Use the
          output at your own discretion and keep a human in the loop for every change.
        </p>
      </div>
      <template #footer>
        <GfButton variant="primary" @click="showPolicy = false">I understand</GfButton>
      </template>
    </GfModal>
  </div>
</template>

<style scoped>
.land {
  min-height: 100vh;
  background: var(--gf-hero-soft);
}
.topbar {
  background: var(--gf-surface);
  border-bottom: 1px solid var(--gf-line);
}
.topbar__inner {
  max-width: 980px;
  margin: 0 auto;
  padding: 0 24px;
  height: 56px;
  display: flex;
  align-items: center;
  gap: 12px;
}
.brand {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 700;
  font-size: 15px;
}
.brand__mark {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 9px;
  color: #fff;
  background: var(--gf-hero);
}
.topbar__link {
  text-decoration: none;
}
.topbar__link:hover {
  border-color: var(--gf-purple);
  color: var(--gf-accent);
  text-decoration: none;
}

.hero {
  max-width: 780px;
  margin: 0 auto;
  padding: 72px 24px 40px;
  text-align: center;
}
.hero__eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-bottom: 16px;
  padding: 5px 12px;
  border-radius: 999px;
  background: var(--gf-purple-soft);
  color: var(--gf-accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
  text-decoration: none;
  transition: background-color 0.12s ease;
}
a.hero__eyebrow:hover {
  background: #efe2ff;
  text-decoration: none;
}
.hero__title {
  margin: 0 0 18px;
  font-size: 56px;
  line-height: 1.03;
  font-weight: 800;
  letter-spacing: -0.025em;
  background: var(--gf-hero);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.hero__desc {
  margin: 0 auto 28px;
  max-width: 600px;
  font-size: 17px;
  line-height: 1.6;
  color: var(--gf-text-2);
}

.how {
  max-width: 780px;
  margin: 0 auto;
  padding: 12px 24px 8px;
}
.how__intro {
  text-align: center;
  margin-bottom: 18px;
}
.how__title {
  margin: 0 0 4px;
  font-size: 24px;
  letter-spacing: -0.01em;
}
.how__sub {
  margin: 0;
  font-size: 14px;
}
.how__try {
  margin-top: 34px;
  text-align: center;
}
.how__try-title {
  margin: 0 0 4px;
  font-size: 20px;
}
.how__try :deep(.try) {
  margin-top: 16px;
  text-align: left;
}

/* Continue button looks dim/inactive until every required field is filled,
   but stays clickable so a click reveals the red validation. */
.connect__go_dim {
  opacity: 0.5;
  filter: saturate(0.65);
}

.connect {
  max-width: 780px;
  margin: 0 auto;
  padding: 50px 24px 80px;
  scroll-margin-top: 18px;
}
.connect__head {
  text-align: center;
  margin-bottom: 18px;
}
.connect__title {
  margin: 0 0 4px;
  font-size: 24px;
  letter-spacing: -0.01em;
}
.connect__sub {
  margin: 0;
  font-size: 14px;
}
.connect__card {
  padding: 24px 26px 22px;
}
.field {
  display: block;
  margin-bottom: 16px;
}
.field__label {
  display: flex;
  align-items: center;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--gf-text-2);
  margin-bottom: 7px;
}
.field__opt {
  margin-left: 6px;
  font-weight: 500;
  color: var(--gf-text-3);
}
.field__msg {
  display: block;
  margin-top: 5px;
  font-size: 12px;
  color: var(--gf-red);
}
.input {
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
.input:focus {
  outline: none;
  border-color: var(--gf-purple);
}
.input::placeholder {
  color: var(--gf-text-3);
}
.input_group {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
}
.input__lead {
  color: var(--gf-text-3);
  flex: none;
}
.input__field {
  flex: 1;
  height: 100%;
  border: 0;
  outline: 0;
  background: transparent;
  font: inherit;
  font-size: 13.5px;
  color: var(--gf-text);
}
.input__field::placeholder {
  color: var(--gf-text-3);
}
/* Hide the browser's built-in password reveal/clear control (Edge) so only our
   custom toggle remains. */
.input__field::-ms-reveal,
.input__field::-ms-clear {
  display: none;
}
.input__toggle {
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
.input__toggle:hover {
  background: var(--gf-surface-3);
  color: var(--gf-text);
}
/* Red underline + border on validation failure */
.field_error .input {
  border-color: var(--gf-red);
  box-shadow: 0 1px 0 0 var(--gf-red);
}

.advanced {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin: 0 0 14px;
  border: 0;
  background: transparent;
  color: var(--gf-text-2);
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
}
.advanced__caret {
  transition: transform 0.12s ease;
}
.advanced__caret_open {
  transform: rotate(90deg);
}

.consent {
  display: grid;
  gap: 12px;
  margin: 4px 0 20px;
  padding: 16px;
  border: 1px solid var(--gf-line);
  border-radius: 12px;
  background: var(--gf-surface-2);
}
.check {
  display: flex;
  gap: 10px;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--gf-text-2);
  cursor: pointer;
}
.check input {
  margin-top: 2px;
  flex: none;
  width: 16px;
  height: 16px;
  accent-color: var(--gf-purple);
}
.check_error {
  color: var(--gf-red);
}
.check_error input {
  outline: 2px solid var(--gf-red);
  outline-offset: 1px;
  border-radius: 3px;
}
.connect__foot {
  margin: 12px 0 0;
  font-size: 12px;
  text-align: center;
}
.connect__err {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin: 0 0 16px;
  padding: 12px 14px;
  border: 1px solid var(--gf-red);
  border-radius: 10px;
  background: var(--gf-red-bg);
}
.connect__err :deep(.gf-icon) {
  color: var(--gf-red);
  flex: none;
  margin-top: 1px;
}
.connect__err strong {
  font-size: 13px;
  color: var(--gf-red);
}
.connect__err p {
  margin: 3px 0 0;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--gf-text-2);
}
.connect__cta {
  display: flex;
  flex-direction: column;
  align-items: center;
}
.link-btn {
  border: 0;
  background: transparent;
  padding: 0;
  font: inherit;
  font-size: inherit;
  color: var(--gf-accent);
  font-weight: 600;
  text-decoration: underline;
  cursor: pointer;
}
.policy {
  font-size: 13.5px;
  line-height: 1.6;
  color: var(--gf-text-2);
}
.policy h4 {
  margin: 16px 0 4px;
  font-size: 13.5px;
  color: var(--gf-text);
}
.policy p {
  margin: 0 0 8px;
}
.policy strong {
  color: var(--gf-accent);
}
@media (max-width: 560px) {
  .hero__title {
    font-size: 40px;
  }
}
</style>
