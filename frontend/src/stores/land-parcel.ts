import { defineStore } from 'pinia'
import { ref } from 'vue'
import { landParcelApi, type ParcelInput, type ParcelQuery } from '@/api/land-parcel'
import type { LandParcel } from '@/types/land-parcel'

export const useLandParcelStore = defineStore('land-parcels', () => {
  const items = ref<LandParcel[]>([])
  const loading = ref(false)

  async function fetch(params?: ParcelQuery) {
    loading.value = true
    try {
      const { data } = await landParcelApi.list(params)
      items.value = data.data
    } finally {
      loading.value = false
    }
  }

  async function create(body: ParcelInput) {
    const { data } = await landParcelApi.create(body)
    items.value.unshift(data.data)
    return data.data
  }

  async function update(id: number, body: ParcelInput) {
    const { data } = await landParcelApi.update(id, body)
    replace(data.data)
    return data.data
  }

  function replace(item: LandParcel) {
    const index = items.value.findIndex((current) => current.id === item.id)
    if (index >= 0) items.value[index] = item
  }

  return { items, loading, fetch, create, update }
})
