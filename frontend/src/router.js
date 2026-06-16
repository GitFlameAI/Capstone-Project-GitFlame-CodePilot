import { createRouter, createWebHistory } from 'vue-router'
import RepositoryView from './views/RepositoryView.vue'
import DetailedAnalysisView from './views/DetailedAnalysisView.vue'

const routes = [
  {
    path: '/',
    name: 'repository',
    component: RepositoryView,
    meta: { title: 'Repository · GitFlame CodePilot' },
  },
  {
    path: '/recommendations',
    name: 'recommendations',
    component: DetailedAnalysisView,
    meta: { title: 'Detailed analysis · GitFlame CodePilot' },
  },
  // Unknown paths fall back to the repository view.
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
