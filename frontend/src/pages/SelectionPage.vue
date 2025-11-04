<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import CCAGrid from "../components/CCAGrid.vue"
import CCATable from "../components/CCATable.vue"
import type { Course, GradeRequirement, GradeRequirementGroup } from "@/types"

interface CourseWithSelection extends Course {
	selected: boolean
}

const ALL_PERIODS = "__ALL_PERIODS__"

const props = defineProps<{
	ccas: CourseWithSelection[]
	searchActive: boolean
	userGrade?: string
	grades: GradeRequirement[]
	periods: string[]
	initialPeriod?: string
	initialViewMode?: "grid" | "table"
	disableClientRestriction: boolean
	updatingCcaId: string | null
}>()

const isLoading = computed<boolean>(() => props.periods.length === 0)
const emit = defineEmits<{
	toggle: [id: string]
	periodChange: [period: string]
	viewModeChange: [mode: "grid" | "table"]
}>()

const initialSelectedPeriod =
	typeof props.initialPeriod === "string" && props.initialPeriod.length > 0
		? props.initialPeriod
		: ALL_PERIODS
const selectedPeriod = ref<string>(initialSelectedPeriod)
const hasNoResults = computed<boolean>(
	() => !isLoading.value && filteredCCAs.value.length === 0,
)
const viewMode = ref<"grid" | "table">(props.initialViewMode ?? "grid")
const reqGroups = ref<GradeRequirementGroup[]>([])
const isAllPeriods = computed<boolean>(
	() => selectedPeriod.value === ALL_PERIODS,
)

watch(
	() => viewMode.value,
	(newMode): void => {
		emit("viewModeChange", newMode)
	},
)

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

const initPeriod = (): void => {
	const initialPeriod =
		typeof props.initialPeriod === "string" &&
		props.initialPeriod.length > 0
			? props.initialPeriod
			: null
	if (initialPeriod !== null) {
		selectedPeriod.value = initialPeriod
		emit("periodChange", initialPeriod)
		return
	}
	if (props.periods.length === 0) {
		return
	}
	if (selectedPeriod.value === ALL_PERIODS) {
		emit("periodChange", "")
		return
	}
	if (!props.periods.includes(selectedPeriod.value)) {
		selectedPeriod.value = ALL_PERIODS
		emit("periodChange", "")
	}
}

onMounted((): void => {
	updateReqGroups()
	initPeriod()
})

watch(() => [props.userGrade, props.grades], updateReqGroups)
watch(() => props.periods, initPeriod)
watch(
	() => props.initialPeriod,
	(newInitial): void => {
		if (typeof newInitial === "string" && newInitial.length > 0) {
			selectedPeriod.value = newInitial
		} else if (selectedPeriod.value !== ALL_PERIODS) {
			selectedPeriod.value = ALL_PERIODS
		}
	},
)

const selectPeriod = (period: string): void => {
	if (period === ALL_PERIODS) {
		selectedPeriod.value = ALL_PERIODS
		emit("periodChange", "")
	} else {
		selectedPeriod.value = period
		emit("periodChange", period)
	}
}

const filteredCCAs = computed<CourseWithSelection[]>(() => {
	if (selectedPeriod.value === ALL_PERIODS) {
		return props.ccas
	}
	return props.ccas.filter((c) => c.period === selectedPeriod.value)
})

const ccasByPeriod = computed<Record<string, CourseWithSelection[]>>(() => {
	const grouped: Record<string, CourseWithSelection[]> = {}
	props.ccas.forEach((c) => {
		const list = grouped[c.period]
		if (Array.isArray(list)) {
			list.push(c)
		} else {
			grouped[c.period] = [c]
		}
	})
	if (
		props.searchActive &&
		Object.keys(grouped).length > 0 &&
		selectedPeriod.value !== ALL_PERIODS
	) {
		const firstPeriodWithResults = props.periods.find((p) => {
			const periodGroup = grouped[p]
			return Array.isArray(periodGroup) && periodGroup.length > 0
		})
		if (
			firstPeriodWithResults !== undefined &&
			selectedPeriod.value !== firstPeriodWithResults
		) {
			const selectedGroup = grouped[selectedPeriod.value]
			if (!Array.isArray(selectedGroup) || selectedGroup.length === 0) {
				// eslint-disable-next-line vue/no-side-effects-in-computed-properties
				selectedPeriod.value = firstPeriodWithResults
				emit("periodChange", firstPeriodWithResults)
			}
		}
	}
	return grouped
})

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
</script>

<template>
	<div class="flex flex-1">
		<aside
			class="w-56 border-r border-subtle bg-surface p-8 sticky self-start overflow-y-auto"
			style="
				top: var(--cca-header-offset);
				max-height: calc(100vh - var(--cca-header-offset));
			"
		>
			<ul v-if="!isLoading" class="space-y-2 text-sm text-ink-muted">
				<li>
					<button
						type="button"
						class="w-full text-left px-2 py-1.5 rounded-md"
						@click="selectPeriod(ALL_PERIODS)"
						:class="[
							isAllPeriods
								? 'text-primary font-medium bg-primary-soft'
								: 'hover:text-ink',
						]"
						:aria-pressed="isAllPeriods ? 'true' : 'false'"
						:aria-current="isAllPeriods ? 'true' : undefined"
					>
						All periods
					</button>
				</li>
				<li v-for="period in props.periods" :key="period">
					<button
						type="button"
						class="w-full text-left px-2 py-1.5 rounded-md"
						@click="selectPeriod(period)"
						:class="[
							selectedPeriod === period
								? 'text-primary font-medium bg-primary-soft'
								: '',
							searchActive && !ccasByPeriod[period]?.length
								? 'text-ink-muted opacity-60'
								: 'hover:text-ink',
						]"
						:aria-pressed="
							selectedPeriod === period ? 'true' : 'false'
						"
						:aria-current="
							selectedPeriod === period ? 'true' : undefined
						"
					>
						{{ period }}
					</button>
				</li>
			</ul>
			<div v-else class="space-y-2">
				<div class="skeleton h-6 w-full"></div>
				<div class="skeleton h-6 w-full"></div>
				<div class="skeleton h-6 w-full"></div>
				<div class="skeleton h-6 w-full"></div>
			</div>
		</aside>

		<main class="flex-1 flex flex-col bg-subtle">
			<div
				class="sticky z-20 border-b border-subtle px-8 py-4 flex justify-between items-center bg-surface"
				style="top: var(--cca-header-offset); min-height: 4.5rem"
			>
				<div v-if="!isLoading" class="flex gap-3 text-base">
					<template v-for="(req, i) in requirementCounts" :key="i">
						<span v-if="i > 0" class="text-ink-muted opacity-60"
							>·</span
						>
						<span
							class="font-medium"
							:style="{
								color:
									req.selected >= req.required
										? 'rgb(var(--cca-primary))'
										: 'rgb(var(--cca-danger))',
							}"
							>{{ req.selected }} of {{ req.required }}
							{{ req.categories.join("/") }}
							({{
								req.selected >= req.required
									? "Satisfied"
									: "Unsatisfied"
							}})</span
						>
					</template>
				</div>
				<div v-else class="skeleton h-10 w-64"></div>
				<div class="flex gap-2">
					<button
						type="button"
						@click="viewMode = 'grid'"
						class="p-2 border rounded"
						:class="
							viewMode === 'grid'
								? 'bg-primary text-white border-primary'
								: 'border-gray-300 text-ink-muted'
						"
						:aria-pressed="viewMode === 'grid' ? 'true' : 'false'"
						aria-label="Show grid view"
					>
						<svg
							class="w-5 h-5"
							fill="none"
							stroke="currentColor"
							viewBox="0 0 24 24"
						>
							<path
								stroke-linecap="round"
								stroke-linejoin="round"
								stroke-width="2"
								d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z"
							/>
						</svg>
					</button>
					<button
						type="button"
						@click="viewMode = 'table'"
						class="p-2 border rounded"
						:class="
							viewMode === 'table'
								? 'bg-primary text-white border-primary'
								: 'border-gray-300 text-ink-muted'
						"
						:aria-pressed="viewMode === 'table' ? 'true' : 'false'"
						aria-label="Show table view"
					>
						<svg
							class="w-5 h-5"
							fill="none"
							stroke="currentColor"
							viewBox="0 0 24 24"
						>
							<path
								stroke-linecap="round"
								stroke-linejoin="round"
								stroke-width="2"
								d="M3 10h18M3 14h18m-9-4v8m-7 0h14a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"
							/>
						</svg>
					</button>
				</div>
			</div>

			<div class="flex-1 p-8">
				<div
					v-if="isLoading"
					class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6"
				>
					<div class="skeleton h-64 w-full"></div>
					<div class="skeleton h-64 w-full"></div>
					<div class="skeleton h-64 w-full"></div>
				</div>
				<div
					v-else-if="hasNoResults"
					class="flex items-center justify-center h-64 text-ink-muted"
				>
					No result
				</div>
				<CCAGrid
					v-else-if="viewMode === 'grid'"
					:ccas="filteredCCAs"
					:disable-client-restriction="disableClientRestriction"
					:updating-cca-id="updatingCcaId"
					:show-period="isAllPeriods"
					@toggle="emit('toggle', $event)"
				/>
				<CCATable
					v-else
					:ccas="filteredCCAs"
					:disable-client-restriction="disableClientRestriction"
					:updating-cca-id="updatingCcaId"
					:show-period="isAllPeriods"
					@toggle="emit('toggle', $event)"
				/>
			</div>
		</main>
	</div>
</template>
