<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Search, ScrollText } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import GeometryEvidenceDrawer from '@/components/common/GeometryEvidenceDrawer.vue'
import { useAuditStore } from '@/stores/audit'
import type { AuditLog } from '@/types/audit'

const audit = useAuditStore()
const selected = ref<AuditLog | null>(null)
const evidenceOpen = ref(false)
const filters = reactive({ request_id: '', entity: '', actor: '', timeRange: [] as string[] })
const entityOptions = [
  { value: 'LandParcel', label: '地块' },
  { value: 'SurveyObservation', label: '测量观测' },
  { value: 'BoundaryProposal', label: '边界提案' },
  { value: 'TopologyConflict', label: '拓扑冲突' },
]

const selectedGeometry = computed(() => selected.value ? geometryFor(selected.value) : undefined)

function toISOString(value?: string) {
  if (!value) return undefined
  const date = new Date(value)
  return Number.isNaN(date.valueOf()) ? undefined : date.toISOString()
}

async function search() {
  const [from, to] = filters.timeRange
  await audit.fetch({
    request_id: filters.request_id.trim() || undefined,
    entity: filters.entity || undefined,
    actor: filters.actor.trim() || undefined,
    from: toISOString(from),
    to: toISOString(to),
    page_size: 100,
  })
}

function entityName(item: AuditLog) {
  return item.entity ?? item.resource_type ?? '未知实体'
}

function summary(value: string) {
  try {
    const text = JSON.stringify(JSON.parse(value))
    return text.length > 110 ? `${text.slice(0, 110)}…` : text
  } catch {
    return value
  }
}

function geometryFromSnapshot(snapshot: string) {
  try {
    const parsed = JSON.parse(snapshot) as Record<string, unknown>
    for (const key of ['boundary_geojson', 'point_geojson', 'proposed_geojson', 'geometry_geojson']) {
      const geometry = parsed[key]
      if (typeof geometry === 'string') return geometry
      if (geometry && typeof geometry === 'object') return JSON.stringify(geometry)
    }
  } catch {
    return undefined
  }
  return undefined
}

function geometryFor(item: AuditLog) {
  return geometryFromSnapshot(item.after) ?? geometryFromSnapshot(item.before)
}

function showEvidence(item: AuditLog) {
  if (!geometryFor(item)) return
  selected.value = item
  evidenceOpen.value = true
}

onMounted(search)
</script>

<template>
  <PageHeader title="审计中心" eyebrow="IMMUTABLE AUDIT" description="按 request ID、实体、操作者和时间回溯四类地籍变更。" />

  <section class="content-band">
    <div class="audit-filters">
      <el-input v-model="filters.request_id" clearable placeholder="Request ID" @keyup.enter="search" />
      <el-select v-model="filters.entity" clearable placeholder="全部实体"><el-option v-for="entity in entityOptions" :key="entity.value" :label="entity.label" :value="entity.value" /></el-select>
      <el-input v-model="filters.actor" clearable placeholder="操作者" @keyup.enter="search" />
      <el-date-picker v-model="filters.timeRange" type="datetimerange" value-format="YYYY-MM-DDTHH:mm:ss" range-separator="至" start-placeholder="开始时间" end-placeholder="结束时间" />
      <el-button type="primary" @click="search"><Search :size="15" />检索</el-button>
      <span class="toolbar-spacer subtle-count">{{ audit.total }} 条记录</span>
    </div>

    <div class="data-surface">
      <el-table v-loading="audit.loading" :data="audit.items" row-key="id">
        <el-table-column prop="created_at" label="时间" width="170"><template #default="scope">{{ new Date(scope.row.created_at).toLocaleString() }}</template></el-table-column>
        <el-table-column prop="actor_name" label="操作者" width="120"><template #default="scope"><strong>{{ scope.row.actor_name }}</strong></template></el-table-column>
        <el-table-column label="实体" width="150"><template #default="scope"><span class="audit-entity">{{ entityName(scope.row) }} #{{ scope.row.resource_id }}</span></template></el-table-column>
        <el-table-column prop="action" label="动作" width="155"><template #default="scope"><span class="audit-action">{{ scope.row.action }}</span></template></el-table-column>
        <el-table-column label="前值摘要" min-width="180"><template #default="scope"><code>{{ summary(scope.row.before) }}</code></template></el-table-column>
        <el-table-column label="后值摘要" min-width="180"><template #default="scope"><code>{{ summary(scope.row.after) }}</code></template></el-table-column>
        <el-table-column prop="request_id" label="Request ID" width="205"><template #default="scope"><span class="request-id">{{ scope.row.request_id }}</span></template></el-table-column>
        <el-table-column label="证据" width="90"><template #default="scope"><el-button text :disabled="!geometryFor(scope.row)" @click="showEvidence(scope.row)">几何</el-button></template></el-table-column>
        <template #empty><div class="empty-state"><div><ScrollText :size="34" /><strong>没有审计记录</strong><span>放宽检索条件后重试。</span></div></div></template>
      </el-table>
    </div>
  </section>

  <GeometryEvidenceDrawer v-model="evidenceOpen" title="审计几何证据" :geometry="selectedGeometry" :explanation="selected ? `${entityName(selected)} #${selected.resource_id} · ${selected.request_id}` : ''" />
</template>

<style scoped>
.audit-filters { display: flex; align-items: center; flex-wrap: wrap; gap: 9px; margin-bottom: 14px; }
.audit-filters .el-select, .audit-filters .el-input { width: 170px; }
.audit-action { color: var(--accent); font-size: 12px; font-weight: 800; }.audit-entity { color: var(--text-muted); font-size: 12px; }
.request-id, code { display: block; overflow: hidden; color: var(--text-muted); font-family: "SFMono-Regular", Consolas, monospace; font-size: 11px; white-space: nowrap; text-overflow: ellipsis; }.request-id { max-width: 190px; } code { color: #596660; }
@media (max-width: 620px) { .audit-filters .el-select, .audit-filters .el-input { width: calc(50% - 5px); } .audit-filters :deep(.el-date-editor) { width: 100%; } }
</style>
