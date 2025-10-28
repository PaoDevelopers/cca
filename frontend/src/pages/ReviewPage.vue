<script setup lang="ts">
import {computed, onMounted, ref, watch} from 'vue'
import type {Course} from '@/types'

interface CourseWithSelection extends Course {
    selected: boolean
}

const props = defineProps<{ ccas: CourseWithSelection[], userGrade?: string, grades: any[] }>()

const selectedCourses = computed(() => props.ccas.filter(c => c.selected))
const reqGroups = ref<Array<{ id: number, min_count: number, category_ids: string[] }>>([])

const updateReqGroups = () => {
    if (props.userGrade && props.grades.length) {
        const userGradeData = props.grades.find((g: any) => g.grade === props.userGrade)
        if (userGradeData) reqGroups.value = userGradeData.req_groups
    }
}

onMounted(updateReqGroups)

watch(() => [props.userGrade, props.grades], updateReqGroups)

const requirementCounts = computed(() => {
    if (!reqGroups.value.length) return []
    return reqGroups.value.map((group: { id: number, min_count: number, category_ids: string[] }) => {
        const selected = props.ccas.filter(c => c.selected && group.category_ids.indexOf(c.category_id) !== -1).length
        return {selected, required: group.min_count, categories: group.category_ids}
    })
})

const timetable = computed(() => {
    const table: Record<string, Record<string, CourseWithSelection | null>> = {
        '1': {Monday: null, Tuesday: null, Wednesday: null, Thursday: null},
        '2': {Monday: null, Tuesday: null, Wednesday: null, Thursday: null},
        '3': {Monday: null, Tuesday: null, Wednesday: null, Thursday: null}
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
                    <tr v-for="(slot, index) in ['1', '2', '3']" :key="slot"
                        :class="index < 2 ? 'border-b-2 border-gray-300' : ''">
                        <td class="p-6 font-medium border-r-2 border-gray-300">CCA {{ slot }}</td>
                        <td class="p-6 border-r-2 border-gray-300">{{ timetable[slot].Monday?.name || '-' }}</td>
                        <td class="p-6 border-r-2 border-gray-300">{{ timetable[slot].Tuesday?.name || '-' }}</td>
                        <td class="p-6 border-r-2 border-gray-300">{{ timetable[slot].Wednesday?.name || '-' }}</td>
                        <td class="p-6">{{ timetable[slot].Thursday?.name || '-' }}</td>
                    </tr>
                    </tbody>
                </table>
            </div>

            <div
                class="mt-8 flex gap-3 text-xs font-semibold uppercase tracking-wide border border-gray-200 rounded px-4 py-2 bg-white w-fit">
                <template v-for="(req, i) in requirementCounts" :key="i">
                    <span v-if="i > 0" class="text-gray-300">·</span>
                    <span class="text-gray-900">{{ req.selected }}/{{ req.required }} {{
                            req.categories.join('/')
                        }}</span>
                </template>
            </div>
        </div>
    </div>
</template>
