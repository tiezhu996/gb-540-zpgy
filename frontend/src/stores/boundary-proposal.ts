import { defineStore } from 'pinia'
import { ref } from 'vue'
import { boundaryProposalApi, type ProposalCreateInput, type ProposalQuery, type ProposalTransitionInput } from '@/api/boundary-proposal'
import type { BoundaryProposal } from '@/types/boundary-proposal'

export const useBoundaryProposalStore = defineStore('boundary-proposals', () => {
  const items = ref<BoundaryProposal[]>([])
  const loading = ref(false)

  async function fetch(params?: ProposalQuery) {
    loading.value = true
    try {
      const { data } = await boundaryProposalApi.list(params)
      items.value = data.data
    } finally {
      loading.value = false
    }
  }

  async function create(body: ProposalCreateInput) {
    const { data } = await boundaryProposalApi.create(body)
    items.value.unshift(data.data)
    return data.data
  }

  async function transition(id: number, body: ProposalTransitionInput) {
    const { data } = await boundaryProposalApi.transition(id, body)
    replace(data.data)
    return data.data
  }

  function replace(item: BoundaryProposal) {
    const index = items.value.findIndex((current) => current.id === item.id)
    if (index >= 0) items.value[index] = item
  }

  return { items, loading, fetch, create, transition }
})
