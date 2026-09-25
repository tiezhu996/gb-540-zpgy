import { api } from './client'
import type { ApiEnvelope } from '@/types/api'
import type { SurveyObservation } from '@/types/survey-observation'

export interface ObservationQuery {
  parcel_id?: number
  state?: string
  page?: number
  page_size?: number
}

export interface ObservationImportInput {
  parcel_id: number
  observation_code: string
  point_geojson: string
  observed_at: string
  method: string
  horizontal_accuracy_m: number
  source_checksum: string
  observation_state?: string
  quality_note?: string
}

export type ObservationState = 'accepted' | 'rejected' | 'superseded'

export interface ObservationTransitionInput {
  to: ObservationState
  version: number
  replacement_observation_id?: number
  quality_note?: string
}

export const surveyObservationApi = {
  list: (params?: ObservationQuery) => api.get<ApiEnvelope<SurveyObservation[]>>('/observations', { params }),
  detail: (id: number) => api.get<ApiEnvelope<SurveyObservation>>(`/observations/${id}`),
  import: (body: ObservationImportInput) => api.post<ApiEnvelope<SurveyObservation>>('/observations/import', body),
  transition: (id: number, body: ObservationTransitionInput) =>
    api.post<ApiEnvelope<SurveyObservation>>(`/observations/${id}/transition`, body),
}
