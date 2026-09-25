export interface LandParcel {
  id: number
  parcel_code: string
  name: string
  boundary_geojson: string
  coordinate_system: string
  area_square_m: number
  boundary_version: number
  owner_org: string
  parcel_state: string
  created_at: string
  updated_at: string
}
