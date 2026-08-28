<script lang="ts">
	import { removeEnrollments } from "@common/adminApi"
	import type { Course, Enrollment } from "@common/types"
	import FilterBar from "./FilterBar.svelte"
	import DataTools from "./DataTools.svelte"
	import { matchesFilter } from "@common/filter"
	import { filterByCel, type FilterMode } from "@common/cel"
	import ConfirmButton from "./ConfirmButton.svelte"
	import EnrollmentForm from "./EnrollmentForm.svelte"
	import { getAdminData } from "./data.svelte"
	import {
		courseLabel as courseLabelOf,
		enrollmentCelContext,
		enrollmentKey as key,
		policyLabel,
	} from "./enrollments"

	interface Props {
		// The route's ?q=, bound so the box and the URL stay one value.
		filter: string
	}

	let { filter = $bindable("") }: Props = $props()

	const data = getAdminData()
	data.want("enrollments", "courses")

	const courseByID = $derived(
		new Map(data.courses.map((c): [string, Course] => [c.id, c])),
	)

	function courseLabel(id: string, name: string): string {
		return courseLabelOf(courseByID, id, name)
	}

	let mode = $state<FilterMode>("simple")
	// Row selection for bulk deletion.
	let selected = $state<string[]>([])

	function run(action: () => Promise<void>): Promise<boolean> {
		return data.run(action, "enrollments", "courses")
	}

	// Placement can break every negotiable rule, so it is confirmed
	// rather than overridden. Removal cannot break anything, so it
	// takes the plain path.
	function runAccepting(
		action: (accept: string[]) => Promise<void>,
	): Promise<boolean> {
		return data.runAccepting(action, "enrollments", "courses")
	}

	// Removals are grouped by course: remove_enrollments takes one
	// course and many students, which is also the order its locks are
	// taken in.
	function removeMany(targets: Enrollment[]): void {
		const byCourse = new Map<string, string[]>()
		for (const target of targets) {
			const list = byCourse.get(target.course_id)
			if (list === undefined) {
				byCourse.set(target.course_id, [target.student_id])
			} else {
				list.push(target.student_id)
			}
		}
		void run(async (): Promise<void> => {
			for (const [courseID, studentIDs] of [...byCourse].sort()) {
				await removeEnrollments(courseID, studentIDs)
			}
		})
	}

	function celContext(s: Enrollment): Record<string, unknown> {
		return enrollmentCelContext(s, courseByID.get(s.course_id))
	}

	const celResult = $derived(
		mode === "cel"
			? filterByCel(filter, data.enrollments, celContext)
			: { rows: data.enrollments, error: null, unfiltered: false },
	)

	const filtered = $derived(
		mode === "cel"
			? celResult.rows
			: data.enrollments.filter((s): boolean =>
					matchesFilter(filter, {
						student: { exact: s.student_id },
						name: s.student_name,
						grade: { exact: s.grade_id },
						course: { exact: s.course_id },
						title: s.course_name,
						policy: policyLabel(s),
						droppable: s.student_droppable ? "yes" : "no",
						budgeted: s.counts_toward_budget ? "yes" : "no",
					}),
				),
	)

	// An enrollment deleted elsewhere leaves a stale key behind.
	// Dropping those at read time rather than writing `selected` back
	// keeps the selection a plain input to the render, with nothing to
	// converge.
	const liveKeys = $derived(new Set(data.enrollments.map(key)))
	const liveSelected = $derived(
		selected.filter((k): boolean => liveKeys.has(k)),
	)
	const selectedSet = $derived(new Set(liveSelected))

	const allFilteredSelected = $derived(
		filtered.length > 0 &&
			filtered.every((s): boolean => selectedSet.has(key(s))),
	)

	// A filter that did not compile shows every row, so "all filtered"
	// would be "everything" — and the bulk controls are the one place
	// where that is destructive rather than merely wrong. The rows
	// stay on screen; only acting on them in bulk is withheld until
	// the expression means something.
	const bulkUnavailable = $derived(mode === "cel" && celResult.unfiltered)
</script>

<section aria-labelledby="enrollments-heading">
	<!--
		The page had no heading below the h1 at all, so heading
		navigation went straight from "CCA administration" to nothing and
		the section was an unlabelled region. tabindex because it is also
		where the data layer puts the focus when the control that started
		a write no longer exists.
	-->
	<h2 id="enrollments-heading" tabindex="-1">Enrollments</h2>

	{#if !data.ready("enrollments", "courses")}
		<p>Loading&hellip;</p>
	{:else}
		<div class="admin-actions">
			<details class="create-panel">
				<summary>Place students</summary>
				<EnrollmentForm
					courses={data.courses}
					{courseByID}
					busy={data.busy}
					{runAccepting}
				/>
			</details>

			<DataTools
				section="enrollments"
				exportHref="/admin/api/enrollments/export"
				importAction="/admin/api/enrollments/import"
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
				student: "student ID (exact)",
				name: "student name",
				grade: "year group (exact)",
				course: "course ID (exact)",
				title: "course title",
				policy: "Own pick, Invitation, Committed, or Placed",
				droppable: "may the student drop it (yes or no)",
				budgeted: "does it charge their budget (yes or no)",
			}}
			celFields={{
				"student.id": "student ID",
				"student.name": "student name",
				"student.grade": "year group",
				"course.id": "course ID",
				"course.name": "course title",
				"course.category_id": "category of the enrolled course",
				"course.term": "term label",
				"course.teacher": "lead teacher",
				"course.location": "room or venue",
				"course.cost": "cost text, empty when free",
				"course.invite_only": "invite-only (boolean)",
				"course.periods": "list of meeting period IDs",
				"course.max_students": "capacity (number), null when uncapped",
				"course.full":
					"at or over capacity (boolean); false when uncapped",
				"course.current_students": "enrolled count (number)",
				droppable: "may the student drop it (boolean)",
				budgeted: "does it charge their budget (boolean)",
			}}
			celExample={'course.category_id == "SPORT" && student.grade == "Y9"'}
		/>

		<div class="admin-list-status">
			<p>
				{#if filtered.length === data.enrollments.length}
					{data.enrollments.length} enrollments
				{:else}
					{filtered.length} of {data.enrollments.length} enrollments shown
				{/if}
			</p>
			{#if liveSelected.length > 0}
				<p>
					{liveSelected.length}
					{liveSelected.length === 1 ? "enrollment" : "enrollments"} selected.
					<ConfirmButton
						action="Delete"
						target="the {liveSelected.length} selected enrollments"
						disabled={data.busy || bulkUnavailable}
						onconfirm={(): void => {
							removeMany(
								data.enrollments.filter((s): boolean =>
									selectedSet.has(key(s)),
								),
							)
						}}
					/>
				</p>
			{/if}
		</div>

		<div class="scrolls admin-table">
			<table>
				<thead>
					<tr>
						<th scope="col">
							<input
								type="checkbox"
								aria-label="Select all filtered enrollments"
								disabled={bulkUnavailable}
								checked={allFilteredSelected &&
									!bulkUnavailable}
								onchange={(event): void => {
									const keys = filtered.map(key)
									if (event.currentTarget.checked) {
										selected = [
											...new Set([...selected, ...keys]),
										]
									} else {
										const drop = new Set(keys)
										selected = selected.filter(
											(k): boolean => !drop.has(k),
										)
									}
								}}
							/>
						</th>
						<th scope="col">Student</th>
						<th scope="col">Name</th>
						<th scope="col">Grade</th>
						<th scope="col">Course</th>
						<th scope="col">Title</th>
						<th scope="col">Policy</th>
						<th scope="col">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each filtered as enrollment (key(enrollment))}
						<tr>
							<td>
								<input
									type="checkbox"
									aria-label="Select the enrollment of {enrollment.course_id} for {enrollment.student_id}"
									value={key(enrollment)}
									bind:group={selected}
								/>
							</td>
							<th scope="row">{enrollment.student_id}</th>
							<td>{enrollment.student_name}</td>
							<td>{enrollment.grade_id}</td>
							<td><code>{enrollment.course_id}</code></td>
							<td>
								{courseLabel(
									enrollment.course_id,
									enrollment.course_name,
								)}
							</td>
							<td>{policyLabel(enrollment)}</td>
							<td>
								<ConfirmButton
									action="Delete"
									target="the enrollment of {enrollment.course_id} for {enrollment.student_id}"
									disabled={data.busy}
									onconfirm={(): void => {
										removeMany([enrollment])
									}}
								/>
							</td>
						</tr>
					{:else}
						<tr>
							<td colspan="8">
								{data.enrollments.length === 0
									? "No enrollments yet."
									: "No enrollments match your filter."}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</section>
