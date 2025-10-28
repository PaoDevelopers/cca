<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import SelectionPage from './pages/SelectionPage.vue'
import ReviewPage from './pages/ReviewPage.vue'
import type { Course, Student } from './types'

interface CourseWithSelection extends Course {
  selected: boolean
}

const activeTab = ref<'Selection' | 'Review'>('Selection')
const ccas = ref<CourseWithSelection[]>([])
const userInfo = ref<Student | null>(null)
const searchQuery = ref<string>('')
const searchScope = ref<'global' | 'period'>('global')
const currentPeriod = ref<string>('')

onMounted(async () => {
  const [coursesRes, userRes] = await Promise.all([
    fetch('/student/api/courses', { credentials: 'include' }),
    fetch('/student/api/user_info', { credentials: 'include' })
  ])
  ccas.value = (await coursesRes.json()).map((c: any) => ({ ...c, current_students: 0, selected: false }))
  userInfo.value = await userRes.json()
})

const toggleCCA = (id: string) => {
  const cca = ccas.value.find((c: CourseWithSelection) => c.id === id)
  if (!cca) return

  if (cca.selected) {
    cca.selected = false
  } else {
    ccas.value.forEach((c: CourseWithSelection) => {
      if (c.period === cca.period) c.selected = false
    })
    cca.selected = true
  }
}

const filteredCCAs = computed(() => {
  if (!searchQuery.value) return ccas.value

  const query = searchQuery.value.toLowerCase()
  let filtered = ccas.value

  if (searchScope.value === 'period' && currentPeriod.value) {
    filtered = filtered.filter(c => c.period === currentPeriod.value)
  }

  return filtered.filter(c =>
    c.name.toLowerCase().includes(query) ||
    c.id.toLowerCase().includes(query) ||
    c.description.toLowerCase().includes(query) ||
    c.teacher.toLowerCase().includes(query) ||
    c.location.toLowerCase().includes(query)
  )
})
</script>

<template>
  <div class="min-h-screen bg-white flex flex-col">
    <header class="border-b border-gray-200 bg-white/80 backdrop-blur-sm sticky top-0 z-50">
      <div class="flex justify-between items-center px-8 py-5">
        <h1 class="text-xl font-light tracking-wide">CCA Selection</h1>
        <div v-if="userInfo" class="flex items-center gap-3 text-sm">
          <span class="text-gray-900 font-medium">{{ userInfo.name }}</span>
          <span class="text-gray-400">·</span>
          <span class="text-gray-600">{{ userInfo.grade }}</span>
          <span class="text-gray-400">·</span>
          <span class="text-gray-600">ID: {{ userInfo.id }}</span>
        </div>
      </div>
    </header>

    <div class="border-b border-gray-200 bg-white">
      <div class="flex justify-between items-center px-8 py-4">
        <div class="flex gap-12">
          <button
            @click="activeTab = 'Selection'"
            class="text-sm pb-2 transition-colors"
            :class="activeTab === 'Selection' ? 'border-b-2 border-[#5bae31] text-[#5bae31]' : 'text-gray-500 hover:text-gray-900'"
          >
            Selection
          </button>
          <button
            @click="activeTab = 'Review'"
            class="text-sm pb-2 transition-colors"
            :class="activeTab === 'Review' ? 'border-b-2 border-[#5bae31] text-[#5bae31]' : 'text-gray-500 hover:text-gray-900'"
          >
            Review
          </button>
        </div>
        <div class="flex gap-2 items-center">
          <select v-model="searchScope" class="text-xs border border-gray-300 rounded px-2 py-1.5">
            <option value="global">Search globally</option>
            <option value="period" v-if="currentPeriod">Search in {{ currentPeriod }}</option>
          </select>
          <input v-model="searchQuery" type="text" placeholder="Search CCAs..." class="text-sm border border-gray-300 rounded px-3 py-1.5 w-64" />
        </div>
      </div>
    </div>

    <SelectionPage v-if="activeTab === 'Selection'" :ccas="filteredCCAs" :search-active="searchScope === 'global' && !!searchQuery" :user-grade="userInfo?.grade" @toggle="toggleCCA" @period-change="currentPeriod = $event" />
    <ReviewPage v-else :ccas="ccas" />
  </div>
</template>
