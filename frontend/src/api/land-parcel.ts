import { api } from './client'
import type { ApiEnvelope } from '@/types/api'
import type { LandParcel } from '@/types/land-parcel'

export interface ParcelQuery {
  keyword?: string
  state?: string
  page?: number
  page_size?: number
}

export type ParcelInput = {
  parcel_code?: string
  name?: string
  boundary_geojson?: string
  coordinate_system?: string
  owner_org?: string
  parcel_state?: string
  boundary_version?: number
}

export const landParcelApi = {
  list: (params?: ParcelQuery) => api.get<ApiEnvelope<LandParcel[]>>('/parcels', { params }),
  detail: (id: number) => api.get<ApiEnvelope<LandParcel>>(`/parcels/${id}`),
  create: (body: ParcelInput) => api.post<ApiEnvelope<LandParcel>>('/parcels', body),
  update: (id: number, body: ParcelInput) => api.patch<ApiEnvelope<LandParcel>>(`/parcels/${id}`, body),
}
