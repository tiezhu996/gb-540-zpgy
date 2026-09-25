<script setup lang="ts">
import { computed } from 'vue'
const props = defineProps<{ modelValue: boolean; title?: string; geometry?: string; explanation?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
const prettyGeometry = computed(() => { if (!props.geometry) return '暂无几何证据'; try { return JSON.stringify(JSON.parse(props.geometry), null, 2) } catch { return props.geometry } })
</script>
<template>
  <el-drawer :model-value="modelValue" :title="title ?? '几何证据'" size="min(520px, 92vw)" @update:model-value="emit('update:modelValue', $event)">
    <el-alert v-if="explanation" type="info" :closable="false" :title="explanation" />
    <pre class="geometry-preview">{{ prettyGeometry }}</pre>
  </el-drawer>
</template>
<style scoped>.geometry-preview { overflow:auto; margin-top:16px; padding:14px; color:#29463d; background:#edf2ef; border:1px solid var(--line); font-size:12px; line-height:1.55; white-space:pre-wrap; word-break:break-word; }</style>
