import { computed, ref } from 'vue'

export function useTopologyLayers() {
  const activeLayer = ref<'boundary' | 'proposal' | 'conflict'>('boundary')
  const layerLabel = computed(() => ({ boundary: '现状边界', proposal: '提案边界', conflict: '冲突证据' }[activeLayer.value]))
  function selectLayer(layer: 'boundary' | 'proposal' | 'conflict') { activeLayer.value = layer }
  return { activeLayer, layerLabel, selectLayer }
}
