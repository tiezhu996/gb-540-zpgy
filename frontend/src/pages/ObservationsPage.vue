<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Check, Import, RefreshCw, X } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import GeometryEvidenceDrawer from '@/components/common/GeometryEvidenceDrawer.vue'
import { useLandParcelStore } from '@/stores/land-parcel'
import { useSurveyObservationStore } from '@/stores/survey-observation'
import { useAuth } from '@/hooks/useAuth'
import type { SurveyObservation } from '@/types/survey-observation'

const parcels = useLandParcelStore()
const observations = useSurveyObservationStore()
const auth = useAuth()
const importOpen = ref(false)
const evidenceOpen = ref(false)
const supersedeOpen = ref(false)
const selected = ref<SurveyObservation | null>(null)
const superseded = ref<SurveyObservation | null>(null)
const form = reactive({
  parcel_id: 0,
  observation_code: '',
  point_geojson: '{"type":"Point","coordinates":[50,50]}',
  observed_at: new Date().toISOString().slice(0, 16),
  method: 'GNSS',
  horizontal_accuracy_m: 0.05,
  source_checksum: '',
  quality_note: '',
})
const supersedeForm = reactive({ replacement_observation_id: 0, quality_note: '' })

const replacementOptions = computed(() => {
  if (!superseded.value) return []
  return observations.items.filter((item) => item.id !== superseded.value?.id && item.parcel_id === superseded.value?.parcel_id && item.observation_state !== 'superseded')
})

async function load() {
  await Promise.all([parcels.fetch({ page_size: 100 }), observations.fetch({ page_size: 100 })])
}

async function importObservation() {
  await observations.importObservation({ ...form, observed_at: new Date(form.observed_at).toISOString() })
  importOpen.value = false
  await load()
}

function showEvidence(item: SurveyObservation) {
  selected.value = item
  evidenceOpen.value = true
}

async function transition(item: SurveyObservation, to: 'accepted' | 'rejected') {
  await observations.transition(item.id, { to, version: item.version })
  await load()
}

function openSupersede(item: SurveyObservation) {
  superseded.value = item
  supersedeForm.replacement_observation_id = 0
  supersedeForm.quality_note = ''
  supersedeOpen.value = true
}

async function supersede() {
  if (!superseded.value || !supersedeForm.replacement_observation_id) return
  await observations.transition(superseded.value.id, {
    to: 'superseded',
    version: superseded.value.version,
    replacement_observation_id: supersedeForm.replacement_observation_id,
    quality_note: supersedeForm.quality_note || undefined,
  })
  supersedeOpen.value = false
  await load()
}

onMounted(load)
</script>

<template>
  <PageHeader title="测量观测" eyebrow="SURVEY OBSERVATIONS" description="导入点位、精度与来源校验，作为边界提案的可追溯证据。">
    <el-button v-if="auth.hasRole('surveyor', 'gis_analyst', 'admin')" type="primary" @click="importOpen = true"><Import :size="15" />导入观测</el-button>
  </PageHeader>

  <section class="content-band">
    <div class="toolbar"><el-button @click="load"><RefreshCw :size="15" />刷新</el-button><span class="toolbar-spacer subtle-count">{{ observations.items.length }} 条观测</span></div>
    <div class="data-surface">
      <el-table v-loading="observations.loading" :data="observations.items" row-key="id">
        <el-table-column prop="observation_code" label="观测编号" width="160"><template #default="scope"><strong>{{ scope.row.observation_code }}</strong></template></el-table-column>
        <el-table-column label="地块" width="125"><template #default="scope">#{{ scope.row.parcel_id }}</template></el-table-column>
        <el-table-column prop="method" label="方法" width="100" />
        <el-table-column label="水平精度" width="120"><template #default="scope">±{{ scope.row.horizontal_accuracy_m }} m</template></el-table-column>
        <el-table-column label="状态" width="125"><template #default="scope"><span>{{ scope.row.observation_state }}</span><small v-if="scope.row.replaced_by" class="muted">替代 #{{ scope.row.replaced_by }}</small></template></el-table-column>
        <el-table-column prop="observed_at" label="观测时间" min-width="170" />
        <el-table-column label="动作" width="230"><template #default="scope"><div class="observation-actions"><el-button text @click="showEvidence(scope.row)">证据</el-button><template v-if="auth.hasRole('surveyor', 'gis_analyst', 'admin')"><el-button v-if="scope.row.observation_state === 'accepted'" text type="danger" @click="transition(scope.row, 'rejected')"><X :size="14" />拒绝</el-button><el-button v-if="scope.row.observation_state === 'rejected'" text type="primary" @click="transition(scope.row, 'accepted')"><Check :size="14" />接受</el-button><el-button v-if="scope.row.observation_state === 'accepted'" text type="primary" @click="openSupersede(scope.row)">替代</el-button></template></div></template></el-table-column>
      </el-table>
      <div v-if="!observations.loading && !observations.items.length" class="empty-state"><div><strong>暂无观测</strong><span>导入点位后，观测会和对应地块建立证据关系。</span></div></div>
    </div>
  </section>

  <el-dialog v-model="importOpen" title="导入测量观测" width="min(620px, calc(100vw - 28px))">
    <el-form label-position="top">
      <div class="form-grid">
        <el-form-item label="地块"><el-select v-model="form.parcel_id" placeholder="选择地块" style="width: 100%"><el-option v-for="parcel in parcels.items" :key="parcel.id" :label="`${parcel.parcel_code} · ${parcel.name}`" :value="parcel.id" /></el-select></el-form-item>
        <el-form-item label="观测编号"><el-input v-model="form.observation_code" /></el-form-item>
        <el-form-item label="方法"><el-input v-model="form.method" /></el-form-item>
        <el-form-item label="水平精度（m）"><el-input-number v-model="form.horizontal_accuracy_m" :min="0.001" :precision="3" style="width: 100%" /></el-form-item>
      </div>
      <el-form-item label="点位 GeoJSON"><el-input v-model="form.point_geojson" /></el-form-item>
      <el-form-item label="来源 checksum"><el-input v-model="form.source_checksum" /></el-form-item>
      <el-form-item label="观测时间"><el-date-picker v-model="form.observed_at" type="datetime" value-format="YYYY-MM-DDTHH:mm" style="width: 100%" /></el-form-item>
      <el-form-item label="质量说明"><el-input v-model="form.quality_note" type="textarea" :rows="3" maxlength="1000" show-word-limit /></el-form-item>
    </el-form>
    <template #footer><el-button @click="importOpen = false">取消</el-button><el-button type="primary" :disabled="!form.parcel_id || !form.observation_code || !form.source_checksum" @click="importObservation">导入</el-button></template>
  </el-dialog>

  <el-dialog v-model="supersedeOpen" title="替代观测" width="min(520px, calc(100vw - 28px))">
    <el-form label-position="top"><el-form-item label="替代观测"><el-select v-model="supersedeForm.replacement_observation_id" placeholder="选择同一地块的观测" style="width: 100%"><el-option v-for="item in replacementOptions" :key="item.id" :label="`${item.observation_code} · ${item.observation_state}`" :value="item.id" /></el-select></el-form-item><el-form-item label="质量说明"><el-input v-model="supersedeForm.quality_note" type="textarea" :rows="3" maxlength="1000" show-word-limit /></el-form-item></el-form>
    <template #footer><el-button @click="supersedeOpen = false">取消</el-button><el-button type="primary" :disabled="!supersedeForm.replacement_observation_id" @click="supersede">确认替代</el-button></template>
  </el-dialog>

  <GeometryEvidenceDrawer v-model="evidenceOpen" title="观测点位" :geometry="selected?.point_geojson" :explanation="selected ? `${selected.observation_code} · ±${selected.horizontal_accuracy_m} m` : ''" />
</template>

<style scoped>
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 14px; }
.muted { display: block; margin-top: 4px; color: var(--text-muted); font-size: 11px; }
.observation-actions { display: flex; flex-wrap: wrap; gap: 2px; }
@media (max-width: 620px) { .form-grid { grid-template-columns: 1fr; } }
</style>
