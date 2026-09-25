export const PROPOSAL_STATES = ['draft', 'validated', 'submitted', 'reviewed', 'revision', 'accepted', 'rejected'] as const
export type ProposalState = (typeof PROPOSAL_STATES)[number]
export const proposalStateLabel: Record<ProposalState, string> = { draft: '草拟', validated: '已校验', submitted: '已提交', reviewed: '已复核', revision: '待修订', accepted: '内部采纳', rejected: '已拒绝' }
