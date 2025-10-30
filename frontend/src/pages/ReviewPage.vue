<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import type { Course, GradeRequirement, GradeRequirementGroup } from '@/types'

interface CourseWithSelection extends Course {
	selected: boolean
}

interface Selection {
	course_id: string
	period: string
	selection_type: string
}

const props = defineProps<{
	ccas: CourseWithSelection[]
	userGrade?: string
	grades: GradeRequirement[]
	periods: string[]
}>()

const selections = ref<Selection[]>([])
const reqGroups = ref<GradeRequirementGroup[]>([])
const isLoading = ref(true)

const updateReqGroups = (): void => {
	const gradeId = props.userGrade
	if (
		typeof gradeId === 'string' &&
		gradeId.length > 0 &&
		props.grades.length > 0
	) {
		const userGradeData = props.grades.find((g) => g.grade === gradeId)
		if (userGradeData !== undefined) {
			reqGroups.value = userGradeData.req_groups
			return
		}
	}
	reqGroups.value = []
}

const loadSelections = async (): Promise<void> => {
	isLoading.value = true
	const res = await fetch('/student/api/my_selections', {
		credentials: 'include',
		redirect: 'manual',
	})
	if (
		res.type === 'opaqueredirect' ||
		(res.status >= 300 && res.status < 400)
	) {
		if (typeof window !== 'undefined') {
			window.location.href = '/'
		}
		return
	}
	const data = (await res.json()) as Selection[] | null
	selections.value = Array.isArray(data) ? data : []
	isLoading.value = false
}

onMounted(async (): Promise<void> => {
	updateReqGroups()
	await loadSelections()
})

watch(() => [props.userGrade, props.grades], updateReqGroups)

const requirementCounts = computed<
	Array<{
		selected: number
		required: number
		categories: string[]
	}>
>(() => {
	if (reqGroups.value.length === 0) return []
	return reqGroups.value.map((group) => {
		const selected = props.ccas.filter(
			(c) =>
				c.selected && group.category_ids.indexOf(c.category_id) !== -1,
		).length
		return {
			selected,
			required: group.min_count,
			categories: group.category_ids,
		}
	})
})

const selectionRows = computed<Array<{ period: string; cca: string }>>(() => {
	return props.periods.map((period) => {
		const sel = selections.value.find((s) => s.period === period)
		const course =
			sel !== undefined
				? props.ccas.find((c) => c.id === sel.course_id)
				: undefined
		return {
			period,
			cca: course?.name ?? '-',
		}
	})
})
</script>

<template>
	<div class="flex-1 p-8 bg-gray-50/30">
		<div class="max-w-4xl mx-auto">
			<div class="flex items-center justify-between mb-6">
				<h2 class="text-2xl font-light">Your Selections</h2>
				<div
					class="flex gap-3 text-sm border border-gray-200 rounded px-4 py-2 bg-white"
				>
					<span class="text-gray-600">Requirements:</span>
					<template v-for="(req, i) in requirementCounts" :key="i">
						<span v-if="i > 0" class="text-gray-300">·</span>
						<span
							:class="
								req.selected >= req.required
									? 'text-green-600'
									: 'text-gray-900'
							"
							>{{ req.selected }}/{{ req.required }}
							{{ req.categories.join('/') }}</span
						>
					</template>
				</div>
			</div>

			<div
				role="alert"
				class="flex items-center gap-3 bg-[#5bae31]/10 border border-[#5bae31]/30 rounded-lg px-4 py-3 mb-4"
			>
				<svg
					xmlns="http://www.w3.org/2000/svg"
					fill="none"
					viewBox="0 0 24 24"
					class="h-5 w-5 shrink-0 stroke-[#5bae31]"
				>
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
					></path>
				</svg>
				<span class="text-[#5bae31]"
					>If your chosen CCA is in the table, you have successfully
					chosen your CCA.</span
				>
			</div>

			<div
				class="bg-white border-1 border-gray-300 rounded-lg overflow-hidden"
			>
				<div
					v-if="isLoading"
					class="flex justify-center items-center p-12 text-sm text-gray-600"
				>
					<span>Loading...</span>
				</div>
				<table v-else class="w-full border-collapse">
					<thead class="border-b-1 border-gray-300 bg-gray-50">
						<tr>
							<th
								class="text-left p-3 font-medium border-r-1 border-gray-300 w-1/4"
							>
								Period
							</th>
							<th class="text-left p-3 font-medium w-3/4">CCA</th>
						</tr>
					</thead>
					<tbody>
						<tr
							v-for="(row, index) in selectionRows"
							:key="index"
							:class="
								index < selectionRows.length - 1
									? 'border-b-1 border-gray-300'
									: ''
							"
						>
							<td
								class="p-3 font-medium border-r-1 border-gray-300 w-1/4"
							>
								{{ row.period }}
							</td>
							<td class="p-3 w-3/4">{{ row.cca }}</td>
						</tr>
					</tbody>
				</table>
			</div>
		</div>
	</div>
</template>
