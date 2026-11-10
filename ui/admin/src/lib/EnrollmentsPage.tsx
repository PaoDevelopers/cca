import {
	checkEnrollmentImport,
	commitEnrollmentImport,
	removeEnrollments,
	type EnrollmentImportCheck,
} from "@common/adminApi"
import { filterByCel, useCelParser, type FilterMode } from "@common/cel"
import { Button } from "@common/components/ui/button"
import {
	Card,
	CardContent,
	CardHeader,
	CardTitle,
} from "@common/components/ui/card"
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
import { isViolationError } from "@common/http"
import type { Course, Enrollment } from "@common/types"
import {
	memo,
	startTransition,
	useDeferredValue,
	useEffect,
	useMemo,
	useState,
	type ReactElement,
} from "react"
import { ConfirmDelete } from "./ConfirmDelete"
import { DataTools } from "./DataTools"
import { useAdminData } from "./data"
import { EnrollmentForm } from "./EnrollmentForm"
import { EnrollmentImportDraft } from "./EnrollmentImportDraft"
import {
	courseLabel as courseLabelOf,
	enrollmentCelContext,
	enrollmentKey as key,
	policyLabel,
} from "./enrollments"
import { FilterBar } from "./FilterBar"
import { SortableHead, sortRows, type Sort } from "./sorting"

interface Props {
	// The route's ?q=, owned by the shell so the box and the URL stay
	// one value.
	filter: string
	setFilter: (filter: string) => void
}

// The sortable columns, each with the text it sorts by.
const columns = {
	student: (e: Enrollment): string => e.student_id,
	name: (e: Enrollment): string => e.student_name,
	grade: (e: Enrollment): string => e.grade_id,
	course: (e: Enrollment): string => e.course_id,
	title: (e: Enrollment): string => e.course_name,
	policy: (e: Enrollment): string => policyLabel(e),
} as const
type Column = keyof typeof columns

export function EnrollmentsPage({ filter, setFilter }: Props): ReactElement {
	const data = useAdminData()

	useEffect((): void => {
		data.want("enrollments", "courses")
	}, [data])

	const courseByID = useMemo(
		(): Map<string, Course> =>
			new Map(data.courses.map((c): [string, Course] => [c.id, c])),
		[data.courses],
	)

	function courseLabel(id: string, name: string): string {
		return courseLabelOf(courseByID, id, name)
	}

	// When the course meets. Held on the course rather than the
	// enrollment, so a course that is no longer listed leaves this
	// blank rather than the row missing.
	function coursePeriods(id: string): string[] {
		return courseByID.get(id)?.period_ids ?? []
	}

	const [mode, setMode] = useState<FilterMode>("simple")
	const celLoaded = useCelParser(mode)
	// Row selection for bulk deletion.
	const [selected, setSelected] = useState<string[]>([])
	const [sort, setSortNow] = useState<Sort<Column> | null>(null)
	// Thousands of rows once a window has run: a keystroke or a sort
	// renders them in the background, so the box and the header answer
	// at once and the rows follow.
	const query = useDeferredValue(filter)
	const setSort = (next: Sort<Column> | null): void => {
		startTransition((): void => {
			setSortNow(next)
		})
	}
	// The first screenful paints at once and the rest follows.
	const ready = data.ready("enrollments", "courses")
	const [allRows, setAllRows] = useState(false)
	useEffect((): void => {
		if (ready) {
			startTransition((): void => {
				setAllRows(true)
			})
		}
	}, [ready])

	function run(action: () => Promise<void>): Promise<boolean> {
		return data.run(action, "enrollments", "courses")
	}

	// Placement can break every negotiable rule, so it is confirmed
	// rather than overridden. Removal cannot break anything, so it takes
	// the plain path.
	function runAccepting(
		action: (accept: string[]) => Promise<void>,
	): Promise<boolean> {
		return data.runAccepting(action, "enrollments", "courses")
	}

	// A clean file goes straight in; one with conflicts is held as a
	// draft until every conflict has been decided. Numbered so a
	// reopened draft starts with no choices made.
	const [draft, setDraft] = useState<{
		id: number
		file: File
		check: EnrollmentImportCheck
	} | null>(null)

	function importFile(file: File): void {
		let held = false
		void run(async (): Promise<void> => {
			const check = await checkEnrollmentImport(file)
			if (check.violations.length === 0 && check.unknown.length === 0) {
				await commitEnrollmentImport(file, [], [])
			} else {
				held = true
				setDraft({ id: Date.now(), file, check })
			}
		}).then((ok): void => {
			if (ok && !held) {
				toast(`Imported ${file.name} into enrollments.`)
			}
		})
	}

	// Something changed between the check and the upload if the upload
	// is refused, so the file is checked again and the draft reopened.
	function commitDraft(file: File, drop: string[], accept: string[]): void {
		setDraft(null)
		let reopened = false
		void run(async (): Promise<void> => {
			try {
				await commitEnrollmentImport(file, drop, accept)
			} catch (err) {
				if (!isViolationError(err)) {
					throw err
				}
				reopened = true
				setDraft({
					id: Date.now(),
					file,
					check: await checkEnrollmentImport(file),
				})
			}
		}).then((ok): void => {
			if (ok && !reopened) {
				toast(`Imported ${file.name} into enrollments.`)
			}
		})
	}

	// Removals are grouped by course: remove_enrollments takes one
	// course and many students, which is also the order its locks are
	// taken in.
	function removeMany(targets: Enrollment[]): void {
		const byCourse = new Map<string, string[]>()
		for (const target of targets) {
			byCourse.set(target.course_id, [
				...(byCourse.get(target.course_id) ?? []),
				target.student_id,
			])
		}
		void run(async (): Promise<void> => {
			for (const [courseID, studentIDs] of [...byCourse].sort()) {
				await removeEnrollments(courseID, studentIDs)
			}
		}).then((ok): void => {
			if (ok) {
				toast(
					targets.length === 1
						? "Enrollment deleted."
						: `${String(targets.length)} enrollments deleted.`,
				)
			}
		})
	}

	// Memoized on what it reads: the box's own keystroke re-renders the
	// page before the deferred query moves, and that render should not
	// filter thousands of rows again only to get the same answer.
	const { celResult, filtered } = useMemo(() => {
		const cel =
			mode === "cel"
				? filterByCel(query, data.enrollments, (s) =>
						enrollmentCelContext(s, courseByID.get(s.course_id)),
					)
				: { rows: data.enrollments, error: null, unfiltered: false }

		const matching =
			mode === "cel"
				? cel.rows
				: data.enrollments.filter((s): boolean =>
						matchesFilter(query, {
							student: { exact: s.student_id },
							name: s.student_name,
							grade: { exact: s.grade_id },
							course: { exact: s.course_id },
							title: s.course_name,
							period: {
								exact:
									courseByID.get(s.course_id)?.period_ids ??
									[],
							},
							policy: policyLabel(s),
							droppable: s.student_droppable ? "yes" : "no",
							budgeted: s.counts_toward_budget ? "yes" : "no",
						}),
					)

		const sorted = sortRows(matching, sort, (e, column) =>
			columns[column](e),
		)
		return { celResult: cel, filtered: sorted }
		// celLoaded is read by filterByCel through the module, not here:
		// it is the signal that the parser arrived and the answer moved.
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [mode, query, data.enrollments, courseByID, sort, celLoaded])

	const shown = allRows ? filtered : filtered.slice(0, 50)

	// Functional updates, so a row that skipped a render still toggles
	// against the current selection rather than the one it last saw.
	const toggle = (k: string, checked: boolean): void => {
		setSelected((current): string[] =>
			checked ? [...current, k] : current.filter((s): boolean => s !== k),
		)
	}
	const removeOne = (enrollment: Enrollment): void => {
		removeMany([enrollment])
	}

	// An enrollment deleted elsewhere leaves a stale key behind.
	// Dropping those at read time rather than writing the selection back
	// keeps it a plain input to the render, with nothing to converge.
	const liveKeys = new Set(data.enrollments.map(key))
	const liveSelected = selected.filter((k): boolean => liveKeys.has(k))
	const selectedSet = new Set(liveSelected)

	const allFilteredSelected =
		filtered.length > 0 &&
		filtered.every((s): boolean => selectedSet.has(key(s)))

	// A filter that did not compile shows every row, so "all filtered"
	// would be "everything" — and the bulk controls are the one place
	// where that is destructive rather than merely wrong. The rows stay
	// on screen; only acting on them in bulk is withheld until the
	// expression means something.
	const bulkUnavailable = mode === "cel" && celResult.unfiltered

	return (
		<section
			aria-labelledby="enrollments-heading"
			className="flex flex-col gap-4"
		>
			{/*
				tabIndex because it is also where the data layer puts the
				focus when the control that started a write no longer
				exists.
			*/}
			<div className="flex flex-wrap items-center justify-between gap-2">
				<h2
					id="enrollments-heading"
					tabIndex={-1}
					className="text-2xl font-semibold tracking-tight outline-none"
				>
					Enrollments
				</h2>
			</div>

			{data.ready("enrollments", "courses") && (
				<DataTools
					section="enrollments"
					exportHref="/admin/api/enrollments/export"
					importAction="/admin/api/enrollments/import"
					onimport={importFile}
					busy={data.busy}
					run={run}
				/>
			)}

			{!data.ready("enrollments", "courses") ? (
				<p className="text-muted-foreground">Loading…</p>
			) : (
				<>
					<Card>
						<CardHeader>
							<CardTitle>Place students</CardTitle>
						</CardHeader>
						<CardContent>
							<EnrollmentForm
								courses={data.courses}
								courseByID={courseByID}
								busy={data.busy}
								runAccepting={runAccepting}
							/>
						</CardContent>
					</Card>

					<FilterBar
						query={filter}
						onQuery={setFilter}
						mode={mode}
						onMode={setMode}
						error={mode === "cel" ? celResult.error : null}
						summary={
							filtered.length === data.enrollments.length
								? `${String(data.enrollments.length)} enrollments`
								: `${String(filtered.length)} of ${String(data.enrollments.length)} enrollments shown`
						}
						fields={{
							student: "student ID (exact)",
							name: "student name",
							grade: "year group (exact)",
							course: "course ID (exact)",
							title: "course title",
							period: "meeting period ID (exact)",
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
							"course.category_id":
								"category of the enrolled course",
							"course.term": "term label",
							"course.teacher": "lead teacher",
							"course.location": "room or venue",
							"course.cost": "cost text, empty when free",
							"course.invite_only": "invite-only (boolean)",
							"course.periods": "list of meeting period IDs",
							"course.max_students":
								"capacity (number), null when uncapped",
							"course.full":
								"at or over capacity (boolean); false when uncapped",
							"course.current_students":
								"enrolled count (number)",
							droppable: "may the student drop it (boolean)",
							budgeted: "does it charge their budget (boolean)",
						}}
						celExample={
							'course.category_id == "SPORT" && student.grade == "Y9"'
						}
					/>

					{liveSelected.length > 0 && (
						<div className="flex items-center gap-3 rounded-lg border bg-muted/50 px-3 py-2 text-sm">
							<span>
								{liveSelected.length}{" "}
								{liveSelected.length === 1
									? "enrollment"
									: "enrollments"}{" "}
								selected.
							</span>
							<ConfirmDelete
								action="Delete"
								target={`the ${String(liveSelected.length)} selected enrollments`}
								disabled={data.busy || bulkUnavailable}
								onconfirm={(): void => {
									removeMany(
										data.enrollments.filter((s): boolean =>
											selectedSet.has(key(s)),
										),
									)
								}}
							/>
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={(): void => {
									setSelected([])
								}}
							>
								Clear selection
							</Button>
						</div>
					)}

					<Table>
						<TableHeader>
							<TableRow>
								<TableHead scope="col" className="w-8">
									<Checkbox
										aria-label="Select all filtered enrollments"
										disabled={bulkUnavailable}
										checked={
											allFilteredSelected &&
											!bulkUnavailable
										}
										onChange={(event): void => {
											const keys = filtered.map(key)
											if (event.currentTarget.checked) {
												setSelected([
													...new Set([
														...selected,
														...keys,
													]),
												])
											} else {
												const drop = new Set(keys)
												setSelected(
													selected.filter(
														(k): boolean =>
															!drop.has(k),
													),
												)
											}
										}}
									/>
								</TableHead>
								<SortableHead
									column="student"
									label="Student"
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
									column="grade"
									label="Grade"
									sort={sort}
									onSort={setSort}
								/>
								<SortableHead
									column="course"
									label="Course"
									sort={sort}
									onSort={setSort}
								/>
								<SortableHead
									column="title"
									label="Title"
									sort={sort}
									onSort={setSort}
								/>
								<TableHead scope="col">Periods</TableHead>
								<SortableHead
									column="policy"
									label="Policy"
									sort={sort}
									onSort={setSort}
								/>
								<TableHead scope="col">
									<span className="sr-only">Actions</span>
								</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{shown.map((enrollment) => {
								const k = key(enrollment)
								return (
									<EnrollmentRow
										key={k}
										enrollment={enrollment}
										rowKey={k}
										selected={selectedSet.has(k)}
										busy={data.busy}
										periods={coursePeriods(
											enrollment.course_id,
										).join(", ")}
										label={courseLabel(
											enrollment.course_id,
											enrollment.course_name,
										)}
										ontoggle={toggle}
										ondelete={removeOne}
									/>
								)
							})}
							{filtered.length === 0 && (
								<TableRow>
									<TableCell
										colSpan={9}
										className="text-muted-foreground"
									>
										{data.enrollments.length === 0
											? "No enrollments yet."
											: "No enrollments match your filter."}
									</TableCell>
								</TableRow>
							)}
						</TableBody>
					</Table>
				</>
			)}

			{draft !== null && (
				<EnrollmentImportDraft
					key={draft.id}
					check={draft.check}
					holds={(studentID, courseID): boolean =>
						liveKeys.has(`${studentID}:${courseID}`)
					}
					courseLabel={(id): string =>
						courseLabel(id, courseByID.get(id)?.name ?? id)
					}
					upload={(drop, accept): void => {
						commitDraft(draft.file, drop, accept)
					}}
					cancel={(): void => {
						setDraft(null)
					}}
				/>
			)}
		</section>
	)
}

// One enrollment's row. It re-renders only when what it shows changes:
// the callbacks are ignored, since they reach the page's state through
// functional updates and the shared AdminData, never a captured value.
function EnrollmentRowImpl({
	enrollment,
	rowKey,
	selected,
	busy,
	periods,
	label,
	ontoggle,
	ondelete,
}: {
	enrollment: Enrollment
	rowKey: string
	selected: boolean
	busy: boolean
	periods: string
	label: string
	ontoggle: (k: string, checked: boolean) => void
	ondelete: (enrollment: Enrollment) => void
}): ReactElement {
	return (
		<TableRow data-state={selected ? "selected" : undefined}>
			<TableCell>
				<Checkbox
					aria-label={`Select the enrollment of ${enrollment.course_id} for ${enrollment.student_id}`}
					checked={selected}
					onChange={(event): void => {
						ontoggle(rowKey, event.currentTarget.checked)
					}}
				/>
			</TableCell>
			<th scope="row" className="px-3 py-2 text-left font-medium">
				{enrollment.student_id}
			</th>
			<TableCell>{enrollment.student_name}</TableCell>
			<TableCell>{enrollment.grade_id}</TableCell>
			<TableCell>
				<code className="rounded bg-muted px-1 py-0.5 text-xs">
					{enrollment.course_id}
				</code>
			</TableCell>
			<TableCell>{label}</TableCell>
			{/*
				Labelled when blank, as in the courses table: an empty
				cell read out as nothing says only that the reader lost
				their place.
			*/}
			<TableCell aria-label={periods === "" ? "None" : undefined}>
				{periods}
			</TableCell>
			<TableCell>{policyLabel(enrollment)}</TableCell>
			<TableCell className="text-right">
				<ConfirmDelete
					action="Delete"
					target={`the enrollment of ${enrollment.course_id} for ${enrollment.student_id}`}
					disabled={busy}
					onconfirm={(): void => {
						ondelete(enrollment)
					}}
				/>
			</TableCell>
		</TableRow>
	)
}

const EnrollmentRow = memo(
	EnrollmentRowImpl,
	(a, b): boolean =>
		a.enrollment === b.enrollment &&
		a.selected === b.selected &&
		a.busy === b.busy &&
		a.periods === b.periods &&
		a.label === b.label,
)
