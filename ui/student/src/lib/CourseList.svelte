<script lang="ts">
	// A list of courses as cards or as a table, shared by the
	// enrolled list and the available catalog.
	import type { Course, Enrollment, Violation } from "@common/types"
	import CourseCard from "./CourseCard.svelte"
	import CourseTable from "./CourseTable.svelte"

	interface Props {
		courses: Course[]
		// null when listing courses the student is not enrolled in.
		enrollment: (course: Course) => Enrollment | null
		// The server's verdicts; empty for a course they may take.
		violations: (course: Course) => Violation[]
		// Whether Swap is offered. Decided by the parent, which holds
		// the student's whole enrollment list; see swappable().
		canSwap?: ((course: Course) => boolean) | undefined
		windowOpen: boolean
		empty: string
		updating: boolean
		// Shared across both lists, so the parent owns it.
		view: "cards" | "table"
		onview: (view: "cards" | "table") => void
		onenroll: (course: Course) => void
		ondrop: (course: Course) => void
		onswap?: ((course: Course) => void) | undefined
	}

	const {
		courses,
		enrollment,
		violations,
		canSwap = (): boolean => false,
		windowOpen,
		empty,
		updating,
		view,
		onview,
		onenroll,
		ondrop,
		onswap,
	}: Props = $props()
</script>

<p class="viewtoggle">
	<button
		aria-pressed={view === "cards"}
		onclick={(): void => {
			onview("cards")
		}}
	>
		Cards
	</button>
	<button
		aria-pressed={view === "table"}
		onclick={(): void => {
			onview("table")
		}}
	>
		Table
	</button>
</p>

{#if view === "cards"}
	<div class="cards">
		{#each courses as course (course.id)}
			<CourseCard
				{course}
				enrollment={enrollment(course)}
				violations={violations(course)}
				canSwap={canSwap(course)}
				{windowOpen}
				{updating}
				{onenroll}
				{ondrop}
				{onswap}
			/>
		{:else}
			<p>{empty}</p>
		{/each}
	</div>
{:else}
	<CourseTable
		rows={courses.map((course) => ({
			course,
			enrollment: enrollment(course),
			violations: violations(course),
			canSwap: canSwap(course),
		}))}
		{windowOpen}
		{empty}
		{updating}
		{onenroll}
		{ondrop}
		{onswap}
	/>
{/if}

<style>
	.viewtoggle button[aria-pressed="true"] {
		font-weight: bold;
	}
</style>
