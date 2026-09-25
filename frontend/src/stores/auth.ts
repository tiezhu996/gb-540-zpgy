import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { authApi } from '@/api/auth'
import type { User } from '@/types/auth'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('parcelgraph_token') ?? '')
  const stored = localStorage.getItem('parcelgraph_user')
  const user = ref<User | null>(stored ? JSON.parse(stored) : null)
  const authenticated = computed(() => Boolean(token.value && user.value))

  async function login(username: string, password: string) {
    const { data } = await authApi.login({ username, password })
    token.value = data.data.token
    user.value = data.data.user
    localStorage.setItem('parcelgraph_token', token.value)
    localStorage.setItem('parcelgraph_user', JSON.stringify(user.value))
  }
  function logout() {
    token.value = ''; user.value = null
    localStorage.removeItem('parcelgraph_token'); localStorage.removeItem('parcelgraph_user')
  }
  function hasRole(...roles: User['role'][]) { return Boolean(user.value && roles.includes(user.value.role)) }
  return { token, user, authenticated, login, logout, hasRole }
})
