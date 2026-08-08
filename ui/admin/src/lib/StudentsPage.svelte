<script lang="ts">
	import {
		deleteStudent,
		upsertStudents,
		type AdminStudent,
	} from "@common/adminApi"
	import FilterBar from "./FilterBar.svelte"
	import DataTools from "./DataTools.svelte"
	import { matchesFilter } from "@common/filter"
	import { filterByCel, type FilterMode } from "@common/cel"
	import type { LegalSex } from "@common/types"
	import { getAdminData } from "./data.svelte"
	import StudentRow from "./StudentRow.svelte"
	import {
		coursesByStudent,
		statusByStudent,
		studentCelContext,
	} from "./students"

	interface Props {
		// The route's ?q=, bound so the box and the URL stay one value.
		filter: string
	}

	let { filter = $bindable("") }: Props = $props()

	const data = getAdminData()
	data.want("students", "grades", "courses", "periods", "enrollments")

	const sexes: LegalSex[] = ["F", "M", "X"]

	const gradeIDs = $derived(data.grades.map((g): string => g.id))

	// The student whose weekly schedule is expanded under their row.
	let scheduleFor = $state<string | null>(null)
	let mode = $state<FilterMode>("simple")
	let creating = $state(false)

	// The row being edited; null when not editing. The draft itself
	// lives in the editor that row mounts.
	let editing = $state<string | null>(null)

	let newID = $state("")
	let newName = $state("")
	let newGrade = $state("")
	let newSex = $state<LegalSex>("F")

	function run(action: () => Promise<void>): Promise<boolean> {
		return data.run(action, "students")
	}

	// Changing a student's grade or legal sex can invalidate the
	// placements they already hold, so an edit goes through the accept
	// protocol. Changing only their name cannot, and the server
	// re-judges nothing in that case — which is why re-importing an
	// unchanged roster never resurfaces an accepted placement.
	function runAccepting(
		action: (accept: string[]) => Promise<void>,
	): Promise<boolean> {
		return data.runAccepting(action, "students", "enrollments")
	}

	const coursesOf = $derived(coursesByStudent(data.courses, data.enrollments))
	const statusOf = $derived(statusByStudent(data.studentStatus))

	function requirementsText(id: string): string {
		return (statusOf.get(id)?.requirements_met ?? true)
			? "OK"
			: "Unsatisfied"
	}

	// "2/4 periods", or just the count when the grade has no cap.
	function budgetText(id: string): string {
		const status = statusOf.get(id)
		if (status === undefined) {
			return ""
		}
		return status.max_budgeted_periods === null
			? String(status.budgeted_periods_used)
			: `${String(status.budgeted_periods_used)}/${String(status.max_budgeted_periods)}`
	}

	function celContext(s: AdminStudent): Record<string, unknown> {
		return studentCelContext(
			s,
			coursesOf.get(s.id) ?? [],
			statusOf.get(s.id),
		)
	}

	const celResult = $derived(
		mode === "cel"
			? filterByCel(filter, data.students, celContext)
			: { rows: data.students, error: null, unfiltered: false },
	)

	const filtered = $derived(
		mode === "cel"
			? celResult.rows
			: data.students.filter((s): boolean =>
					matchesFilter(filter, {
						id: { exact: s.id },
						name: s.name,
						grade: { exact: s.grade_id },
						sex: s.legal_sex,
						requirements: { exact: requirementsText(s.id) },
					}),
				),
	)
</script>

<section aria-labelledby="students-heading">
	<!--
		The page had no heading below the h1 at all, so heading
		navigation went straight from "CCA administration" to nothing and
		the section was an unlabelled region. tabindex because it is also
		where the data layer puts the focus when the control that started
		a write no longer exists.
	-->
	<h2 id="students-heading" tabindex="-1">Students</h2>

	{#if !data.ready("students", "grades", "courses", "periods", "enrollments")}
		<p>Loading&hellip;</p>
	{:else}
		<div class="admin-actions">
			<details class="create-panel" bind:open={creating}>
				<summary>Add student</summary>
				<form
					onsubmit={(event): void => {
						event.preventDefault()
						const id = newID.trim()
						if (id === "" || newGrade === "") {
							return
						}
						// The same upsert the import uses: adding a student and
						// correcting one are the same intention.
						void runAccepting(async (accept): Promise<void> => {
							await upsertStudents(
								[
									{
										id,
										name: newName.trim(),
										grade_id: newGrade,
										legal_sex: newSex,
									},
								],
								accept,
							)
							newID = ""
							newName = ""
							creating = false
						})
					}}
				>
					<label>
						Student ID
						<input
							type="text"
							bind:value={newID}
							placeholder="s22537"
							required
						/>
					</label>
					<label>
						Name
						<input type="text" bind:value={newName} required />
					</label>
					<label>
						Grade
						<select bind:value={newGrade} required>
							<option value="" disabled>Choose</option>
							{#each gradeIDs as gradeID (gradeID)}
								<option value={gradeID}>{gradeID}</option>
							{/each}
						</select>
					</label>
					<label>
						Legal sex
						<select bind:value={newSex}>
							{#each sexes as sex (sex)}
								<option value={sex}>{sex}</option>
							{/each}
						</select>
					</label>
					<button disabled={data.busy}>Add</button>
				</form>
			</details>

			<DataTools
				section="students"
				exportHref="/admin/api/students/export"
				importAction="/admin/api/students/import"
				busy={data.busy}
				{run}
			/>
		</div>

		<FilterBar
			bind:query={filter}
			bind:mode
			error={mode === "cel" ? celResult.error : null}
			onclear={(): void => {
				const cut = window.location.hash.indexOf("?")
				if (cut !== -1) {
					window.location.hash = window.location.hash.slice(0, cut)
				}
			}}
			fields={{
				id: "student ID (exact)",
				name: "student name",
				grade: "year group (exact)",
				sex: "legal sex (F, M, X)",
				requirements: "requirements (OK, Unsatisfied)",
			}}
			celFields={{
				id: "student ID",
				name: "student name",
				grade: "year group",
				legal_sex: "F, M, or X",
				requirements_ok: "requirements satisfied (boolean)",
				budgeted_periods: "budgeted periods used (number)",
				max_budgeted_periods: "their cap, or null for none",
				course_ids: "list of enrolled course IDs",
				categories: "list of distinct categories enrolled in",
				period_count: "periods occupied (number)",
				enrollment_count: "number of enrollments",
			}}
			celExample={'grade == "Y9" && !requirements_ok'}
		/>

		<p class="admin-list-count">
			{#if filtered.length === data.students.length}
				{data.students.length} students
			{:else}
				{filtered.length} of {data.students.length} students shown
			{/if}
		</p>

		<div class="scrolls admin-table">
			<table>
				<thead>
					<tr>
						<th scope="col">ID</th>
						<th scope="col">Name</th>
						<th scope="col">Grade</th>
						<th scope="col">Legal sex</th>
						<th scope="col">Budget</th>
						<th scope="col">Requirements</th>
						<th scope="col">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each filtered as student (student.id)}
						<StudentRow
							{student}
							{gradeIDs}
							{sexes}
							busy={data.busy}
							courses={data.courses}
							enrollments={data.enrollments}
							periods={data.periodIDs}
							budget={budgetText(student.id)}
							requirements={requirementsText(student.id)}
							editing={editing === student.id}
							scheduleOpen={scheduleFor === student.id}
							onedit={(): void => {
								editing = student.id
							}}
							oncancel={(): void => {
								editing = null
							}}
							onsave={(draft: AdminStudent): void => {
								void runAccepting(
									async (accept): Promise<void> => {
										await upsertStudents([draft], accept)
										editing = null
									},
								)
							}}
							ondelete={(): void => {
								void run(() => deleteStudent(student.id))
							}}
							ontoggleschedule={(): void => {
								scheduleFor =
									scheduleFor === student.id
										? null
										: student.id
							}}
						/>
					{:else}
						<tr>
							<td colspan="7">
								{data.students.length === 0
									? "No students yet."
									: "No students match your filter."}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</section>

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
