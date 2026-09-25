<script setup lang="ts">
import { proposalStateLabel, type ProposalState } from '@/types/enums/proposal-state'
defineProps<{ state: ProposalState | null }>()
</script>
<template><span v-if="state" :class="['proposal-state', `state-${state}`]">{{ proposalStateLabel[state] }}</span><span v-else class="proposal-state">提案缺失</span></template>
<style scoped>
.proposal-state { display:inline-flex; min-height:24px; align-items:center; padding:2px 8px; border:1px solid var(--line-strong); border-radius:3px; color:var(--text-muted); background:var(--surface-strong); font-size:12px; font-weight:800; white-space:nowrap; }
.state-validated,.state-submitted { color:#2a5d78; border-color:#9bc1d6; background:#e8f2f7; }.state-reviewed { color:#755310; border-color:#e0c16b; background:#fff5db; }.state-accepted { color:#17604e; border-color:#9dcebd; background:#e8f4f0; }.state-rejected { color:#9c3028; border-color:#e6afaa; background:#fbeceb; }
</style>
