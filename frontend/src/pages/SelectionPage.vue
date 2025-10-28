<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import CCAGrid from '../components/CCAGrid.vue'
import CCATable from '../components/CCATable.vue'
import type { Course } from '@/types'

interface CourseWithSelection extends Course {
  selected: boolean
}

const props = defineProps<{ ccas: CourseWithSelection[], searchActive: boolean }>()
const emit = defineEmits<{ toggle: [id: string], periodChange: [period: string] }>()

const periods = ref<string[]>([])
const selectedPeriod = ref<string>('')
const viewMode = ref<'grid' | 'table'>('grid')

onMounted(async () => {
  const res = await fetch('/student/api/periods', { credentials: 'include' })
  periods.value = await res.json()
  if (periods.value.length > 0) {
    selectedPeriod.value = periods.value[0]
    emit('periodChange', periods.value[0])
  }
})

const selectPeriod = (period: string) => {
  selectedPeriod.value = period
  emit('periodChange', period)
}

const filteredCCAs = computed(() => props.searchActive ? props.ccas : props.ccas.filter(c => c.period === selectedPeriod.value))

const ccasByPeriod = computed(() => {
  const grouped: Record<string, CourseWithSelection[]> = {}
  props.ccas.forEach(c => {
    if (!grouped[c.period]) grouped[c.period] = []
    grouped[c.period].push(c)
  })
  return grouped
})

const selectedSports = computed(() => props.ccas.filter(c => c.selected && c.category_id === 'Sports').length)
const selectedEnrichment = computed(() => props.ccas.filter(c => c.selected && (c.category_id === 'Arts' || c.category_id === 'Academic' || c.category_id === 'STEM')).length)
</script>

<template>
  <div class="flex flex-1">
    <aside class="w-56 border-r border-gray-200 bg-white p-8">
      <ul class="space-y-2 text-sm text-gray-600">
        <li v-for="period in periods" :key="period" @click="selectPeriod(period)" class="cursor-pointer hover:text-gray-900" :class="selectedPeriod === period ? 'text-[#5bae31] font-medium' : ''">
          {{ period }}
        </li>
      </ul>
    </aside>

    <main class="flex-1 p-8 bg-gray-50/30">
      <div class="flex justify-end mb-6 gap-2">
        <div class="flex gap-3 mr-auto text-xs font-semibold uppercase tracking-wide border border-gray-200 rounded px-4 py-2 bg-white">
          <span class="text-gray-900">{{ selectedSports }}/3 Sport</span>
          <span class="text-gray-300">·</span>
          <span class="text-gray-900">{{ selectedEnrichment }}/1 Enrichment</span>
        </div>
        <button @click="viewMode = 'grid'" class="p-2 border rounded" :class="viewMode === 'grid' ? 'bg-[#5bae31] text-white border-[#5bae31]' : 'border-gray-300 text-gray-600'">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z"/>
          </svg>
        </button>
        <button @click="viewMode = 'table'" class="p-2 border rounded" :class="viewMode === 'table' ? 'bg-[#5bae31] text-white border-[#5bae31]' : 'border-gray-300 text-gray-600'">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 10h18M3 14h18m-9-4v8m-7 0h14a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"/>
          </svg>
        </button>
      </div>

      <template v-if="searchActive">
        <div v-for="(ccas, period) in ccasByPeriod" :key="period" class="mb-8">
          <h2 class="text-lg font-medium mb-4 text-gray-900">{{ period }}</h2>
          <CCAGrid v-if="viewMode === 'grid'" :ccas="ccas" @toggle="emit('toggle', $event)" />
          <CCATable v-else :ccas="ccas" @toggle="emit('toggle', $event)" />
        </div>
      </template>
      <template v-else>
        <CCAGrid v-if="viewMode === 'grid'" :ccas="filteredCCAs" @toggle="emit('toggle', $event)" />
        <CCATable v-else :ccas="filteredCCAs" @toggle="emit('toggle', $event)" />
      </template>
    </main>
  </div>
</template>
