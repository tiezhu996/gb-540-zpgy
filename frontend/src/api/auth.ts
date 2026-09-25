import { api } from './client'
import type { ApiEnvelope } from '@/types/api'
import type { User } from '@/types/auth'

export const authApi = {
  login: (body: { username: string; password: string }) =>
    api.post<ApiEnvelope<{ token: string; expires_at: string; user: User }>>('/auth/login', body),
}
