<script lang="ts">
	// Read-only, or the inline editor in its place.
	import type { AdminStudent } from "@common/adminApi"
	import type { Course, Enrollment, LegalSex } from "@common/types"
	import AdminSchedule from "./AdminSchedule.svelte"
	import ConfirmButton from "./ConfirmButton.svelte"
	import StudentEditor from "./StudentEditor.svelte"

	interface Props {
		student: AdminStudent
		gradeIDs: string[]
		sexes: LegalSex[]
		budget: string
		requirements: string
		busy: boolean
		// Whether this row is the one being edited. The draft itself
		// belongs to the editor, which is mounted only while true.
		editing: boolean
		scheduleOpen: boolean
		periods: string[]
		courses: Course[]
		enrollments: Enrollment[]
		onedit: () => void
		oncancel: () => void
		onsave: (draft: AdminStudent) => void
		ondelete: () => void
		ontoggleschedule: () => void
	}

	const {
		student,
		gradeIDs,
		sexes,
		budget,
		requirements,
		busy,
		editing,
		scheduleOpen,
		periods,
		courses,
		enrollments,
		onedit,
		oncancel,
		onsave,
		ondelete,
		ontoggleschedule,
	}: Props = $props()
</script>

{#if editing}
	<StudentEditor
		{student}
		{gradeIDs}
		{sexes}
		{budget}
		{requirements}
		{busy}
		{onsave}
		{oncancel}
	/>
{:else}
	<tr>
		<th scope="row">{student.id}</th>
		<td>{student.name}</td>
		<td>{student.grade_id}</td>
		<td>{student.legal_sex}</td>
		<td>{budget}</td>
		<td>{requirements}</td>
		<td>
			<button
				aria-label={scheduleOpen
					? `Close the schedule of ${student.id}`
					: `View the schedule of ${student.id}`}
				onclick={ontoggleschedule}
			>
				{scheduleOpen ? "Close" : "Schedule"}
			</button>
			<button
				aria-label="View enrollments of {student.id}"
				onclick={(): void => {
					window.location.hash = `#/enrollments?q=${encodeURIComponent(
						`student:${student.id}`,
					)}`
				}}
			>
				Enrollments
			</button>
			<button
				disabled={busy}
				aria-label="Edit {student.id}"
				onclick={onedit}
			>
				Edit
			</button>
			<ConfirmButton
				action="Delete"
				target={student.id}
				disabled={busy}
				onconfirm={ondelete}
			/>
		</td>
	</tr>
	{#if scheduleOpen}
		<tr>
			<td colspan="7">
				<AdminSchedule
					studentID={student.id}
					{periods}
					{courses}
					{enrollments}
				/>
			</td>
		</tr>
	{/if}
{/if}
