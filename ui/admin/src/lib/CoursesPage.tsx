import {
	createCourse,
	deleteCourse,
	updateCourse,
	type CourseInput,
} from "@common/adminApi"
import { capacityLabel } from "@common/capacity"
import { filterByCel, useCelParser, type FilterMode } from "@common/cel"
import { Button } from "@common/components/ui/button"
import { Dialog, DialogHeader, DialogTitle } from "@common/components/ui/dialog"
import { Checkbox } from "@common/components/ui/input"
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@common/components/ui/table"
import { toast } from "@common/components/ui/toast"
import { matchesFilter } from "@common/filter"
import type { Course } from "@common/types"
import {
	startTransition,
	useDeferredValue,
	useEffect,
	useState,
	type ReactElement,
} from "react"
import { ConfirmDelete } from "./ConfirmDelete"
import { CourseEditor } from "./CourseEditor"
import { courseCelContext } from "./courses"
import { useAdminData } from "./data"
import { DataTools } from "./DataTools"
import { FilterBar } from "./FilterBar"
import { ListIcon, PencilIcon } from "./icons"
import { SortableHead, sortRows, type Sort } from "./sorting"

interface Props {
	// The route's ?q=, owned by the shell so the box and the URL stay
	// one value.
	filter: string
	setFilter: (filter: string) => void
}

// The sortable columns, and what each sorts by. Unsorted is the
// server's order, which is the catalogue's own.
const sortKeys = {
	id: (c: Course): string => c.id,
	name: (c: Course): string => c.name,
	category: (c: Course): string => c.category_id,
	term: (c: Course): string => c.term,
	students: (c: Course): number => c.current_students,
} as const
type SortKey = keyof typeof sortKeys

const columnCount = 9

export function CoursesPage({ filter, setFilter }: Props): ReactElement {
	const data = useAdminData()
	useEffect((): void => {
		data.want("courses", "categories", "periods", "grades")
	}, [data])

	const [mode, setMode] = useState<FilterMode>("simple")
	useCelParser(mode)
	// The course in the sheet: an ID, "new" for the create sheet, or
	// nothing. A course deleted elsewhere closes its own sheet.
	const [editing, setEditing] = useState<string | null>(null)
	const [creating, setCreating] = useState(false)
	const [sort, setSortNow] = useState<Sort<SortKey> | null>(null)
	const setSort = (next: Sort<SortKey> | null): void => {
		startTransition((): void => {
			setSortNow(next)
		})
	}
	// The box answers each keystroke at once; the rows follow.
	const query = useDeferredValue(filter)

	// Row selection for bulk deletion, keyed by course ID. Pairs with
	// the term filter to clear out a season in one go.
	const [selected, setSelected] = useState<string[]>([])

	const run = (action: () => Promise<void>): Promise<boolean> =>
		data.run(action, "courses")

	// Editing a course can break the placements already in it —
	// rescheduling it into a clash, narrowing its restrictions,
	// shrinking it below its enrollment — so saving goes through the
	// accept protocol. Creating one cannot: it has no enrollees yet.
	const runAccepting = (
		action: (accept: string[]) => Promise<void>,
	): Promise<boolean> => data.runAccepting(action, "courses", "enrollments")

	if (!data.ready("courses", "categories", "periods", "grades")) {
		return (
			<section aria-labelledby="courses-heading">
				<h2 id="courses-heading" className="sr-only">
					Courses
				</h2>
				<p className="text-muted-foreground">Loading…</p>
			</section>
		)
	}

	const categories = data.categoryIDs
	const periods = data.periodIDs
	const gradeIDs = data.grades.map((g): string => g.id)

	const celResult =
		mode === "cel"
			? filterByCel(query, data.courses, courseCelContext)
			: { rows: data.courses, error: null, unfiltered: false }

	const filtered =
		mode === "cel"
			? celResult.rows
			: data.courses.filter((c): boolean =>
					matchesFilter(query, {
						id: { exact: c.id },
						name: c.name,
						category: { exact: c.category_id },
						teacher: c.teacher,
						location: c.location,
						period: { exact: c.period_ids },
						invite_only: c.invite_only ? "yes" : "no",
						term: { exact: c.term },
					}),
				)

	const shown = sortRows(filtered, sort, (c, key) => sortKeys[key](c))

	// A course deleted elsewhere leaves a stale ID behind. Dropping
	// those at read time rather than writing the selection back keeps it
	// a plain input to the render, with nothing to converge.
	const liveIDs = new Set(data.courses.map((c): string => c.id))
	const liveSelected = selected.filter((id): boolean => liveIDs.has(id))
	const selectedSet = new Set(liveSelected)

	const allFilteredSelected =
		filtered.length > 0 &&
		filtered.every((c): boolean => selectedSet.has(c.id))

	// A filter that did not compile shows every row, so "all filtered"
	// would be "everything" — and the bulk controls are the one place
	// where that is destructive rather than merely wrong. The rows stay
	// on screen; only acting on them in bulk is withheld until the
	// expression means something.
	const bulkUnavailable = mode === "cel" && celResult.unfiltered

	const editedCourse =
		editing === null
			? undefined
			: data.courses.find((c): boolean => c.id === editing)

	return (
		<section
			aria-labelledby="courses-heading"
			className="flex flex-col gap-4"
		>
			{/*
				tabIndex because it is also where the data layer puts the
				focus when the control that started a write no longer
				exists.
			*/}
			<div className="flex flex-wrap items-center justify-between gap-2">
				<h2
					id="courses-heading"
					tabIndex={-1}
					className="text-xl font-semibold tracking-tight outline-none"
				>
					Courses
				</h2>
				<Button
					type="button"
					size="sm"
					onClick={(): void => {
						setCreating(true)
					}}
				>
					Add course
				</Button>
			</div>

			<DataTools
				section="courses"
				exportHref="/admin/api/courses/export"
				importAction="/admin/api/courses/import"
				busy={data.busy}
				run={run}
			/>

			<FilterBar
				query={filter}
				onQuery={setFilter}
				mode={mode}
				onMode={setMode}
				error={mode === "cel" ? celResult.error : null}
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
					max_students: "capacity (number), null when uncapped",
					full: "at or over capacity (boolean); false when uncapped",
					current_students: "enrolled count (number)",
					allowed_grades: "list of permitted year groups",
					allowed_legal_sexes: "list of permitted legal sexes",
				}}
				celExample={'term == "Semester" && full'}
			/>

			<div className="admin-list-status flex min-h-8 flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
				<p>
					{filtered.length === data.courses.length
						? `${String(data.courses.length)} courses`
						: `${String(filtered.length)} of ${String(data.courses.length)} courses shown`}
				</p>
				{liveSelected.length > 0 && (
					<p className="flex items-center gap-2 text-foreground">
						{liveSelected.length}{" "}
						{liveSelected.length === 1 ? "course" : "courses"}{" "}
						selected.
						<ConfirmDelete
							action="Delete"
							target={`the ${String(liveSelected.length)} selected courses`}
							disabled={data.busy || bulkUnavailable}
							detail="Their enrollments go with them. This cannot be undone."
							onconfirm={(): void => {
								const targets = [...liveSelected]
								void run(async (): Promise<void> => {
									for (const id of targets) {
										await deleteCourse(id)
									}
								}).then((ok): void => {
									if (ok) {
										toast(
											`Deleted ${String(targets.length)} courses`,
										)
									}
								})
							}}
						/>
					</p>
				)}
			</div>

			<Table>
				<TableHeader>
					<TableRow>
						<TableHead scope="col" className="w-10">
							<Checkbox
								aria-label="Select all filtered courses"
								disabled={bulkUnavailable}
								checked={
									allFilteredSelected && !bulkUnavailable
								}
								onChange={(event): void => {
									const ids = filtered.map(
										(c): string => c.id,
									)
									if (event.currentTarget.checked) {
										setSelected([
											...new Set([...selected, ...ids]),
										])
									} else {
										const drop = new Set(ids)
										setSelected(
											selected.filter(
												(id): boolean => !drop.has(id),
											),
										)
									}
								}}
							/>
						</TableHead>
						<SortableHead
							column="id"
							label="ID"
							sort={sort}
							onSort={setSort}
						/>
						<SortableHead
							column="name"
							label="Name"
							sort={sort}
							onSort={setSort}
						/>
						<SortableHead
							column="category"
							label="Category"
							sort={sort}
							onSort={setSort}
						/>
						<TableHead scope="col">Periods</TableHead>
						<SortableHead
							column="term"
							label="Term"
							sort={sort}
							onSort={setSort}
						/>
						<SortableHead
							column="students"
							label="Students"
							sort={sort}
							onSort={setSort}
						/>
						<TableHead scope="col">Invite only</TableHead>
						<TableHead scope="col" className="text-right">
							Actions
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{shown.map((course) => (
						<TableRow
							key={course.id}
							data-state={
								selectedSet.has(course.id)
									? "selected"
									: undefined
							}
						>
							<TableCell>
								<Checkbox
									aria-label={`Select ${course.id}`}
									checked={selectedSet.has(course.id)}
									onChange={(event): void => {
										setSelected(
											event.currentTarget.checked
												? [
														...new Set([
															...selected,
															course.id,
														]),
													]
												: selected.filter(
														(id): boolean =>
															id !== course.id,
													),
										)
									}}
								/>
							</TableCell>
							<TableCell>
								<code className="font-medium">{course.id}</code>
							</TableCell>
							<TableCell>{course.name}</TableCell>
							<TableCell>{course.category_id}</TableCell>
							<TableCell
								aria-label={
									course.period_ids.length === 0
										? "Empty"
										: undefined
								}
							>
								{course.period_ids.join(", ")}
							</TableCell>
							<TableCell>{course.term}</TableCell>
							<TableCell className="tabular-nums">
								{course.current_students}/
								{capacityLabel(course.max_students)}
							</TableCell>
							<TableCell>
								{course.invite_only ? "Yes" : "No"}
							</TableCell>
							<TableCell>
								<div className="flex justify-end gap-1">
									<Button
										type="button"
										variant="outline"
										size="icon"
										className="text-muted-foreground hover:text-foreground"
										aria-label={`View enrollments of ${course.id}`}
										tooltip={`View enrollments of ${course.id}`}
										onClick={(): void => {
											window.location.hash = `#/enrollments?q=${encodeURIComponent(
												`course:${course.id}`,
											)}`
										}}
									>
										<ListIcon />
									</Button>
									<Button
										type="button"
										variant="outline"
										size="icon"
										className="text-muted-foreground hover:text-foreground"
										disabled={data.busy}
										aria-label={`Edit ${course.id}`}
										tooltip={`Edit ${course.id}`}
										onClick={(): void => {
											setEditing(course.id)
										}}
									>
										<PencilIcon />
									</Button>
									<ConfirmDelete
										action="Delete"
										target={course.id}
										disabled={data.busy}
										detail="Its enrollments go with it. This cannot be undone."
										onconfirm={(): void => {
											void run((): Promise<void> =>
												deleteCourse(course.id),
											).then((ok): void => {
												if (ok) {
													toast(
														`Deleted ${course.id}`,
													)
												}
											})
										}}
									/>
								</div>
							</TableCell>
						</TableRow>
					))}
					{shown.length === 0 && (
						<TableRow>
							<TableCell
								colSpan={columnCount}
								className="py-8 text-center text-muted-foreground"
							>
								{data.courses.length === 0
									? "No courses yet."
									: "No courses match your filter."}
							</TableCell>
						</TableRow>
					)}
				</TableBody>
			</Table>

			{creating && (
				<Dialog
					side="right"
					aria-labelledby="course-sheet-title"
					onClose={(): void => {
						setCreating(false)
					}}
				>
					<DialogHeader>
						<DialogTitle id="course-sheet-title">
							Add course
						</DialogTitle>
					</DialogHeader>
					<CourseEditor
						initial={null}
						categories={categories}
						periods={periods}
						gradeIDs={gradeIDs}
						busy={data.busy}
						onsave={async (
							input: CourseInput,
						): Promise<boolean> => {
							const ok = await run((): Promise<void> =>
								createCourse(input),
							)
							if (ok) {
								toast(`Created ${input.id}`)
							}
							return ok
						}}
					/>
				</Dialog>
			)}

			{editedCourse !== undefined && (
				<Dialog
					// Remounted per course, so the editor's snapshot is of
					// the course it was opened for.
					key={editedCourse.id}
					side="right"
					aria-labelledby="course-sheet-title"
					onClose={(): void => {
						setEditing(null)
					}}
				>
					<DialogHeader>
						<DialogTitle id="course-sheet-title">
							Edit {editedCourse.id}
						</DialogTitle>
					</DialogHeader>
					<CourseEditor
						initial={editedCourse}
						categories={categories}
						periods={periods}
						gradeIDs={gradeIDs}
						busy={data.busy}
						onsave={async (
							input: CourseInput,
						): Promise<boolean> => {
							const ok = await runAccepting(
								(accept): Promise<void> =>
									updateCourse(editedCourse.id, {
										...input,
										accept,
									}),
							)
							if (ok) {
								toast(`Saved ${editedCourse.id}`)
							}
							return ok
						}}
					/>
				</Dialog>
			)}
		</section>
	)
}
