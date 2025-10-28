<script setup lang="ts">
import { ref, computed } from 'vue'
import type { Course } from '@/types'

interface CourseWithSelection extends Course {
  selected: boolean
}

const props = defineProps<{ ccas: CourseWithSelection[] }>()

const confirmed = ref(false)
const selectedCourses = computed(() => props.ccas.filter(c => c.selected))

const timetable = computed(() => {
  const table: Record<string, Record<string, CourseWithSelection | null>> = {
    '1': { Monday: null, Tuesday: null, Wednesday: null, Thursday: null },
    '2': { Monday: null, Tuesday: null, Wednesday: null, Thursday: null },
    '3': { Monday: null, Tuesday: null, Wednesday: null, Thursday: null }
  }

  selectedCourses.value.forEach(course => {
    const match = course.period.match(/^(MW|TT)(\d)$/)
    if (match) {
      const [, days, slot] = match
      if (days === 'MW') {
        table[slot].Monday = course
        table[slot].Wednesday = course
      } else {
        table[slot].Tuesday = course
        table[slot].Thursday = course
      }
    }
  })

  return table
})
</script>

<template>
  <div class="flex-1 p-8 bg-gray-50/30">
    <div class="max-w-4xl mx-auto">
      <h2 class="text-2xl font-light mb-8">Your Selections</h2>

      <div class="bg-white border-2 border-gray-300 rounded-lg overflow-hidden">
        <table class="w-full border-collapse">
          <thead class="border-b-2 border-gray-300 bg-gray-50">
            <tr>
              <th class="text-left p-6 font-medium border-r-2 border-gray-300">Period</th>
              <th class="text-left p-6 font-medium border-r-2 border-gray-300">Monday</th>
              <th class="text-left p-6 font-medium border-r-2 border-gray-300">Tuesday</th>
              <th class="text-left p-6 font-medium border-r-2 border-gray-300">Wednesday</th>
              <th class="text-left p-6 font-medium">Thursday</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(slot, index) in ['1', '2', '3']" :key="slot" :class="index < 2 ? 'border-b-2 border-gray-300' : ''">
              <td class="p-6 font-medium border-r-2 border-gray-300">CCA {{ slot }}</td>
              <td class="p-6 border-r-2 border-gray-300">{{ timetable[slot].Monday?.name || '-' }}</td>
              <td class="p-6 border-r-2 border-gray-300">{{ timetable[slot].Tuesday?.name || '-' }}</td>
              <td class="p-6 border-r-2 border-gray-300">{{ timetable[slot].Wednesday?.name || '-' }}</td>
              <td class="p-6">{{ timetable[slot].Thursday?.name || '-' }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="mt-8 flex justify-end">
        <button
          @click="confirmed = !confirmed"
          class="px-8 py-3 rounded transition-colors"
          :class="confirmed ? 'bg-gray-500 text-white hover:bg-gray-600' : 'bg-[#5bae31] text-white hover:bg-[#4a9428]'"
        >
          {{ confirmed ? 'Unconfirm' : 'Confirm' }}
        </button>
      </div>
    </div>
  </div>
</template>
