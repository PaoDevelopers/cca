<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import type { Course, GradeRequirement, GradeRequirementGroup } from "@/types"

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
		typeof gradeId === "string" &&
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
	const res = await fetch("/student/api/my_selections", {
		credentials: "include",
		redirect: "manual",
	})
	if (
		res.type === "opaqueredirect" ||
		(res.status >= 300 && res.status < 400)
	) {
		if (typeof window !== "undefined") {
			window.location.href = "/"
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
			cca: course?.name ?? "-",
		}
	})
})
</script>

<template>
	<div class="flex-1 p-8 bg-subtle">
		<div class="max-w-6xl mx-auto">
			<h2 class="text-2xl font-light mb-6">Your Selections</h2>
			<div class="grid grid-cols-3 gap-6">
				<div class="col-span-1">
					<div class="bg-surface border border-subtle rounded-lg p-4">
						<h3 class="text-base font-medium text-ink mb-3">
							Requirements Status
						</h3>
						<div class="space-y-2">
							<div
								v-for="(req, i) in requirementCounts"
								:key="i"
								class="flex items-center justify-between text-base py-2 border-b border-subtle last:border-b-0"
							>
								<div class="flex flex-col gap-1">
									<span
										:class="
											req.selected >= req.required
												? 'text-primary font-medium'
												: 'text-ink'
										"
									>
										{{ req.selected }} of {{ req.required }}
										{{ req.categories.join("/") }}
									</span>
									<span
										v-if="req.selected >= req.required"
										class="text-sm text-primary"
									>
										✓ Satisfied
									</span>
									<span v-else class="text-sm text-warning">
										Need
										{{ req.required - req.selected }} more
									</span>
								</div>
							</div>
						</div>
					</div>
				</div>

				<div class="col-span-2">
					<p class="mb-4 text-ink-muted">
						If your chosen CCA appears in the table, you have
						successfully chosen your CCA.
					</p>

					<div
						class="bg-surface border border-subtle rounded-lg overflow-hidden"
					>
						<div
							v-if="isLoading"
							class="flex justify-center items-center p-12 text-sm text-ink-muted"
						>
							<span>Loading...</span>
						</div>
						<table v-else class="w-full border-collapse">
							<thead class="border-b border-subtle bg-subtle">
								<tr>
									<th
										class="text-left p-3 font-medium border-r border-subtle w-1/4"
									>
										Period
									</th>
									<th class="text-left p-3 font-medium w-3/4">
										CCA
									</th>
								</tr>
							</thead>
							<tbody>
								<tr
									v-for="(row, index) in selectionRows"
									:key="index"
									:class="
										index < selectionRows.length - 1
											? 'border-b border-subtle'
											: ''
									"
								>
									<td
										class="p-3 font-medium border-r border-subtle w-1/4"
									>
										{{ row.period }}
									</td>
									<td class="p-3 w-3/4 text-ink-muted">
										{{ row.cca }}
									</td>
								</tr>
							</tbody>
						</table>
					</div>
				</div>
			</div>
		</div>
	</div>
</template>
