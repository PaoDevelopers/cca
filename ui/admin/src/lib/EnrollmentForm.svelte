<script lang="ts">
	// Placing students into one course. One course, because that is
	// the unit the database locks and judges as a whole — and because
	// its batch order is significant: students compete for the same
	// seats, and the ones named first win when the course fills.
	import { placeEnrollments } from "@common/adminApi"
	import type { Course } from "@common/types"
	import { courseLabel } from "./enrollments"

	interface Props {
		courses: Course[]
		courseByID: Map<string, Course>
		busy: boolean
		// Placement can break every negotiable rule, so it goes
		// through the accept protocol rather than an override switch.
		runAccepting: (
			action: (accept: string[]) => Promise<void>,
		) => Promise<boolean>
	}

	const { courses, courseByID, busy, runAccepting }: Props = $props()

	const uid = $props.id()

	let newStudents = $state("")
	let newCourse = $state("")

	// The two policy bits, chosen independently. The old three-valued
	// type could not say "the student's own committed pick"; these
	// can.
	let droppable = $state(true)
	let budgeted = $state(false)
</script>

<form
	onsubmit={(event): void => {
		event.preventDefault()
		// Not sorted and not deduplicated: the order the
		// administrator typed is the order seats are handed out in.
		const ids = newStudents
			.split(/[\s,]+/)
			.filter((part): boolean => part !== "")
		if (ids.length === 0 || newCourse.trim() === "") {
			return
		}
		void runAccepting(async (accept): Promise<void> => {
			await placeEnrollments(
				newCourse.trim(),
				ids,
				droppable,
				budgeted,
				accept,
			)
			newStudents = ""
			newCourse = ""
		})
	}}
>
	<label>
		Student IDs (comma or space separated)
		<input
			type="text"
			bind:value={newStudents}
			placeholder="s22537 s23321"
			required
		/>
	</label>
	<label>
		Course
		<input
			type="text"
			list="{uid}-courses"
			bind:value={newCourse}
			required
		/>
	</label>
	<datalist id="{uid}-courses">
		{#each courses as course (course.id)}
			<option value={course.id}>
				{courseLabel(courseByID, course.id, course.name)}
			</option>
		{/each}
	</datalist>
	<label>
		<input type="checkbox" bind:checked={droppable} />
		Student may drop
	</label>
	<label>
		<input type="checkbox" bind:checked={budgeted} />
		Counts toward their budget
	</label>
	<button disabled={busy}>Place</button>
</form>

<style>
	form {
		margin-top: 1rem;
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem 1rem;
		align-items: baseline;
	}

	form label {
		display: inline-flex;
		align-items: baseline;
		gap: 0.25rem;
	}
</style>
