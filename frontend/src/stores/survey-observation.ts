import { defineStore } from 'pinia'
import { ref } from 'vue'
import { surveyObservationApi, type ObservationImportInput, type ObservationQuery, type ObservationTransitionInput } from '@/api/survey-observation'
import type { SurveyObservation } from '@/types/survey-observation'

export const useSurveyObservationStore = defineStore('survey-observations', () => {
  const items = ref<SurveyObservation[]>([])
  const loading = ref(false)

  async function fetch(params?: ObservationQuery) {
    loading.value = true
    try {
      const { data } = await surveyObservationApi.list(params)
      items.value = data.data
    } finally {
      loading.value = false
    }
  }

  async function importObservation(body: ObservationImportInput) {
    const { data } = await surveyObservationApi.import(body)
    items.value.unshift(data.data)
    return data.data
  }

  async function transition(id: number, body: ObservationTransitionInput) {
    const { data } = await surveyObservationApi.transition(id, body)
    const index = items.value.findIndex((item) => item.id === id)
    if (index >= 0) items.value[index] = data.data
    return data.data
  }

  return { items, loading, fetch, importObservation, transition }
})
