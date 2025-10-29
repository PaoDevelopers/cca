<script setup lang="ts">
import {computed} from 'vue'
import CCACard from './CCACard.vue'
import type {Course} from '@/types'

interface CourseWithSelection extends Course {
    selected: boolean
}

const props = defineProps<{ ccas: CourseWithSelection[], disableClientRestriction: boolean }>()
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
    <div class="space-y-8">
        <div v-for="(ccas, category) in groupedCCAs" :key="category">
            <h3 class="text-xl font-medium mb-4">{{ category }}</h3>
            <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 max-w-7xl">
                <CCACard v-for="cca in ccas" :key="cca.id" :cca="cca" :disable-client-restriction="disableClientRestriction" @toggle="emit('toggle', $event)"/>
            </div>
        </div>
    </div>
</template>
