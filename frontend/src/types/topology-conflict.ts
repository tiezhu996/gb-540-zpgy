import type { ConflictType } from './enums/conflict-type'

export interface TopologyConflict {
  id: number
  proposal_id: number
  parcel_ids: number[]
  conflict_type: ConflictType
  geometry_geojson: string
  magnitude_square_m: number
  severity: string
  algorithm_version: string
  input_hash: string
  conflict_state: string
  suggested_resolution_json: string
  explanation: string
  detected_at: string
  resolved_by?: number | null
}

// The relational model stores the participant IDs as JSON text. Keep that wire
// format at the HTTP boundary and expose a stable array to pages and stores.
export type TopologyConflictWire = Omit<TopologyConflict, 'parcel_ids'> & {
  parcel_ids: number[] | string | null
}

export function normalizeParcelIDs(value: TopologyConflictWire['parcel_ids']): number[] {
  let candidates: unknown[] = []
  if (Array.isArray(value)) {
    candidates = value
  } else if (typeof value === 'string') {
    try {
      const parsed: unknown = JSON.parse(value)
      if (Array.isArray(parsed)) candidates = parsed
    } catch {
      return []
    }
  }

  const seen = new Set<number>()
  return candidates.reduce<number[]>((ids, candidate) => {
    const id = typeof candidate === 'number' ? candidate : Number(candidate)
    if (Number.isSafeInteger(id) && id > 0 && !seen.has(id)) {
      seen.add(id)
      ids.push(id)
    }
    return ids
  }, [])
}

export function normalizeTopologyConflict(item: TopologyConflictWire): TopologyConflict {
  return { ...item, parcel_ids: normalizeParcelIDs(item.parcel_ids) }
}

// A recheck participant pins one parcel to the boundary version and geometry
// hash the reviewer verified before applying a suggestion.
export interface RecheckParticipant {
  parcel_id: number
  parcel_code: string
  boundary_version: number
  geometry_hash: string
}

export interface RecheckResidualConflict {
  parcel_ids: number[]
  conflict_type: ConflictType
  magnitude_square_m: number
  explanation: string
}

// SuggestionRecheck is the verifiable review evidence recomputed against the
// current parcel version and every active neighbour. The participant snapshot
// must be echoed back when the suggestion is applied.
export interface SuggestionRecheck {
  conflict_id: number
  proposal_id: number
  parcel_id: number
  parcel_code: string
  base_version: number
  coordinate_system: string
  snap_tolerance_m: number
  area_delta_square_m: number
  moved_coordinate_count: number
  residual_conflicts: RecheckResidualConflict[]
  participants: RecheckParticipant[]
  snapshot_hash: string
  detection_stale: boolean
  applicable: boolean
}

export interface ParticipantChange {
  parcel_id: number
  parcel_code: string
  submitted_boundary_version: number
  current_boundary_version: number
  geometry_changed: boolean
}

// SuggestionApplyRejection is the structured 409 payload returned when the
// apply is refused; the conflict keeps its previous state.
export interface SuggestionApplyRejection {
  reason: 'participants_changed' | 'residual_conflicts'
  changed_participants?: ParticipantChange[]
  added_participants?: RecheckParticipant[]
  removed_participants?: RecheckParticipant[]
  residual_conflicts?: RecheckResidualConflict[]
  current_snapshot_hash?: string
}
