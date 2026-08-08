<script lang="ts">
	import {
		createCourse,
		deleteCourse,
		updateCourse,
		type CourseInput,
	} from "@common/adminApi"
	import FilterBar from "./FilterBar.svelte"
	import DataTools from "./DataTools.svelte"
	import { matchesFilter } from "@common/filter"
	import { filterByCel, type FilterMode } from "@common/cel"
	import ConfirmButton from "./ConfirmButton.svelte"
	import CourseEditor from "./CourseEditor.svelte"
	import CourseRow from "./CourseRow.svelte"
	import { courseCelContext } from "./courses"
	import { getAdminData } from "./data.svelte"

	interface Props {
		// The route's ?q=, bound so the box and the URL stay one value.
		filter: string
	}

	let { filter = $bindable("") }: Props = $props()

	const data = getAdminData()
	data.want("courses", "categories", "periods", "grades")

	const gradeIDs = $derived(data.grades.map((g): string => g.id))

	let mode = $state<FilterMode>("simple")
	let editing = $state<string | null>(null)
	let creating = $state(false)
	// Bumped to reset the create editor after a successful create.
	let createGeneration = $state(0)

	// Row selection for bulk deletion, keyed by course ID. Pairs with
	// the term filter to clear out a season in one go.
	let selected = $state<string[]>([])

	function run(action: () => Promise<void>): Promise<boolean> {
		return data.run(action, "courses")
	}

	// Editing a course can break the placements already in it —
	// rescheduling it into a clash, narrowing its restrictions,
	// shrinking it below its enrollment — so saving goes through the
	// accept protocol. Creating one cannot: it has no enrollees yet.
	function runAccepting(
		action: (accept: string[]) => Promise<void>,
	): Promise<boolean> {
		return data.runAccepting(action, "courses", "enrollments")
	}

	const celResult = $derived(
		mode === "cel"
			? filterByCel(filter, data.courses, courseCelContext)
			: { rows: data.courses, error: null, unfiltered: false },
	)

	const filtered = $derived(
		mode === "cel"
			? celResult.rows
			: data.courses.filter((c): boolean =>
					matchesFilter(filter, {
						id: { exact: c.id },
						name: c.name,
						category: { exact: c.category_id },
						teacher: c.teacher,
						location: c.location,
						period: { exact: c.period_ids },
						invite_only: c.invite_only ? "yes" : "no",
						term: { exact: c.term },
					}),
				),
	)

	// A course deleted elsewhere leaves a stale ID behind. Dropping
	// those at read time rather than writing `selected` back keeps the
	// selection a plain input to the render, with nothing to converge.
	const liveIDs = $derived(new Set(data.courses.map((c): string => c.id)))
	const liveSelected = $derived(
		selected.filter((id): boolean => liveIDs.has(id)),
	)
	const selectedSet = $derived(new Set(liveSelected))

	const allFilteredSelected = $derived(
		filtered.length > 0 &&
			filtered.every((c): boolean => selectedSet.has(c.id)),
	)

	// A filter that did not compile shows every row, so "all filtered"
	// would be "everything" — and the bulk controls are the one place
	// where that is destructive rather than merely wrong. The rows
	// stay on screen; only acting on them in bulk is withheld until
	// the expression means something.
	const bulkUnavailable = $derived(mode === "cel" && celResult.unfiltered)
</script>

<section aria-labelledby="courses-heading">
	<!--
		The page had no heading below the h1 at all, so heading
		navigation went straight from "CCA administration" to nothing and
		the section was an unlabelled region. tabindex because it is also
		where the data layer puts the focus when the control that started
		a write no longer exists.
	-->
	<h2 id="courses-heading" tabindex="-1">Courses</h2>

	{#if !data.ready("courses", "categories", "periods", "grades")}
		<p>Loading&hellip;</p>
	{:else}
		<div class="admin-actions">
			<details class="create-panel" bind:open={creating}>
				<summary>Add course</summary>
				{#key createGeneration}
					<CourseEditor
						initial={null}
						categories={data.categoryIDs}
						periods={data.periodIDs}
						{gradeIDs}
						busy={data.busy}
						onsave={(input): void => {
							void run(async (): Promise<void> => {
								await createCourse(input)
								createGeneration += 1
								creating = false
							})
						}}
					/>
				{/key}
			</details>

			<DataTools
				section="courses"
				exportHref="/admin/api/courses/export"
				importAction="/admin/api/courses/import"
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
				id: "course ID (exact)",
				name: "course title",
				category: "category (exact)",
				teacher: "lead teacher",
				location: "room or venue",
				period: "meeting period ID (exact)",
				invite_only: "yes or no",
				term: "term label (exact)",
			}}
			celFields={{
				id: "course ID",
				name: "course title",
				description: "course description",
				category_id: "category",
				term: "term label",
				cost: "cost text, empty when free",
				teacher: "lead teacher",
				teacher_email: "lead teacher e-mail",
				location: "room or venue",
				invite_only: "invite-only (boolean)",
				periods: "list of meeting period IDs",
				max_students: "capacity (number)",
				current_students: "enrolled count (number)",
				allowed_grades: "list of permitted year groups",
				allowed_legal_sexes: "list of permitted legal sexes",
			}}
			celExample={'term == "Semester" && current_students >= max_students'}
		/>

		<div class="admin-list-status">
			<p>
				{#if filtered.length === data.courses.length}
					{data.courses.length} courses
				{:else}
					{filtered.length} of {data.courses.length} courses shown
				{/if}
			</p>
			{#if liveSelected.length > 0}
				<p>
					{liveSelected.length}
					{liveSelected.length === 1 ? "course" : "courses"} selected.
					<ConfirmButton
						action="Delete"
						target="the {liveSelected.length} selected courses"
						disabled={data.busy || bulkUnavailable}
						onconfirm={(): void => {
							const targets = [...liveSelected]
							void run(async (): Promise<void> => {
								for (const id of targets) {
									await deleteCourse(id)
								}
							})
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
								aria-label="Select all filtered courses"
								disabled={bulkUnavailable}
								checked={allFilteredSelected &&
									!bulkUnavailable}
								onchange={(event): void => {
									const ids = filtered.map(
										(c): string => c.id,
									)
									if (event.currentTarget.checked) {
										selected = [
											...new Set([...selected, ...ids]),
										]
									} else {
										const drop = new Set(ids)
										selected = selected.filter(
											(id): boolean => !drop.has(id),
										)
									}
								}}
							/>
						</th>
						<th scope="col">ID</th>
						<th scope="col">Name</th>
						<th scope="col">Category</th>
						<th scope="col">Periods</th>
						<th scope="col">Term</th>
						<th scope="col">Students</th>
						<th scope="col">Invite only</th>
						<th scope="col">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each filtered as course (course.id)}
						<CourseRow
							{course}
							categories={data.categoryIDs}
							periods={data.periodIDs}
							{gradeIDs}
							busy={data.busy}
							editing={editing === course.id}
							selected={selectedSet.has(course.id)}
							onselect={(checked: boolean): void => {
								selected = checked
									? [...new Set([...selected, course.id])]
									: selected.filter(
											(id): boolean => id !== course.id,
										)
							}}
							ontoggleedit={(): void => {
								editing =
									editing === course.id ? null : course.id
							}}
							onsave={(input: CourseInput): void => {
								void runAccepting(
									async (accept): Promise<void> => {
										await updateCourse(course.id, {
											...input,
											accept,
										})
										editing = null
									},
								)
							}}
							ondelete={(): void => {
								void run(() => deleteCourse(course.id))
							}}
						/>
					{:else}
						<tr>
							<td colspan="9">
								{data.courses.length === 0
									? "No courses yet."
									: "No courses match your filter."}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</section>
