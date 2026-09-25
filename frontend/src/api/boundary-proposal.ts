import { api } from './client'
import type { ApiEnvelope } from '@/types/api'
import type { BoundaryProposal } from '@/types/boundary-proposal'
import type { ProposalState } from '@/types/enums/proposal-state'

export interface ProposalQuery {
  parcel_id?: number
  state?: ProposalState
  page?: number
  page_size?: number
}

export interface ProposalCreateInput {
  parcel_id: number
  base_version: number
  proposed_geojson: string
  observation_ids: number[]
  snap_tolerance_m: number
  rationale: string
}

export interface ProposalTransitionInput {
  to: ProposalState
  version: number
  rationale?: string
}

export const boundaryProposalApi = {
  list: (params?: ProposalQuery) => api.get<ApiEnvelope<BoundaryProposal[]>>('/proposals', { params }),
  detail: (id: number) => api.get<ApiEnvelope<BoundaryProposal>>(`/proposals/${id}`),
  create: (body: ProposalCreateInput) => api.post<ApiEnvelope<BoundaryProposal>>('/proposals', body),
  transition: (id: number, body: ProposalTransitionInput) =>
    api.post<ApiEnvelope<BoundaryProposal>>(`/proposals/${id}/transition`, body),
}
