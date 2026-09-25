<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
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
import type { RecheckResidualConflict, SuggestionApplyRejection, SuggestionRecheck, TopologyConflict } from '@/types/topology-conflict'

const conflicts = useTopologyConflictStore()
const proposals = useBoundaryProposalStore()
const parcels = useLandParcelStore()
const auth = useAuth()
const detectOpen = ref(false)
const evidenceOpen = ref(false)
const selected = ref<TopologyConflict | null>(null)
const form = reactive({ proposal_id: 0 })

const reviewOpen = ref(false)
const reviewLoading = ref(false)
const applyLoading = ref(false)
const reviewTarget = ref<TopologyConflict | null>(null)
const recheck = ref<SuggestionRecheck | null>(null)
const applyRejection = ref<SuggestionApplyRejection | null>(null)

function typeLabel(value: unknown) {
  return conflictTypeLabel[value as keyof typeof conflictTypeLabel] ?? String(value)
}

function proposalStateFor(conflict: TopologyConflict): ProposalState | null {
  return proposals.items.find((proposal) => proposal.id === conflict.proposal_id)?.proposal_state ?? null
}

function parcelLabelFor(conflict: TopologyConflict) {
  const labels = conflict.parcel_ids.map((id) => {
    const parcel = parcels.items.find((item) => item.id === id)
    return parcel ? parcel.parcel_code : `地块 #${id}`
  })
  return labels.length ? labels.join(' · ') : '未记录参与地块'
}

function participantLabel(id: number) {
  const participant = recheck.value?.participants.find((item) => item.parcel_id === id)
  if (participant) return participant.parcel_code
  const parcel = parcels.items.find((item) => item.id === id)
  return parcel ? parcel.parcel_code : `地块 #${id}`
}

function residualLabel(residual: RecheckResidualConflict) {
  return residual.parcel_ids.map(participantLabel).join(' · ')
}

function formatDelta(value: number) {
  const sign = value > 0 ? '+' : value < 0 ? '−' : '±'
  return `${sign}${Math.abs(value).toFixed(2)}`
}

const deltaClass = computed(() => {
  const value = recheck.value?.area_delta_square_m ?? 0
  if (value > 0) return 'delta-positive'
  if (value < 0) return 'delta-negative'
  return ''
})

const rejectionTitle = computed(() =>
  applyRejection.value?.reason === 'participants_changed'
    ? '参与地块版本或集合已变化，冲突保持原状态；请重新核对后再提交。'
    : '重算仍存在残留冲突，冲突保持原状态。',
)

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

async function applySuggestion(item: TopologyConflict) {
  reviewTarget.value = item
  recheck.value = null
  applyRejection.value = null
  reviewOpen.value = true
  await runRecheck()
}

async function runRecheck() {
  if (!reviewTarget.value) return
  reviewLoading.value = true
  applyRejection.value = null
  try {
    recheck.value = await conflicts.recheckSuggestion(reviewTarget.value.id)
  } catch {
    reviewOpen.value = false
  } finally {
    reviewLoading.value = false
  }
}

async function confirmApply() {
  const current = recheck.value
  if (!current || !current.applicable) return
  applyLoading.value = true
  try {
    const derived = await conflicts.applySuggestion(current.conflict_id, {
      snapshot_hash: current.snapshot_hash,
      participants: current.participants,
    })
    reviewOpen.value = false
    ElMessage.success(`已生成草稿提案 #${derived.id}，冲突 #${current.conflict_id} 已结案`)
    await load()
  } catch (error) {
    const details = (error as { response?: { data?: { error?: { details?: SuggestionApplyRejection } } } })
      ?.response?.data?.error?.details
    if (details) applyRejection.value = details
  } finally {
    applyLoading.value = false
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
        <el-table-column label="动作" width="270"><template #default="scope"><div class="conflict-actions"><el-button text @click="showEvidence(scope.row)">证据</el-button><template v-if="auth.hasRole('reviewer', 'admin')"><el-button v-if="scope.row.conflict_state === 'detected'" text type="primary" @click="transition(scope.row, 'confirmed')"><Check :size="14" />确认</el-button><el-button v-if="scope.row.conflict_state === 'detected'" text type="warning" @click="transition(scope.row, 'false_positive')"><X :size="14" />误报</el-button><el-button v-if="scope.row.conflict_state === 'confirmed'" text type="primary" @click="transition(scope.row, 'resolution_proposed')"><FilePlus :size="14" />准备建议</el-button><el-button v-if="scope.row.conflict_state === 'resolution_proposed'" text type="primary" @click="applySuggestion(scope.row)"><ShieldCheck :size="14" />复核并应用</el-button><el-button v-if="scope.row.conflict_state === 'false_positive' || scope.row.conflict_state === 'resolved'" text @click="transition(scope.row, 'closed')"><X :size="14" />关闭</el-button></template></div></template></el-table-column>
      </el-table>
      <div v-if="!conflicts.loading && !conflicts.items.length" class="empty-state"><div><strong>暂无冲突</strong><span>选择一个提案运行检测，系统会记录算法版本和输入哈希。</span></div></div>
    </div>
  </section>

  <el-dialog v-model="detectOpen" title="检测提案拓扑" width="min(480px, calc(100vw - 28px))">
    <el-form label-position="top"><el-form-item label="边界提案"><el-select v-model="form.proposal_id" placeholder="选择提案" style="width: 100%"><el-option v-for="item in proposals.items" :key="item.id" :label="`提案 #${item.id} · ${item.proposal_state}`" :value="item.id" /></el-select></el-form-item><el-alert type="warning" :closable="false" title="检测是离线决策支持，不会修改原始地块边界或法定登记。" /></el-form>
    <template #footer><el-button @click="detectOpen = false">取消</el-button><el-button type="primary" :disabled="!form.proposal_id" @click="detect">开始检测</el-button></template>
  </el-dialog>

  <el-dialog v-model="reviewOpen" title="复核吸附建议" width="min(760px, calc(100vw - 28px))">
    <div v-loading="reviewLoading" class="review-body">
      <template v-if="recheck">
        <p class="review-lead">冲突 #{{ recheck.conflict_id }} · 提案 #{{ recheck.proposal_id }} · {{ recheck.parcel_code }}（{{ recheck.coordinate_system }}，容差 {{ recheck.snap_tolerance_m }} m）。应用只会基于吸附证据创建新的草稿提案并结案当前冲突，不改写原提案或地块边界。</p>
        <div class="review-metrics">
          <div class="review-metric"><span>面积增减</span><strong :class="deltaClass">{{ formatDelta(recheck.area_delta_square_m) }} m²</strong></div>
          <div class="review-metric"><span>坐标移动</span><strong>{{ recheck.moved_coordinate_count }} 处</strong></div>
          <div class="review-metric"><span>残留冲突</span><strong :class="recheck.residual_conflicts.length ? 'delta-negative' : 'delta-positive'">{{ recheck.residual_conflicts.length }} 条</strong></div>
          <div class="review-metric"><span>基线版本</span><strong>v{{ recheck.base_version }}</strong></div>
        </div>
        <el-alert v-if="recheck.detection_stale" type="warning" :closable="false" title="检测输入快照已过期：当前地块数据与检测时不一致，本对话框以实时重算为准。" />
        <el-alert v-if="!recheck.applicable" type="error" :closable="false" title="重算后仍存在残留冲突，不能生成草稿；冲突保持 resolution_proposed。" />
        <el-alert v-if="applyRejection" type="error" :closable="false" :title="rejectionTitle">
          <ul class="rejection-list">
            <li v-for="change in applyRejection.changed_participants ?? []" :key="`changed-${change.parcel_id}`">地块 {{ change.parcel_code }} 版本 v{{ change.submitted_boundary_version }} → v{{ change.current_boundary_version }}<template v-if="change.geometry_changed">（几何已变化）</template></li>
            <li v-for="added in applyRejection.added_participants ?? []" :key="`added-${added.parcel_id}`">地块 {{ added.parcel_code }} 新加入参与集合（v{{ added.boundary_version }}）</li>
            <li v-for="removed in applyRejection.removed_participants ?? []" :key="`removed-${removed.parcel_id}`">地块 {{ removed.parcel_code }} 已退出参与集合</li>
            <li v-for="(residual, index) in applyRejection.residual_conflicts ?? []" :key="`residual-${index}`">残留 {{ typeLabel(residual.conflict_type) }} {{ residual.magnitude_square_m.toFixed(2) }} m²（{{ residualLabel(residual) }}）</li>
          </ul>
        </el-alert>
        <h4 class="review-section">参与地块快照</h4>
        <el-table :data="recheck.participants" size="small">
          <el-table-column label="地块" min-width="150"><template #default="scope"><strong>{{ scope.row.parcel_code }}</strong><small class="muted">#{{ scope.row.parcel_id }}</small></template></el-table-column>
          <el-table-column label="边界版本" width="90"><template #default="scope">v{{ scope.row.boundary_version }}</template></el-table-column>
          <el-table-column label="几何哈希" min-width="170"><template #default="scope"><code>{{ scope.row.geometry_hash.slice(0, 16) }}…</code></template></el-table-column>
        </el-table>
        <template v-if="recheck.residual_conflicts.length">
          <h4 class="review-section">重算残留冲突</h4>
          <el-table :data="recheck.residual_conflicts" size="small">
            <el-table-column label="类型" width="130"><template #default="scope"><span :class="['conflict-tag', `tone-${scope.row.conflict_type}`]">{{ typeLabel(scope.row.conflict_type) }}</span></template></el-table-column>
            <el-table-column label="量级" width="110"><template #default="scope">{{ scope.row.magnitude_square_m.toFixed(2) }} m²</template></el-table-column>
            <el-table-column label="参与地块" min-width="150"><template #default="scope">{{ residualLabel(scope.row) }}</template></el-table-column>
            <el-table-column prop="explanation" label="说明" min-width="200" show-overflow-tooltip />
          </el-table>
        </template>
      </template>
    </div>
    <template #footer>
      <el-button @click="reviewOpen = false">取消</el-button>
      <el-button :disabled="reviewLoading || applyLoading" @click="runRecheck"><RefreshCw :size="14" />重新核对</el-button>
      <el-button type="primary" :loading="applyLoading" :disabled="!recheck || !recheck.applicable || reviewLoading" @click="confirmApply"><ShieldCheck :size="14" />生成草稿并结案</el-button>
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
.review-body { min-height: 220px; }
.review-lead { margin: 0 0 14px; color: var(--text-muted); font-size: 13px; line-height: 1.6; }
.review-metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(130px, 1fr)); gap: 8px; margin-bottom: 14px; }
.review-metric { border: 1px solid var(--line-strong); padding: 10px 12px; background: var(--surface-muted, #f4f5f4); }
.review-metric span { display: block; font-size: 11px; color: var(--text-muted); letter-spacing: 0.04em; }
.review-metric strong { display: block; margin-top: 4px; font-size: 18px; font-variant-numeric: tabular-nums; }
.delta-positive { color: #17604e; }.delta-negative { color: #9c3028; }
.review-section { margin: 16px 0 8px; font-size: 13px; letter-spacing: 0.04em; }
.rejection-list { margin: 8px 0 0; padding-left: 18px; line-height: 1.7; }
.el-alert { margin-bottom: 12px; }
</style>
