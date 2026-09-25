<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Check, FilePlus, RefreshCw, ScanSearch, ShieldCheck, X } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import TopologyLegend from '@/components/common/TopologyLegend.vue'
import GeometryEvidenceDrawer from '@/components/common/GeometryEvidenceDrawer.vue'
import ProposalStateBadge from '@/components/common/ProposalStateBadge.vue'
import { useTopologyConflictStore } from '@/stores/topology-conflict'
import { useBoundaryProposalStore } from '@/stores/boundary-proposal'
import { useLandParcelStore } from '@/stores/land-parcel'
import { useAuth } from '@/hooks/useAuth'
import { conflictTypeLabel } from '@/types/enums/conflict-type'
import type { ProposalState } from '@/types/enums/proposal-state'
import type { SuggestionReview, TopologyConflict } from '@/types/topology-conflict'

const conflicts = useTopologyConflictStore()
const proposals = useBoundaryProposalStore()
const parcels = useLandParcelStore()
const auth = useAuth()
const detectOpen = ref(false)
const evidenceOpen = ref(false)
const selected = ref<TopologyConflict | null>(null)
const reviewOpen = ref(false)
const reviewLoading = ref(false)
const applying = ref(false)
const review = ref<SuggestionReview | null>(null)
const reviewTarget = ref<TopologyConflict | null>(null)
const form = reactive({ proposal_id: 0 })

const participantStatusLabel: Record<string, string> = { unchanged: '未变化', changed: '已变更', added: '新增相邻', removed: '已移除', unverified: '无法核对' }

function typeLabel(value: unknown) {
  return conflictTypeLabel[value as keyof typeof conflictTypeLabel] ?? String(value)
}

function proposalStateFor(conflict: TopologyConflict): ProposalState | null {
  return proposals.items.find((proposal) => proposal.id === conflict.proposal_id)?.proposal_state ?? null
}

function parcelLabel(id: number) {
  return parcels.items.find((item) => item.id === id)?.parcel_code ?? `地块 #${id}`
}

function parcelLabels(ids: number[]) {
  return ids.length ? ids.map(parcelLabel).join(' · ') : '未记录参与地块'
}

function parcelLabelFor(conflict: TopologyConflict) {
  return parcelLabels(conflict.parcel_ids)
}

async function load() {
  await Promise.all([
    parcels.fetch({ page_size: 100 }),
    proposals.fetch({ page_size: 100 }),
    conflicts.fetch({ page_size: 100 }),
  ])
}

async function detect() {
  await conflicts.detect({ proposal_id: form.proposal_id })
  detectOpen.value = false
  await load()
}

async function transition(item: TopologyConflict, to: string) {
  await conflicts.transition(item.id, { to })
  await load()
}

async function openReview(item: TopologyConflict) {
  reviewTarget.value = item
  review.value = null
  reviewOpen.value = true
  await refreshReview()
}

async function refreshReview() {
  if (!reviewTarget.value) return
  reviewLoading.value = true
  try {
    review.value = await conflicts.reviewSuggestion(reviewTarget.value.id)
  } catch {
    reviewOpen.value = false
  } finally {
    reviewLoading.value = false
  }
}

function reviewFromError(error: unknown): SuggestionReview | null {
  const details = (error as { response?: { data?: { error?: { details?: unknown } } } })?.response?.data?.error?.details
  return details && typeof details === 'object' ? (details as SuggestionReview) : null
}

async function confirmApply() {
  if (!reviewTarget.value || !review.value) return
  applying.value = true
  try {
    const created = await conflicts.applySuggestion(reviewTarget.value.id, { snapshot_hash: review.value.snapshot_hash })
    ElMessage.success(`已生成草稿提案 #${created.id}，冲突 #${reviewTarget.value.id} 已结案`)
    reviewOpen.value = false
    await load()
  } catch (error) {
    const fresh = reviewFromError(error)
    if (fresh) review.value = fresh
    await load()
  } finally {
    applying.value = false
  }
}

function showEvidence(item: TopologyConflict) {
  selected.value = item
  evidenceOpen.value = true
}

onMounted(load)
</script>

<template>
  <PageHeader title="冲突消解" eyebrow="TOPOLOGY CONFLICTS" description="查看重叠、缝隙和无效拓扑证据，所有建议都需要人工确认。">
    <el-button v-if="auth.hasRole('gis_analyst', 'admin')" type="primary" @click="detectOpen = true"><ScanSearch :size="15" />运行检测</el-button>
  </PageHeader>

  <section class="content-band">
    <div class="toolbar"><el-button @click="load"><RefreshCw :size="15" />刷新</el-button><TopologyLegend /><span class="toolbar-spacer subtle-count">{{ conflicts.items.length }} 条冲突</span></div>
    <div class="data-surface">
      <el-table v-loading="conflicts.loading" :data="conflicts.items" row-key="id">
        <el-table-column label="冲突" width="90"><template #default="scope"><strong>#{{ scope.row.id }}</strong></template></el-table-column>
        <el-table-column label="参与地块" min-width="180"><template #default="scope"><strong>{{ parcelLabelFor(scope.row) }}</strong><small class="muted">提案 #{{ scope.row.proposal_id }}</small><ProposalStateBadge :state="proposalStateFor(scope.row)" /></template></el-table-column>
        <el-table-column label="类型" width="125"><template #default="scope"><span :class="['conflict-tag', `tone-${scope.row.conflict_type}`]">{{ typeLabel(scope.row.conflict_type) }}</span></template></el-table-column>
        <el-table-column prop="severity" label="严重度" width="95" />
        <el-table-column label="量级" width="125"><template #default="scope">{{ scope.row.magnitude_square_m.toFixed(2) }} m²</template></el-table-column>
        <el-table-column prop="explanation" label="说明" min-width="220" show-overflow-tooltip />
        <el-table-column label="状态" width="145"><template #default="scope"><span class="status-pill" :class="scope.row.conflict_state">{{ scope.row.conflict_state }}</span></template></el-table-column>
        <el-table-column label="动作" width="270"><template #default="scope"><div class="conflict-actions"><el-button text @click="showEvidence(scope.row)">证据</el-button><template v-if="auth.hasRole('reviewer', 'admin')"><el-button v-if="scope.row.conflict_state === 'detected'" text type="primary" @click="transition(scope.row, 'confirmed')"><Check :size="14" />确认</el-button><el-button v-if="scope.row.conflict_state === 'detected'" text type="warning" @click="transition(scope.row, 'false_positive')"><X :size="14" />误报</el-button><el-button v-if="scope.row.conflict_state === 'confirmed'" text type="primary" @click="transition(scope.row, 'resolution_proposed')"><FilePlus :size="14" />准备建议</el-button><el-button v-if="scope.row.conflict_state === 'resolution_proposed'" text type="primary" @click="openReview(scope.row)"><ShieldCheck :size="14" />复核并应用</el-button><el-button v-if="scope.row.conflict_state === 'false_positive' || scope.row.conflict_state === 'resolved'" text @click="transition(scope.row, 'closed')"><X :size="14" />关闭</el-button></template></div></template></el-table-column>
      </el-table>
      <div v-if="!conflicts.loading && !conflicts.items.length" class="empty-state"><div><strong>暂无冲突</strong><span>选择一个提案运行检测，系统会记录算法版本和输入哈希。</span></div></div>
    </div>
  </section>

  <el-dialog v-model="detectOpen" title="检测提案拓扑" width="min(480px, calc(100vw - 28px))">
    <el-form label-position="top"><el-form-item label="边界提案"><el-select v-model="form.proposal_id" placeholder="选择提案" style="width: 100%"><el-option v-for="item in proposals.items" :key="item.id" :label="`提案 #${item.id} · ${item.proposal_state}`" :value="item.id" /></el-select></el-form-item><el-alert type="warning" :closable="false" title="检测是离线决策支持，不会修改原始地块边界或法定登记。" /></el-form>
    <template #footer><el-button @click="detectOpen = false">取消</el-button><el-button type="primary" :disabled="!form.proposal_id" @click="detect">开始检测</el-button></template>
  </el-dialog>

  <el-dialog v-model="reviewOpen" title="应用建议前复核" width="min(760px, calc(100vw - 28px))">
    <div v-loading="reviewLoading" class="review-body">
      <template v-if="review">
        <div class="review-stats">
          <div class="stat"><span>面积增减</span><strong :class="review.area_delta_square_m >= 0 ? 'positive' : 'negative'">{{ review.area_delta_square_m >= 0 ? '+' : '' }}{{ review.area_delta_square_m.toFixed(2) }} m²</strong></div>
          <div class="stat"><span>坐标移动</span><strong>{{ review.snap_change_count }} 处</strong></div>
          <div class="stat"><span>参与地块快照</span><strong :class="review.snapshot_matches ? 'positive' : 'negative'">{{ review.snapshot_matches ? '一致' : '已变化' }}</strong></div>
          <div class="stat"><span>残留冲突</span><strong :class="review.residual_conflicts.length ? 'negative' : 'positive'">{{ review.residual_conflicts.length }} 条</strong></div>
        </div>

        <el-alert v-if="review.blockers.length" type="warning" :closable="false" title="复核未通过：冲突保持 resolution_proposed，不会生成草稿。">
          <ul class="blocker-list"><li v-for="blocker in review.blockers" :key="blocker">{{ blocker }}</li></ul>
        </el-alert>
        <el-alert v-else type="success" :closable="false" title="快照与当前参与地块一致，重算无残留冲突，可生成草稿并结案。" />

        <h4 class="review-heading">参与地块核对</h4>
        <el-table :data="review.participants" size="small">
          <el-table-column label="地块" min-width="150"><template #default="scope"><strong>{{ scope.row.parcel_code || `地块 #${scope.row.parcel_id}` }}</strong><small class="muted">#{{ scope.row.parcel_id }}</small></template></el-table-column>
          <el-table-column label="快照版本" width="95"><template #default="scope">{{ scope.row.stored_version || '—' }}</template></el-table-column>
          <el-table-column label="当前版本" width="95"><template #default="scope">{{ scope.row.current_version || '—' }}</template></el-table-column>
          <el-table-column label="状态" width="110"><template #default="scope"><span :class="['review-tag', `tone-${scope.row.status}`]">{{ participantStatusLabel[scope.row.status] ?? scope.row.status }}</span></template></el-table-column>
        </el-table>

        <h4 class="review-heading">重算结果</h4>
        <el-table v-if="review.recheck_conflicts.length" :data="review.recheck_conflicts" size="small">
          <el-table-column label="类型" width="110"><template #default="scope"><span :class="['conflict-tag', `tone-${scope.row.conflict_type}`]">{{ typeLabel(scope.row.conflict_type) }}</span></template></el-table-column>
          <el-table-column label="参与地块" min-width="160"><template #default="scope">{{ parcelLabels(scope.row.parcel_ids) }}</template></el-table-column>
          <el-table-column label="量级" width="115"><template #default="scope">{{ scope.row.magnitude_square_m.toFixed(2) }} m²</template></el-table-column>
          <el-table-column label="判定" width="90"><template #default="scope"><span :class="['review-tag', scope.row.self ? 'tone-self' : 'tone-residual']">{{ scope.row.self ? '本冲突' : '残留' }}</span></template></el-table-column>
        </el-table>
        <div v-else class="empty-mini">重算未发现任何拓扑冲突。</div>
      </template>
    </div>
    <template #footer>
      <el-button @click="reviewOpen = false">取消</el-button>
      <el-button v-if="review && !review.can_apply" :loading="reviewLoading" @click="refreshReview">重新核对</el-button>
      <el-button v-if="review?.can_apply" type="primary" :loading="applying" @click="confirmApply">创建草稿并结案</el-button>
    </template>
  </el-dialog>

  <GeometryEvidenceDrawer v-model="evidenceOpen" title="冲突证据" :geometry="selected?.geometry_geojson" :explanation="selected?.explanation" />
</template>

<style scoped>
.conflict-tag { display: inline-flex; padding: 3px 8px; border: 1px solid var(--line-strong); font-size: 12px; font-weight: 800; }
.tone-overlap { color: #9c3028; background: #fbeceb; border-color: #e6afaa; }.tone-gap { color: #755310; background: #fff5db; border-color: #e0c16b; }.tone-self_intersection { color: #7b4c9e; background: #f4ecfa; }.tone-dangling_edge { color: #2b6f96; background: #e8f2f7; }
.muted { display: block; margin-top: 4px; color: var(--text-muted); font-size: 11px; }
.conflict-actions { display: flex; flex-wrap: wrap; gap: 2px; }
.status-pill.confirmed, .status-pill.resolution_proposed { color: #755310; background: #fff5db; }.status-pill.resolved { color: #17604e; background: #e8f4f0; }.status-pill.false_positive, .status-pill.closed { color: #4b5551; background: #e8ecea; }
.review-body { min-height: 160px; }
.review-stats { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; margin-bottom: 14px; }
.stat { padding: 10px 12px; border: 1px solid var(--line); background: var(--surface-muted, #f4f7f5); }
.stat span { display: block; color: var(--text-muted); font-size: 12px; }
.stat strong { display: block; margin-top: 4px; font-size: 16px; }
.positive { color: #17604e; }.negative { color: #9c3028; }
.blocker-list { margin: 6px 0 0; padding-left: 18px; }
.review-heading { margin: 16px 0 8px; font-size: 13px; }
.review-tag { display: inline-flex; padding: 2px 8px; border: 1px solid var(--line-strong); font-size: 12px; font-weight: 700; }
.tone-unchanged { color: #17604e; background: #e8f4f0; }.tone-changed, .tone-residual { color: #9c3028; background: #fbeceb; }.tone-added { color: #755310; background: #fff5db; }.tone-removed, .tone-unverified { color: #4b5551; background: #e8ecea; }.tone-self { color: #2b6f96; background: #e8f2f7; }
.empty-mini { padding: 12px; color: var(--text-muted); border: 1px dashed var(--line-strong); font-size: 13px; }
@media (max-width: 640px) { .review-stats { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
