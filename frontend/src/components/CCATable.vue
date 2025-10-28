<script setup lang="ts">
import {computed} from 'vue'
import type {Course} from '@/types'

interface CourseWithSelection extends Course {
    selected: boolean
}

const props = defineProps<{ ccas: CourseWithSelection[] }>()
const emit = defineEmits<{ toggle: [id: string] }>()

const groupedCCAs = computed(() => {
    const groups: Record<string, CourseWithSelection[]> = {}
    props.ccas.forEach(cca => {
        if (!groups[cca.category_id]) groups[cca.category_id] = []
        groups[cca.category_id].push(cca)
    })
    return groups
})
</script>

<template>
    <div class="bg-white border border-gray-200 rounded-lg overflow-hidden">
        <table class="w-full text-sm">
            <thead class="border-b border-gray-200 bg-gray-50">
            <tr>
                <th class="text-left p-4 font-medium w-12"></th>
                <th class="text-left p-4 font-medium">Name</th>
                <th class="text-left p-4 font-medium">Enrollment</th>
                <th class="text-left p-4 font-medium">ID</th>
                <th class="text-left p-4 font-medium">Membership</th>
                <th class="text-left p-4 font-medium">Teacher</th>
                <th class="text-left p-4 font-medium">Location</th>
            </tr>
            </thead>
            <tbody>
            <template v-for="(ccas, category) in groupedCCAs" :key="category">
                <tr class="bg-gray-100">
                    <td colspan="7" class="p-3 font-medium text-sm">{{ category }}</td>
                </tr>
                <tr v-for="cca in ccas" :key="cca.id" class="border-b border-gray-200"
                    :class="cca.current_students >= cca.max_students && !cca.selected ? 'opacity-50' : 'hover:bg-gray-50'">
                    <td class="p-4">
                        <button
                            @click="emit('toggle', cca.id)"
                            :disabled="cca.current_students >= cca.max_students && !cca.selected"
                            class="w-8 h-8 flex items-center justify-center border rounded transition-colors"
                            :class="cca.selected ? 'bg-[#5bae31] border-[#5bae31] text-white' : (cca.current_students >= cca.max_students ? 'border-gray-300 text-gray-400 cursor-not-allowed' : 'border-gray-300 text-gray-400 hover:border-[#5bae31] hover:text-[#5bae31]')"
                        >
                            <svg v-if="cca.selected" class="w-4 h-4" fill="none" stroke="currentColor"
                                 viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                                      d="M5 13l4 4L19 7"/>
                            </svg>
                            <span v-else class="text-lg">+</span>
                        </button>
                    </td>
                    <td class="p-4 font-medium">{{ cca.name }}</td>
                    <td class="p-4 text-gray-600">{{ cca.current_students }}/{{ cca.max_students }} <span v-if="cca.current_students >= cca.max_students" class="text-red-500">(Full!)</span></td>
                    <td class="p-4 text-gray-600">{{ cca.id }}</td>
                    <td class="p-4 text-gray-600">{{ cca.membership }}</td>
                    <td class="p-4 text-gray-600">{{ cca.teacher }}</td>
                    <td class="p-4 text-gray-600">{{ cca.location }}</td>
                </tr>
            </template>
            </tbody>
        </table>
    </div>
</template>
