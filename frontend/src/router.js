import { createRouter, createWebHistory } from 'vue-router'
import GitFlameDemoView from './views/GitFlameDemoView.vue'
import LandingView from './views/LandingView.vue'
import WorkspaceView from './views/WorkspaceView.vue'

// Three screens, matching the demo flow:
//   /            mock GitFlame repository page with the "Work with AI" button
//   /codepilot   CodePilot service landing + repository connect form
//   /workspace   the 4-tab workspace (Repository / Config / Autogen / Recommendations)
const routes = [
  {
    path: '/',
    name: 'gitflame',
    component: GitFlameDemoView,
    meta: { title: 'Repository · GitFlame' },
  },
  {
    path: '/codepilot',
    name: 'codepilot',
    component: LandingView,
    meta: { title: 'GitFlame CodePilot' },
  },
  {
    path: '/workspace',
    name: 'workspace',
    component: WorkspaceView,
    meta: { title: 'Workspace · GitFlame CodePilot' },
  },
  // Unknown paths fall back to the demo GitFlame page.
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior() {
    return { top: 0 }
  },
})

router.afterEach((to) => {
  if (to.meta?.title) document.title = to.meta.title
})

export default router
