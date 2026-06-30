<script setup>
// Demo GitFlame repository page — the entry point for the demo.
//
// It re-creates the look of a GitFlame "Code" tab (top bar, repo header, branch
// toolbar, file list) so reviewers see *where* CodePilot would plug in. Per the
// brief it deliberately does NOT show recommendation cards here: GitFlame will
// not embed our UI, so the only integration point shown is the purple
// "Work with AI" button, which routes to our standalone service landing.
import { useRouter } from 'vue-router'
import { DEMO_REPO, demoFiles, demoLastCommit } from '../data/demo.js'
import RepoChrome from '../components/RepoChrome.vue'
import RepoToolbar from '../components/RepoToolbar.vue'
import FileBrowser from '../components/FileBrowser.vue'

const router = useRouter()

// Compose the repo object the existing GitFlame-chrome components expect.
const gfRepo = {
  owner: DEMO_REPO.owner,
  name: DEMO_REPO.name,
  defaultBranch: DEMO_REPO.defaultBranch,
  branches: ['main', 'develop', 'ai/demo'],
  lastCommit: demoLastCommit,
}

function goToService() {
  router.push('/codepilot')
}
</script>

<template>
  <RepoChrome :repo="gfRepo" active-tab="code">
    <RepoToolbar :repo="gfRepo" @work-with-ai="goToService" />
    <FileBrowser :repo="gfRepo" :files="demoFiles" />
    <p class="hint gf-muted">
      This is a mock of a GitFlame repository page. Press
      <strong>Work with AI</strong> to open the CodePilot service.
    </p>
  </RepoChrome>
</template>

<style scoped>
.hint {
  margin: 16px 2px 0;
  font-size: 12.5px;
  text-align: center;
}
</style>
