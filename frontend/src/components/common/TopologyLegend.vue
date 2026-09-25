<script setup lang="ts">
import { conflictTypeLabel, type ConflictType } from '@/types/enums/conflict-type'
defineProps<{ active?: ConflictType | '' }>()
</script>
<template>
  <div class="topology-legend" aria-label="拓扑冲突图例">
    <span v-for="(label, type) in conflictTypeLabel" :key="type" :class="['legend-item', { active: active === type }]">
      <i :class="`marker marker-${type}`" />{{ label }}
    </span>
  </div>
</template>
<style scoped>
.topology-legend { display:flex; flex-wrap:wrap; gap:10px 16px; color:var(--text-muted); font-size:12px; }
.legend-item { display:inline-flex; align-items:center; gap:6px; padding:3px 5px; border:1px solid transparent; }
.legend-item.active { color:var(--text); border-color:var(--line-strong); background:var(--surface-strong); }
.marker { width:8px; height:8px; display:inline-block; border-radius:50%; background:#a43a32; }
.marker-gap { background:#a66d0b; }.marker-self_intersection { background:#7b4c9e; }.marker-dangling_edge { background:#2b6f96; }
</style>
