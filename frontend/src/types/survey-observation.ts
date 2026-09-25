export interface SurveyObservation {
  id: number
  parcel_id: number
  observation_code: string
  point_geojson: string
  observed_at: string
  method: string
  horizontal_accuracy_m: number
  source_checksum: string
  observation_state: string
  version: number
  replaced_by?: number | null
  quality_note: string
  imported_by: number
}
