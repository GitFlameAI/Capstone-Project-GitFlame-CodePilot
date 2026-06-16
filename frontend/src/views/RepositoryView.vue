<script setup>
import { ref } from 'vue'
import { demoRepo, demoFiles } from '../data/demo.js'
import RepoChrome from '../components/RepoChrome.vue'
import RepoToolbar from '../components/RepoToolbar.vue'
import FileBrowser from '../components/FileBrowser.vue'
import RecommendationsWidget from '../components/RecommendationsWidget.vue'
import WorkWithAiWizard from '../components/WorkWithAiWizard.vue'

// This view imitates the GitFlame repository / Code page so the AI integration
// points (the purple "Work with AI" button and the recommendations widget) can
// be demonstrated in context.
const activeTab = ref('code')
const wizardOpen = ref(false)
</script>

<template>
  <RepoChrome :repo="demoRepo" :active-tab="activeTab" @tab="activeTab = $event">
    <!-- Code tab: the main demo surface -->
    <template v-if="activeTab === 'code'">
      <RepoToolbar :repo="demoRepo" @work-with-ai="wizardOpen = true" />
      <FileBrowser :repo="demoRepo" :files="demoFiles" />
      <RecommendationsWidget />
    </template>

    <!-- Other tabs are placeholders – the demo focuses on the Code tab -->
    <div v-else class="placeholder gf-card">
      <p>
        This is a demo of the GitFlame AI integration. The
        <strong>Code</strong> tab hosts the “Work with AI” button and the
        recommendations widget.
      </p>
      <button class="placeholder__link" @click="activeTab = 'code'">
        ← Back to Code
      </button>
    </div>
  </RepoChrome>

  <WorkWithAiWizard v-if="wizardOpen" @close="wizardOpen = false" />
</template>

<style scoped>
.placeholder {
  padding: 40px 28px;
  text-align: center;
  color: var(--gf-text-2);
  font-size: 14px;
  line-height: 1.6;
}
.placeholder__link {
  margin-top: 14px;
  border: 0;
  background: transparent;
  color: var(--gf-accent);
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}
</style>
