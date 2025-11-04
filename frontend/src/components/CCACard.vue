<script setup lang="ts">
import { computed } from "vue"
import type { Course } from "@/types"

interface CourseWithSelection extends Course {
	selected: boolean
}

const props = defineProps<{
	cca: CourseWithSelection
	disableClientRestriction: boolean
	updatingCcaId: string | null
	showPeriod: boolean
}>()
const emit = defineEmits<{ toggle: [id: string] }>()

const isOutOfCapacity = computed(
	() =>
		props.cca.current_students >= props.cca.max_students &&
		!props.cca.selected,
)
const isUpdating = computed(() => props.updatingCcaId === props.cca.id)
const isInviteOnly = computed(
	() => props.cca.membership === "invite_only" && !props.cca.selected,
)
const isDisabled = computed(
	() =>
		props.updatingCcaId !== null ||
		(props.disableClientRestriction
			? false
			: isOutOfCapacity.value || isInviteOnly.value),
)
const toggleLabel = computed(
	() => (props.cca.selected ? "Unselect " : "Select ") + props.cca.name,
)
</script>

<template>
	<div
		class="bg-surface border border-subtle rounded-lg p-6 relative flex flex-col"
		:class="
			(isOutOfCapacity || isInviteOnly) && !disableClientRestriction
				? 'opacity-50'
				: 'hover:border-primary'
		"
	>
		<div class="flex justify-between items-start mb-3">
			<div class="pr-12">
				<p
					v-if="showPeriod"
					class="text-xs font-medium uppercase tracking-wide text-primary mb-1"
				>
					{{ cca.period }}
				</p>
				<h3 class="text-lg font-semibold">{{ cca.name }}</h3>
			</div>
			<button
				@click="emit('toggle', cca.id)"
				:disabled="isDisabled"
				type="button"
				class="w-8 h-8 flex items-center justify-center border rounded flex-shrink-0"
				:class="
					cca.selected
						? 'bg-primary border-primary text-white'
						: isDisabled
							? 'border-subtle text-ink-muted cursor-not-allowed'
							: 'border-subtle text-ink-muted hover:border-primary hover:text-primary'
				"
				:aria-label="toggleLabel"
				:aria-pressed="cca.selected ? 'true' : 'false'"
				:aria-busy="isUpdating ? 'true' : 'false'"
				:title="toggleLabel"
			>
				<span
					v-if="isUpdating"
					class="text-sm leading-none text-ink-muted"
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
		</div>

		<p class="text-xs text-ink-muted mb-3">{{ cca.id }}</p>
		<p
			class="text-sm mb-4 leading-relaxed"
			:class="
				(isOutOfCapacity || isInviteOnly) && !disableClientRestriction
					? 'text-ink-muted'
					: 'text-ink'
			"
		>
			{{ cca.description }}
		</p>

		<div class="space-y-1.5 text-sm mt-auto">
			<div class="flex justify-between">
				<span class="text-ink-muted">Teacher</span>
				<span class="font-medium">{{ cca.teacher }}</span>
			</div>
			<div class="flex justify-between">
				<span class="text-ink-muted">Location</span>
				<span class="font-medium">{{ cca.location }}</span>
			</div>
			<div class="flex justify-between">
				<span class="text-ink-muted">Capacity</span>
				<span class="font-medium"
					>{{ cca.current_students }}/{{ cca.max_students }}
					<span
						v-if="cca.current_students >= cca.max_students"
						class="text-danger"
						>(Full!)</span
					></span
				>
			</div>
			<div
				v-if="cca.membership === 'invite_only'"
				class="flex justify-between"
			>
				<span class="text-ink-muted">Membership</span>
				<span class="text-xs font-medium text-warning uppercase"
					>Invite Only</span
				>
			</div>
		</div>
	</div>
</template>
