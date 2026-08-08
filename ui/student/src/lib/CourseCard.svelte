<script lang="ts">
	import type { Course, Enrollment, Violation } from "@common/types"
	import EnrollmentActions from "./EnrollmentActions.svelte"

	interface Props {
		course: Course
		enrollment: Enrollment | null
		violations: Violation[]
		windowOpen: boolean
		updating: boolean
		canSwap: boolean
		onenroll: (course: Course) => void
		ondrop: (course: Course) => void
		onswap?: ((course: Course) => void) | undefined
	}

	const {
		course,
		enrollment,
		violations,
		windowOpen,
		updating,
		canSwap,
		onenroll,
		ondrop,
		onswap,
	}: Props = $props()

	// The card is labelled by its course name, which is not the
	// article's first child.
	const uid = $props.id()
</script>

<article class="card course" aria-labelledby="{uid}-title">
	<div>
		<h3 id="{uid}-title">{course.name}</h3>
		<code>{course.id}</code>
	</div>
	<div>
		<span>
			{#if course.teacher_email !== ""}
				<a href="mailto:{course.teacher_email}">{course.teacher}</a>
			{:else}
				{course.teacher}
			{/if}
		</span>
		<span>{course.category_id}</span>
	</div>
	<div>
		<span>{course.location}</span>
		<span>{course.period_ids.join(", ")}</span>
	</div>
	<div>
		<span>{course.term}</span>
		{#if course.cost !== ""}
			<span>{course.cost}</span>
		{/if}
	</div>
	{#if course.description !== ""}
		<p>{course.description}</p>
	{/if}
	<div class="bottom">
		<EnrollmentActions
			{course}
			{enrollment}
			{violations}
			{windowOpen}
			{updating}
			{canSwap}
			count
			{onenroll}
			{ondrop}
			{onswap}
		/>
	</div>
</article>

<style>
	article.course {
		> div {
			display: flex;
			justify-content: space-between;
			align-items: baseline;
			gap: 0.5rem;
		}

		/* The status/action line sits flush with the card bottom, and
		   the actions component spans the full row. */
		> div.bottom {
			margin-top: auto;

			> :global(.actions) {
				flex: 1;
			}
		}

		h3 {
			font-size: 1em;
			margin: 0;
		}

		p {
			margin: 0;
		}
	}
</style>
