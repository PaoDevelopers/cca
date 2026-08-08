<script lang="ts">
	import type { Course, Enrollment, Violation } from "@common/types"
	import EnrollmentActions from "./EnrollmentActions.svelte"

	interface Row {
		course: Course
		enrollment: Enrollment | null
		violations: Violation[]
		canSwap: boolean
	}

	interface Props {
		rows: Row[]
		windowOpen: boolean
		empty: string
		updating: boolean
		onenroll: (course: Course) => void
		ondrop: (course: Course) => void
		onswap?: ((course: Course) => void) | undefined
	}

	const {
		rows,
		windowOpen,
		empty,
		updating,
		onenroll,
		ondrop,
		onswap,
	}: Props = $props()
</script>

<div class="scrolls">
	<table>
		<thead>
			<tr>
				<th scope="col">ID</th>
				<th scope="col">Title</th>
				<th scope="col">Teacher</th>
				<th scope="col">Location</th>
				<th scope="col">Category</th>
				<th scope="col">Periods</th>
				<th scope="col">Term</th>
				<th scope="col">Cost</th>
				<th scope="col">Students</th>
				<th scope="col">Status</th>
			</tr>
		</thead>
		<tbody>
			{#each rows as row (row.course.id)}
				<tr>
					<th scope="row"><code>{row.course.id}</code></th>
					<td>{row.course.name}</td>
					<td>
						{#if row.course.teacher_email !== ""}
							<a href="mailto:{row.course.teacher_email}">
								{row.course.teacher}
							</a>
						{:else}
							{row.course.teacher}
						{/if}
					</td>
					<td>{row.course.location}</td>
					<td>{row.course.category_id}</td>
					<td
						aria-label={row.course.period_ids.length === 0
							? "Empty"
							: undefined}
					>
						{row.course.period_ids.join(", ")}
					</td>
					<td>{row.course.term}</td>
					<td
						aria-label={row.course.cost === ""
							? "Empty"
							: undefined}
					>
						{row.course.cost}
					</td>
					<td>
						{row.course.current_students}/{row.course.max_students}
					</td>
					<td>
						<EnrollmentActions
							course={row.course}
							enrollment={row.enrollment}
							violations={row.violations}
							canSwap={row.canSwap}
							{windowOpen}
							{updating}
							{onenroll}
							{ondrop}
							{onswap}
						/>
					</td>
				</tr>
			{:else}
				<tr>
					<td colspan="10">{empty}</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>
