<script setup lang="ts">
import { computed } from "vue"
import CCACard from "./CCACard.vue"
import type { Course } from "@/types"

interface CourseWithSelection extends Course {
	selected: boolean
}

const props = defineProps<{
	ccas: CourseWithSelection[]
	disableClientRestriction: boolean
	updatingCcaId: string | null
	showPeriod: boolean
}>()
const emit = defineEmits<{ toggle: [id: string] }>()

const groupedCCAs = computed<Record<string, CourseWithSelection[]>>(() => {
	const groups: Record<string, CourseWithSelection[]> = {}
	props.ccas.forEach((cca) => {
		const existing = groups[cca.category_id]
		if (Array.isArray(existing)) {
			existing.push(cca)
			return
		}
		groups[cca.category_id] = [cca]
	})
	return groups
})
</script>

<template>
	<div class="space-y-8">
		<div v-for="(ccas_, category) in groupedCCAs" :key="category">
			<h3 class="text-xl font-medium mb-4">{{ category }}</h3>
			<div
				class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 max-w-7xl"
			>
				<CCACard
					v-for="cca in ccas_"
					:key="cca.id"
					:cca="cca"
					:disable-client-restriction="disableClientRestriction"
					:updating-cca-id="updatingCcaId"
					:show-period="showPeriod"
					@toggle="emit('toggle', $event)"
				/>
			</div>
		</div>
	</div>
</template>
