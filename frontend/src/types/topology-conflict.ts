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
