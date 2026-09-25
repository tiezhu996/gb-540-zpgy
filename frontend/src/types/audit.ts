export type AuditEntity = 'LandParcel' | 'SurveyObservation' | 'BoundaryProposal' | 'TopologyConflict'

export interface AuditLog {
  id: number
  actor_id: number
  actor_name: string
  action: string
  resource_type?: AuditEntity | string
  entity?: AuditEntity | string
  resource_id: number
  request_id: string
  before: string
  after: string
  created_at: string
}
