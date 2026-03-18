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
</script>

<template>
	<div
		class="bg-white border border-gray-200 rounded-lg p-6 relative flex flex-col"
		:class="
			(isOutOfCapacity || isInviteOnly) && !disableClientRestriction
				? 'opacity-50'
				: 'hover:border-[#5bae31]'
		"
	>
		<div class="flex justify-between items-start mb-3">
			<div class="pr-12">
				<p
					v-if="showPeriod"
					class="text-xs font-medium uppercase tracking-wide text-[#5bae31] mb-1"
				>
					{{ cca.period }}
				</p>
				<h3 class="text-lg font-semibold">{{ cca.name }}</h3>
			</div>
			<button
				@click="emit('toggle', cca.id)"
				:disabled="isDisabled"
				class="w-8 h-8 flex items-center justify-center border rounded flex-shrink-0"
				:class="
					cca.selected
						? 'bg-[#5bae31] border-[#5bae31] text-white'
						: isDisabled
							? 'border-gray-300 text-gray-400 cursor-not-allowed'
							: 'border-gray-400 text-gray-600 hover:border-[#5bae31] hover:text-[#5bae31]'
				"
			>
				<span
					v-if="isUpdating"
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
		</div>

		<p class="text-xs text-gray-500 mb-3">{{ cca.id }}</p>
		<p
			class="text-sm mb-4 leading-relaxed"
			:class="
				(isOutOfCapacity || isInviteOnly) && !disableClientRestriction
					? 'text-gray-500'
					: 'text-gray-700'
			"
		>
			{{ cca.description }}
		</p>

		<div class="space-y-1.5 text-sm mt-auto">
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
				<span class="font-medium"
					>{{ cca.current_students }}/{{ cca.max_students }}
					<span
						v-if="cca.current_students >= cca.max_students"
						class="text-red-500"
						>(Full!)</span
					></span
				>
			</div>
			<div
				v-if="cca.membership === 'invite_only'"
				class="flex justify-between"
			>
				<span class="text-gray-500">Membership</span>
				<span class="text-xs font-medium text-amber-600 uppercase"
					>Invite Only</span
				>
			</div>
		</div>
	</div>
</template>
