<script setup lang="ts">
import { ref, computed } from 'vue'
import CCAGrid from '../components/CCAGrid.vue'
import CCATable from '../components/CCATable.vue'
import type { Course } from '@/types'

interface CourseWithSelection extends Course {
  selected: boolean
}

const props = defineProps<{ ccas: CourseWithSelection[] }>()
const emit = defineEmits<{ toggle: [id: string] }>()

const selectedPeriod = ref<string>('Monday/Wednesday CCA 1')
const viewMode = ref<'grid' | 'table'>('grid')

const filteredCCAs = computed(() => props.ccas.filter(c => c.period === selectedPeriod.value))

const selectedSports = computed(() => props.ccas.filter(c => c.selected && c.category_id === 'Sports').length)
const selectedEnrichment = computed(() => props.ccas.filter(c => c.selected && (c.category_id === 'Arts' || c.category_id === 'Academic' || c.category_id === 'STEM')).length)
</script>

<template>
  <div class="flex flex-1">
    <aside class="w-56 border-r border-gray-200 bg-white p-8 space-y-8">
      <div>
        <h3 class="text-sm font-medium mb-3 text-gray-900">Monday/Wednesday</h3>
        <ul class="space-y-2 text-sm text-gray-600">
          <li @click="selectedPeriod = 'Monday/Wednesday CCA 1'" class="cursor-pointer hover:text-gray-900" :class="selectedPeriod === 'Monday/Wednesday CCA 1' ? 'text-[#5bae31] font-medium' : ''">CCA 1</li>
          <li @click="selectedPeriod = 'Monday/Wednesday CCA 2'" class="cursor-pointer hover:text-gray-900" :class="selectedPeriod === 'Monday/Wednesday CCA 2' ? 'text-[#5bae31] font-medium' : ''">CCA 2</li>
          <li @click="selectedPeriod = 'Monday/Wednesday CCA 3'" class="cursor-pointer hover:text-gray-900" :class="selectedPeriod === 'Monday/Wednesday CCA 3' ? 'text-[#5bae31] font-medium' : ''">CCA 3</li>
        </ul>
      </div>
      <div>
        <h3 class="text-sm font-medium mb-3 text-gray-900">Tuesday/Thursday</h3>
        <ul class="space-y-2 text-sm text-gray-600">
          <li @click="selectedPeriod = 'Tuesday/Thursday CCA 1'" class="cursor-pointer hover:text-gray-900" :class="selectedPeriod === 'Tuesday/Thursday CCA 1' ? 'text-[#5bae31] font-medium' : ''">CCA 1</li>
          <li @click="selectedPeriod = 'Tuesday/Thursday CCA 2'" class="cursor-pointer hover:text-gray-900" :class="selectedPeriod === 'Tuesday/Thursday CCA 2' ? 'text-[#5bae31] font-medium' : ''">CCA 2</li>
          <li @click="selectedPeriod = 'Tuesday/Thursday CCA 3'" class="cursor-pointer hover:text-gray-900" :class="selectedPeriod === 'Tuesday/Thursday CCA 3' ? 'text-[#5bae31] font-medium' : ''">CCA 3</li>
        </ul>
      </div>
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

      <CCAGrid v-if="viewMode === 'grid'" :ccas="filteredCCAs" @toggle="emit('toggle', $event)" />
      <CCATable v-else :ccas="filteredCCAs" @toggle="emit('toggle', $event)" />
    </main>
  </div>
</template>
