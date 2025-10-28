<script setup lang="ts">
import { ref, onMounted } from 'vue'
import SelectionPage from './pages/SelectionPage.vue'
import ReviewPage from './pages/ReviewPage.vue'
import type { Course } from './types'

interface CourseWithSelection extends Course {
  selected: boolean
}

const activeTab = ref<'Selection' | 'Review'>('Selection')
const ccas = ref<CourseWithSelection[]>([])

onMounted(async () => {
  const res = await fetch('/student/api/courses', {
    credentials: 'include'
  })
  const data = await res.json()
  ccas.value = data.map((c: any) => ({ ...c, current_students: 0, selected: false }))
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
</script>

<template>
  <div class="min-h-screen bg-white flex flex-col">
    <header class="border-b border-gray-200 bg-white/80 backdrop-blur-sm sticky top-0 z-50">
      <div class="flex justify-between items-center px-8 py-5">
        <h1 class="text-xl font-light tracking-wide">CCA Selection</h1>
        <button class="px-6 py-2 text-sm border border-[#5bae31] rounded text-[#5bae31] hover:bg-[#5bae31] hover:text-white transition-colors">Login</button>
      </div>
    </header>

    <div class="border-b border-gray-200 bg-white">
      <div class="flex gap-12 px-8 py-4">
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
    </div>

    <SelectionPage v-if="activeTab === 'Selection'" :ccas="ccas" @toggle="toggleCCA" />
    <ReviewPage v-else :ccas="ccas" />
  </div>
</template>