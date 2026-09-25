import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('@/pages/LoginPage.vue'), meta: { public: true } },
    { path: '/', redirect: '/parcels' },
    { path: '/parcels', name: 'parcels', component: () => import('@/pages/ParcelsPage.vue') },
    { path: '/observations', name: 'observations', component: () => import('@/pages/ObservationsPage.vue') },
    { path: '/proposals', name: 'proposals', component: () => import('@/pages/ProposalsPage.vue') },
    { path: '/conflicts', name: 'conflicts', component: () => import('@/pages/ConflictsPage.vue') },
    { path: '/audit', name: 'audit', component: () => import('@/pages/AuditPage.vue'), meta: { roles: ['reviewer', 'auditor', 'admin'] } },
    { path: '/:pathMatch(.*)*', redirect: '/parcels' },
  ],
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (!to.meta.public && !auth.authenticated) return { name: 'login', query: { redirect: to.fullPath } }
  if (to.name === 'login' && auth.authenticated) return { name: 'parcels' }
  const roles = to.meta.roles as string[] | undefined
  if (roles && !roles.includes(auth.user?.role ?? '')) return { name: 'parcels' }
  return true
})

export default router
