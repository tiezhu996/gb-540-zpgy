export const APP_ROLES = ['surveyor', 'gis_analyst', 'reviewer', 'auditor', 'admin'] as const

export type AppRole = (typeof APP_ROLES)[number]

export interface User {
  id: number
  username: string
  display_name: string
  role: AppRole
}
