import { defineStore } from 'pinia'
import { ref } from 'vue'
import { topologyConflictApi, type ApplySuggestionInput, type ConflictDetectInput, type ConflictQuery, type ConflictTransitionInput } from '@/api/topology-conflict'
import type { BoundaryProposal } from '@/types/boundary-proposal'
import type { TopologyConflict } from '@/types/topology-conflict'

export const useTopologyConflictStore = defineStore('topology-conflicts', () => {
  const items = ref<TopologyConflict[]>([])
  const loading = ref(false)

  async function fetch(params?: ConflictQuery) {
    loading.value = true
    try {
      const { data } = await topologyConflictApi.list(params)
      items.value = data.data
    } finally {
      loading.value = false
    }
  }

  async function detect(body: ConflictDetectInput, idempotencyKey?: string) {
    const { data } = await topologyConflictApi.detect(body, idempotencyKey)
    items.value = [
      ...data.data,
      ...items.value.filter((current) => !data.data.some((next) => next.id === current.id)),
    ]
    return data.data
  }

  async function transition(id: number, body: ConflictTransitionInput) {
    const { data } = await topologyConflictApi.transition(id, body)
    replace(data.data)
    return data.data
  }

  async function applySuggestion(id: number, body?: ApplySuggestionInput): Promise<BoundaryProposal> {
    const { data } = await topologyConflictApi.applySuggestion(id, body)
    return data.data
  }

  function replace(item: TopologyConflict) {
    const index = items.value.findIndex((current) => current.id === item.id)
    if (index >= 0) items.value[index] = item
  }

  return { items, loading, fetch, detect, transition, applySuggestion }
})
