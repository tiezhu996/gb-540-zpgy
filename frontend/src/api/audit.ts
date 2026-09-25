import { api } from './client'
import type { ApiEnvelope } from '@/types/api'
import type { AuditLog } from '@/types/audit'

export interface AuditQuery {
  request_id?: string
  entity?: string
  actor?: string
  from?: string
  to?: string
  page?: number
  page_size?: number
}

export const auditApi = {
  list: (params?: AuditQuery) => api.get<ApiEnvelope<AuditLog[]>>('/audit', { params }),
}
