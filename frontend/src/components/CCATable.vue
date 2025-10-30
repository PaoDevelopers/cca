<script setup lang="ts">
import { computed } from 'vue'
import type { Course } from '@/types'

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
		if (Object.prototype.hasOwnProperty.call(groups, cca.category_id)) {
			groups[cca.category_id].push(cca)
		} else {
			groups[cca.category_id] = [cca]
		}
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
				<template
					v-for="(ccas, category) in groupedCCAs"
					:key="category"
				>
					<tr class="bg-gray-100">
						<td colspan="7" class="p-3 font-medium text-sm">
							{{ category }}
						</td>
					</tr>
					<tr
						v-for="cca in ccas"
						:key="cca.id"
						class="border-b border-gray-200"
						:class="
							(cca.current_students >= cca.max_students ||
								cca.membership === 'invite_only') &&
							!cca.selected &&
							!disableClientRestriction
								? 'opacity-50'
								: 'hover:bg-gray-50'
						"
					>
						<td class="p-4">
							<button
								@click="emit('toggle', cca.id)"
								:disabled="
									updatingCcaId !== null ||
									(disableClientRestriction
										? false
										: (cca.current_students >=
												cca.max_students ||
												cca.membership ===
													'invite_only') &&
											!cca.selected)
								"
								class="w-8 h-8 flex items-center justify-center border rounded"
								:class="
									cca.selected
										? 'bg-[#5bae31] border-[#5bae31] text-white'
										: (disableClientRestriction
													? false
													: cca.current_students >=
															cca.max_students ||
														cca.membership ===
															'invite_only') &&
											  !cca.selected
											? 'border-gray-300 text-gray-400 cursor-not-allowed'
											: 'border-gray-300 text-gray-400 hover:border-[#5bae31] hover:text-[#5bae31]'
								"
							>
								<span
									v-if="updatingCcaId === cca.id"
									class="text-sm leading-none text-gray-500"
									>Loading...</span
								>
								<svg
									v-else-if="cca.selected"
									class="w-4 h-4"
									fill="none"
									stroke="currentColor"
									viewBox="0 0 24 24"
								>
									<path
										stroke-linecap="round"
										stroke-linejoin="round"
										stroke-width="2"
										d="M5 13l4 4L19 7"
									/>
								</svg>
								<span v-else class="text-lg">+</span>
							</button>
						</td>
						<td class="p-4 font-medium">
							<div class="flex flex-col">
								<span>{{ cca.name }}</span>
								<span
									v-if="showPeriod"
									class="text-xs font-medium uppercase tracking-wide text-[#5bae31] mt-1"
									>{{ cca.period }}</span
								>
							</div>
						</td>
						<td class="p-4 text-gray-600">
							{{ cca.current_students }}/{{ cca.max_students }}
							<span
								v-if="cca.current_students >= cca.max_students"
								class="text-red-500"
								>(Full!)</span
							>
						</td>
						<td class="p-4 text-gray-600">{{ cca.id }}</td>
						<td class="p-4 text-gray-600">
							<span
								v-if="cca.membership === 'invite_only'"
								class="text-xs font-medium text-amber-600 uppercase"
								>Invite Only</span
							>
							<span v-else>{{ cca.membership }}</span>
						</td>
						<td class="p-4 text-gray-600">{{ cca.teacher }}</td>
						<td class="p-4 text-gray-600">{{ cca.location }}</td>
					</tr>
				</template>
			</tbody>
		</table>
	</div>
</template>
