export const CONFLICT_TYPES = ['overlap', 'gap', 'self_intersection', 'dangling_edge'] as const
export type ConflictType = (typeof CONFLICT_TYPES)[number]
export const conflictTypeLabel: Record<ConflictType, string> = { overlap: '重叠', gap: '缝隙', self_intersection: '自相交', dangling_edge: '悬挂边' }
