<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Plus, RefreshCw, Search } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import TopologyLegend from '@/components/common/TopologyLegend.vue'
import { useLandParcelStore } from '@/stores/land-parcel'
import { useSurveyObservationStore } from '@/stores/survey-observation'
import { useAuth } from '@/hooks/useAuth'

const parcels = useLandParcelStore()
const observations = useSurveyObservationStore()
const auth = useAuth()
const createOpen = ref(false)
const keyword = ref('')
const form = reactive({
  parcel_code: '',
  name: '',
  boundary_geojson: '{"type":"Polygon","coordinates":[[[0,0],[100,0],[100,100],[0,100],[0,0]]]}',
  coordinate_system: 'EPSG:3857',
  owner_org: '',
  parcel_state: 'active',
})

const observationCountByParcel = computed(() => {
  const counts = new Map<number, number>()
  for (const observation of observations.items) {
    counts.set(observation.parcel_id, (counts.get(observation.parcel_id) ?? 0) + 1)
  }
  return counts
})

async function load() {
  await Promise.all([
    parcels.fetch({ keyword: keyword.value || undefined, page_size: 100 }),
    observations.fetch({ page_size: 100 }),
  ])
}

async function create() {
  await parcels.create(form)
  createOpen.value = false
  Object.assign(form, { parcel_code: '', name: '', owner_org: '' })
}

onMounted(load)
</script>

<template>
  <PageHeader title="地块管理" eyebrow="LAND PARCELS" description="维护平面坐标边界、版本和观测证据，原始几何始终可追溯。">
    <el-button v-if="auth.hasRole('surveyor', 'gis_analyst', 'admin')" type="primary" @click="createOpen = true">
      <Plus :size="15" />新建地块
    </el-button>
  </PageHeader>

  <section class="content-band">
    <div class="toolbar">
      <el-input v-model="keyword" clearable placeholder="搜索地块编号、名称或组织" @keyup.enter="load">
        <template #prefix><Search :size="15" /></template>
      </el-input>
      <el-button @click="load"><RefreshCw :size="15" />刷新</el-button>
      <span class="toolbar-spacer subtle-count">{{ parcels.items.length }} 个地块</span>
    </div>

    <div class="data-surface">
      <el-table v-loading="parcels.loading || observations.loading" :data="parcels.items" row-key="id">
        <el-table-column prop="parcel_code" label="地块编号" width="150">
          <template #default="scope"><strong>{{ scope.row.parcel_code }}</strong></template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="170" />
        <el-table-column prop="owner_org" label="责任组织" min-width="150" />
        <el-table-column label="边界" width="145">
          <template #default="scope">
            <span>{{ scope.row.area_square_m.toLocaleString() }} m²</span>
            <small class="muted">v{{ scope.row.boundary_version }} · {{ scope.row.coordinate_system }}</small>
          </template>
        </el-table-column>
        <el-table-column label="观测" width="104">
          <template #default="scope">
            <strong>{{ observationCountByParcel.get(scope.row.id) ?? 0 }}</strong><small class="muted">条证据</small>
          </template>
        </el-table-column>
        <el-table-column prop="parcel_state" label="状态" width="90" />
        <el-table-column label="更新时间" width="170">
          <template #default="scope">{{ new Date(scope.row.updated_at).toLocaleString() }}</template>
        </el-table-column>
      </el-table>
      <div v-if="!parcels.loading && !parcels.items.length" class="empty-state"><div><strong>暂无地块</strong><span>新建或导入一个平面坐标地块后，观测和提案会在这里关联。</span></div></div>
    </div>

    <div class="legend-row"><TopologyLegend /><span>面积由平面坐标 GeoJSON 计算，不接受经纬度直接测量。</span></div>
  </section>

  <el-dialog v-model="createOpen" title="新建地块" width="min(620px, calc(100vw - 28px))">
    <el-form label-position="top">
      <div class="form-grid">
        <el-form-item label="地块编号"><el-input v-model="form.parcel_code" /></el-form-item>
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="坐标系"><el-input v-model="form.coordinate_system" /></el-form-item>
        <el-form-item label="责任组织"><el-input v-model="form.owner_org" /></el-form-item>
      </div>
      <el-form-item label="边界 GeoJSON"><el-input v-model="form.boundary_geojson" type="textarea" :rows="5" /></el-form-item>
      <el-alert type="info" :closable="false" title="只接受闭合、有效 Polygon；EPSG:4326/4490 等经纬度坐标会被拒绝。" />
    </el-form>
    <template #footer><el-button @click="createOpen = false">取消</el-button><el-button type="primary" :disabled="!form.parcel_code || !form.name" @click="create">保存地块</el-button></template>
  </el-dialog>
</template>

<style scoped>
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 14px; }
.muted { display: block; margin-top: 4px; color: var(--text-muted); font-size: 11px; }
.legend-row { display: flex; justify-content: space-between; gap: 16px; margin-top: 15px; color: var(--text-muted); font-size: 12px; }
@media (max-width: 620px) { .form-grid { grid-template-columns: 1fr; } .legend-row { align-items: flex-start; flex-direction: column; } }
</style>
