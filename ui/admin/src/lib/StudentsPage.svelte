<script lang="ts">
	import {
		deleteStudent,
		startStudentSession,
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

	// Sign in as a student and look at their page.
	//
	// The tab is opened on the click itself, before anything is
	// awaited: a window.open that happens after a fetch resolves is a
	// pop-up as far as the browser is concerned, and is blocked. So it
	// opens blank, and is pointed at the student area once the cookie
	// is actually set — or closed again if minting was refused, rather
	// than left sitting on a blank page.
	//
	// Only the student cookie is written, so this tab keeps its
	// administrator session and nothing here has to be signed into
	// again afterwards.
	function impersonate(id: string): void {
		const tab = window.open("", "_blank")
		if (tab === null) {
			data.report(
				new Error(
					`Allow pop-ups for this site to open the student view of ${id}.`,
				),
				"Could not open a tab",
			)
			return
		}
		// No resources are named: this changes nothing in the database,
		// so there is nothing on this page to reload.
		void data
			.run(() => startStudentSession(id))
			.then((ok): void => {
				if (ok) {
					tab.location.replace("/student/")
				} else {
					tab.close()
				}
			})
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

	// A student's address is their id under the student domain. There
	// is no email column to read: 0002_types.sql defines the key as
	// "the localpart of the school email address", and the auth
	// boundary is where the two halves were last together.
	//
	// That same file allows a staff localpart as a student key, for a
	// teacher enrolled to test with. Those really live under
	// ykpaoschool.cn and nothing stored here tells them apart, so a
	// roster holding one yields one wrong address — visible in the To:
	// field, where it can be fixed.
	const studentDomain = "stu.ykpaoschool.cn"

	// The listed students, not every student: an administrator who has
	// filtered to one grade means that grade.
	const addresses = $derived(
		filtered.map((s): string => `${s.id}@${studentDomain}`),
	)

	// Cleared on a timer, so copying the same list twice still says
	// something happened.
	let copied = $state(false)
	let copyTimer = 0

	async function copyAddresses(): Promise<void> {
		try {
			// Semicolons: Outlook splits recipients on them. It splits
			// on commas only if that setting has been turned on, and it
			// is off by default — a comma-separated paste then lands as
			// one long unresolvable recipient instead of a class.
			await navigator.clipboard.writeText(addresses.join("; "))
			copied = true
			window.clearTimeout(copyTimer)
			copyTimer = window.setTimeout((): void => {
				copied = false
			}, 4000)
		} catch (err) {
			// Writing to the clipboard needs a secure context, which a
			// plain-HTTP deployment is not. Better said out loud than
			// left as a paste of whatever was in there before.
			data.report(err, "Could not copy to the clipboard")
		}
	}
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

			{#snippet copyTool()}
				<p>
					<button
						type="button"
						class="linklike"
						disabled={addresses.length === 0}
						onclick={(): void => {
							void copyAddresses()
						}}
					>
						Copy students&rsquo; email addresses
					</button>
					<!--
						Polite and always present, so the confirmation is
						announced rather than only seen, and so the region
						is not created at the moment it gets its text.
					-->
					<span aria-live="polite">
						{copied ? `Copied ${String(addresses.length)}.` : ""}
					</span>
				</p>
			{/snippet}

			<DataTools
				section="students"
				exportHref="/admin/api/students/export"
				importAction="/admin/api/students/import"
				busy={data.busy}
				{run}
				extra={copyTool}
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
							onimpersonate={(): void => {
								impersonate(student.id)
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
