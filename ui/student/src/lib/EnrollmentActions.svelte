<script lang="ts">
	// What the student may do with one course, and why not when they
	// may not.
	//
	// The reasons come from the server as violations, never recomputed
	// here: the same function that judges the write judges the display,
	// so a course that looks available cannot be refused, and one that
	// looks barred cannot secretly be allowed.
	import type { Course, Enrollment, Violation } from "@common/types"

	interface Props {
		course: Course
		enrollment: Enrollment | null
		violations: Violation[]
		// Whether the student's own enrollment window is open. Closed,
		// they may look but not act.
		windowOpen: boolean
		updating: boolean
		// Whether to offer Swap for this course. Decided by the one
		// definition in enrollment.ts, which needs the student's whole
		// enrollment list — this component only sees its own course,
		// so it is told rather than working it out. There used to be a
		// second, subtly different derivation here.
		canSwap: boolean
		// The card view has no separate count column.
		count?: boolean | undefined
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
		count = false,
		onenroll,
		ondrop,
		onswap,
	}: Props = $props()

	const countSuffix = $derived(
		count ? ` (${course.current_students}/${course.max_students})` : "",
	)

	// The accessible names put the count where the visible text has it,
	// and the course name after.
	//
	// They used to read "Enroll in Chess", which overrides the button's
	// own text — so the seat count, which in card view appears nowhere
	// else on the card, was simply not there for anyone using a screen
	// reader. A course could fill up and the only sign of it was a
	// number they could not reach.
	//
	// Order matters: with the count first, the visible text is a
	// contiguous prefix of the name, which is what WCAG's Label in Name
	// asks for and what lets somebody say "click Enroll nought of two".
	const uid = $props.id()

	// Dropping and swapping ask for an inline confirmation first.
	let confirmingDrop = $state(false)
	let confirmingSwap = $state(false)

	// Arming a confirmation destroys the button that was pressed and
	// mounts two new ones, so the focus falls to <body>: a student
	// hears nothing, and the button they must now press is two tab
	// stops further on with nothing to say it appeared. Moving to the
	// confirming button is also the safer default — it is the one they
	// asked for — and X remains one Shift+Tab away.
	function focusConfirm(node: HTMLButtonElement): void {
		node.focus()
	}

	const selected = $derived(enrollment !== null)

	// An enrollment the student may not drop: an administrator placed
	// them and did not leave the door open.
	const fixed = $derived(enrollment !== null && !enrollment.student_droppable)

	// Invite-only is a gate rather than a negotiable rule, so it does
	// not appear among the violations and is read from the course.
	const barred = $derived(
		course.invite_only || (violations.length > 0 && !canSwap),
	)

	const reasons = $derived.by((): string[] => {
		const out = violations.map((v): string => v.detail)
		if (course.invite_only) {
			out.push("invitation required")
		}
		return out
	})
</script>

<span class="actions">
	<span>
		{#if selected}
			{#if fixed}
				Placed by an administrator
			{:else if enrollment?.counts_toward_budget === false}
				Invited
			{:else}
				Selected
			{/if}
		{:else if reasons.length > 0}
			<span id="{uid}-reasons">{reasons.join("; ")}</span>
		{/if}
	</span>

	{#if !windowOpen}
		<!--
			The window is the gate; the server refuses regardless, so
			this only spares the student a rejection they can do
			nothing about.
		-->
		<span class="closed">Enrollment closed</span>
	{:else if selected && fixed}
		<span class="closed">Cannot be dropped</span>
	{:else if selected && confirmingDrop}
		<button
			aria-label="Cancel dropping {course.name}"
			onclick={(): void => {
				confirmingDrop = false
			}}
		>
			X
		</button>
		<button
			disabled={updating}
			aria-label="Confirm dropping {course.name}"
			data-course-action={course.id}
			use:focusConfirm
			onclick={(): void => {
				confirmingDrop = false
				ondrop(course)
			}}
		>
			Confirm drop
		</button>
	{:else if selected}
		<button
			disabled={updating}
			aria-label="Drop{countSuffix} {course.name}"
			data-course-action={course.id}
			onclick={(): void => {
				confirmingDrop = true
			}}
		>
			Drop{countSuffix}
		</button>
	{:else if canSwap && onswap !== undefined}
		<!--
			Swapping is one operation, not a drop followed by an
			enroll: the new course is judged with the old ones
			disregarded, which is the only way to move between two
			courses that clash.
		-->
		{#if confirmingSwap}
			<button
				aria-label="Cancel swapping into {course.name}"
				data-course-action={course.id}
				onclick={(): void => {
					confirmingSwap = false
				}}
			>
				X
			</button>
			<button
				disabled={updating}
				aria-label="Confirm swapping into {course.name}"
				data-course-action={course.id}
				use:focusConfirm
				onclick={(): void => {
					confirmingSwap = false
					onswap(course)
				}}
			>
				Confirm swap
			</button>
		{:else}
			<button
				disabled={updating}
				aria-label="Swap in{countSuffix}, replacing the courses {course.name} clashes with"
				data-course-action={course.id}
				onclick={(): void => {
					confirmingSwap = true
				}}
			>
				Swap in{countSuffix}
			</button>
		{/if}
	{:else}
		<button
			disabled={updating || barred}
			aria-label="Enroll{countSuffix} in {course.name}"
			aria-describedby={barred && reasons.length > 0
				? `${uid}-reasons`
				: undefined}
			data-course-action={course.id}
			onclick={(): void => {
				onenroll(course)
			}}
		>
			Enroll{countSuffix}
		</button>
	{/if}
</span>

<style>
	span.actions {
		display: flex;
		align-items: flex-end;
		gap: 0.5rem;

		/* Status text on the left, action buttons pushed right. */
		> span:first-child {
			flex: 1;
		}
	}

	.closed {
		color: color-mix(in oklab, var(--fg) 60%, var(--bg));
		white-space: nowrap;
	}
</style>
