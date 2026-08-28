<script lang="ts">
	import type { CourseInput } from "@common/adminApi"
	import { capacityLabel } from "@common/capacity"
	import type { Course } from "@common/types"
	import ConfirmButton from "./ConfirmButton.svelte"
	import CourseEditor from "./CourseEditor.svelte"

	interface Props {
		course: Course
		categories: string[]
		periods: string[]
		gradeIDs: string[]
		busy: boolean
		editing: boolean
		selected: boolean
		onselect: (selected: boolean) => void
		ontoggleedit: () => void
		onsave: (input: CourseInput) => void
		ondelete: () => void
	}

	const {
		course,
		categories,
		periods,
		gradeIDs,
		busy,
		editing,
		selected,
		onselect,
		ontoggleedit,
		onsave,
		ondelete,
	}: Props = $props()
</script>

<tr>
	<td>
		<input
			type="checkbox"
			aria-label="Select {course.id}"
			checked={selected}
			onchange={(event): void => {
				onselect(event.currentTarget.checked)
			}}
		/>
	</td>
	<th scope="row"><code>{course.id}</code></th>
	<td>{course.name}</td>
	<td>{course.category_id}</td>
	<td aria-label={course.period_ids.length === 0 ? "Empty" : undefined}>
		{course.period_ids.join(", ")}
	</td>
	<td>{course.term}</td>
	<td>{course.current_students}/{capacityLabel(course.max_students)}</td>
	<td>{course.invite_only ? "Yes" : "No"}</td>
	<td>
		<button
			aria-label="View enrollments of {course.id}"
			onclick={(): void => {
				window.location.hash = `#/enrollments?q=${encodeURIComponent(
					`course:${course.id}`,
				)}`
			}}
		>
			Enrollments
		</button>
		<button
			disabled={busy}
			aria-label={editing
				? `Close editor for ${course.id}`
				: `Edit ${course.id}`}
			onclick={ontoggleedit}
		>
			{editing ? "Close" : "Edit"}
		</button>
		<ConfirmButton
			action="Delete"
			target={course.id}
			disabled={busy}
			onconfirm={ondelete}
		/>
	</td>
</tr>
{#if editing}
	<tr>
		<td colspan="9">
			<CourseEditor
				initial={course}
				{categories}
				{periods}
				{gradeIDs}
				{busy}
				oncancel={ontoggleedit}
				{onsave}
			/>
		</td>
	</tr>
{/if}
