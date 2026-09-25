import type { ProposalState } from './enums/proposal-state'

export interface BoundaryProposal {
  id: number
  parcel_id: number
  base_version: number
  proposed_geojson: string
  observation_ids: number[] | string
  snap_tolerance_m: number
  area_delta_square_m: number
  proposal_state: ProposalState
  rationale: string
  version: number
  created_by: number
  reviewed_by?: number | null
  created_at: string
  updated_at: string
}
