<script setup lang="ts">
import type { Course } from '@/types'

interface CourseWithSelection extends Course {
  selected: boolean
}

defineProps<{ cca: CourseWithSelection }>()
const emit = defineEmits<{ toggle: [id: string] }>()
</script>

<template>
  <div class="bg-white border border-gray-200 rounded-lg p-6 hover:border-[#5bae31] transition-colors relative">
    <div class="flex justify-between items-start mb-3">
      <h3 class="text-lg font-semibold pr-12">{{ cca.name }}</h3>
      <button
        @click="emit('toggle', cca.id)"
        class="w-8 h-8 flex items-center justify-center border rounded transition-colors flex-shrink-0"
        :class="cca.selected ? 'bg-[#5bae31] border-[#5bae31] text-white' : 'border-gray-400 text-gray-600 hover:border-[#5bae31] hover:text-[#5bae31]'"
      >
        <svg v-if="cca.selected" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
        </svg>
        <span v-else class="text-lg">+</span>
      </button>
    </div>

    <p class="text-xs text-gray-500 mb-3">{{ cca.id }}</p>
    <p class="text-sm text-gray-700 mb-4 leading-relaxed">{{ cca.description }}</p>

    <div class="space-y-1.5 text-sm">
      <div class="flex justify-between">
        <span class="text-gray-500">Teacher</span>
        <span class="font-medium">{{ cca.teacher }}</span>
      </div>
      <div class="flex justify-between">
        <span class="text-gray-500">Location</span>
        <span class="font-medium">{{ cca.location }}</span>
      </div>
      <div class="flex justify-between">
        <span class="text-gray-500">Capacity</span>
        <span class="font-medium">{{ cca.current_students }}/{{ cca.max_students }}</span>
      </div>
      <div v-if="cca.membership === 'invite_only'" class="flex justify-between">
        <span class="text-gray-500">Membership</span>
        <span class="text-xs font-medium text-amber-600 uppercase">Invite Only</span>
      </div>
    </div>
  </div>
</template>
