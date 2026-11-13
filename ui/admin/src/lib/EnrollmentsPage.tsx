import {
	checkEnrollmentImport,
	commitEnrollmentImport,
	discardPendingEnrollments,
	holdEnrollmentImport,
	pendingAsFile,
	removeEnrollments,
	type EnrollmentImportCheck,
	type PendingEnrollment,
} from "@common/adminApi"
import { filterByCel, useCelParser, type FilterMode } from "@common/cel"
import { Badge } from "@common/components/ui/badge"
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
import { EnrollmentImportDraft, row } from "./EnrollmentImportDraft"
import {
	clashingKeys,
	courseLabel as courseLabelOf,
	enrollmentCelContext,
	enrollmentKey as key,
	policyLabel,
} from "./enrollments"
import { FilterBar } from "./FilterBar"
import { SortableHead, sortRows, type Sort } from "./sorting"
import { copyStudentAddresses } from "./students"

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

	// Clashes an administrator let through, kept to be revisited: the
	// student is emailed from this list and one side dropped once they
	// answer — or both kept for good, as for two teams sharing a slot.
	const clashes = useMemo(
		(): Set<string> => clashingKeys(courseByID, data.enrollments),
		[courseByID, data.enrollments],
	)
	const clashingStudents = useMemo(
		(): number =>
			new Set(
				data.enrollments
					.filter((e): boolean => clashes.has(key(e)))
					.map((e): string => e.student_id),
			).size,
		[data.enrollments, clashes],
	)

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

	// A clean file goes straight in; one with conflicts is held open
	// until the administrator uploads or closes it. Numbered so a
	// reopened dialog starts with no choices made. source names the
	// draft whatever it leaves behind waits in: the file's own name, or
	// the draft's when one is being resolved.
	interface OpenImport {
		id: number
		file: File
		source: string
		check: EnrollmentImportCheck
	}
	const [draft, setDraft] = useState<OpenImport | null>(null)

	// A new upload never joins an existing draft that shares its name:
	// that draft's rows would merge into it and go with its discard.
	function freshSource(name: string): string {
		const taken = new Set(data.pendingImport.map((p): string => p.source))
		let source = name
		for (let n = 2; taken.has(source); n++) {
			source = name.replace(/(\.[^.]*)?$/, ` (${String(n)})$1`)
		}
		return source
	}

	function importFile(file: File, source = freshSource(file.name)): void {
		let held = false
		void run(async (): Promise<void> => {
			const check = await checkEnrollmentImport(file)
			if (check.violations.length === 0 && check.unknown.length === 0) {
				await commitEnrollmentImport(file, [], [], [], source)
			} else {
				held = true
				setDraft({ id: Date.now(), file, source, check })
			}
		}).then((ok): void => {
			if (ok && !held) {
				toast(`Imported ${source} into enrollments.`)
			}
		})
	}

	// Something changed between the check and the upload if the upload
	// is refused, so the file is checked again and the dialog reopened.
	// Any other failure reopens it as it was: the file is only here.
	function commitDraft(
		open: OpenImport,
		drop: string[],
		accept: string[],
		later: string[],
	): void {
		setDraft(null)
		let reopened = false
		let kept = 0
		void run(async (): Promise<void> => {
			try {
				kept = await commitEnrollmentImport(
					open.file,
					drop,
					accept,
					later,
					open.source,
				)
			} catch (err) {
				reopened = true
				if (!isViolationError(err)) {
					setDraft({ ...open, id: Date.now() })
					throw err
				}
				let check = open.check
				try {
					check = await checkEnrollmentImport(open.file)
				} finally {
					setDraft({ ...open, id: Date.now(), check })
				}
			}
		}).then((ok): void => {
			if (ok && !reopened) {
				const lost = later.length - kept
				const parts = [`Imported ${open.source} into enrollments`]
				if (kept > 0) {
					parts.push(
						`${String(kept)} ${kept === 1 ? "row" : "rows"} kept as a draft`,
					)
				}
				if (lost > 0) {
					parts.push(
						`${String(lost)} ${lost === 1 ? "row" : "rows"} left undecided weren’t applied or kept: the student is already enrolled, or the student or course was removed`,
					)
				}
				toast(`${parts.join("; ")}.`)
			}
		})
	}

	// Closed without uploading: nothing is placed, and the file waits as
	// a draft. Rows naming no such student or course cannot be kept. If
	// keeping it fails the dialog reopens, so the file is not lost.
	function holdDraft(open: OpenImport): void {
		setDraft(null)
		let kept = 0
		void run(async (): Promise<void> => {
			try {
				kept = await holdEnrollmentImport(
					open.file,
					open.check.unknown.map((u): string =>
						row(u.course_id, u.student_id),
					),
					open.source,
				)
			} catch (err) {
				setDraft({ ...open, id: Date.now() })
				throw err
			}
		}).then((ok): void => {
			if (ok) {
				toast(
					kept === 0
						? `Nothing from ${open.source} was imported or kept: every row names an unknown student or course, is already enrolled, or is in another draft.`
						: `Kept ${open.source} as a draft; nothing was imported.`,
				)
			}
		})
	}

	// One draft per uploaded file, in the server's order (by name).
	const bySource = new Map<string, PendingEnrollment[]>()
	for (const p of data.pendingImport) {
		const rows = bySource.get(p.source)
		if (rows === undefined) {
			bySource.set(p.source, [p])
		} else {
			rows.push(p)
		}
	}
	const drafts = [...bySource].map(([source, rows]) => ({ source, rows }))

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
						enrollmentCelContext(
							s,
							courseByID.get(s.course_id),
							clashes.has(key(s)),
						),
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
							clash: clashes.has(key(s)) ? "yes" : "no",
						}),
					)

		const sorted = sortRows(matching, sort, (e, column) =>
			columns[column](e),
		)
		return { celResult: cel, filtered: sorted }
		// celLoaded is read by filterByCel through the module, not here:
		// it is the signal that the parser arrived and the answer moved.
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [mode, query, data.enrollments, courseByID, clashes, sort, celLoaded])

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
					extra={
						<>
							<Button
								type="button"
								variant="outline"
								size="sm"
								disabled={clashes.size === 0}
								onClick={(): void => {
									setMode("simple")
									setFilter("clash:yes")
								}}
							>
								{clashes.size === 0
									? "No clashes"
									: `Show clashes (${String(clashingStudents)} ${clashingStudents === 1 ? "student" : "students"})`}
							</Button>
							<Button
								type="button"
								variant="outline"
								size="sm"
								disabled={
									filtered.length === 0 || bulkUnavailable
								}
								onClick={(): void => {
									void copyStudentAddresses(
										filtered.map(
											(e): string => e.student_id,
										),
										(err, fallback): void => {
											data.report(err, fallback)
										},
									)
								}}
							>
								Copy students’ email addresses
							</Button>
						</>
					}
				/>
			)}

			{data.ready("enrollments", "courses") && drafts.length > 0 && (
				<Card>
					<CardHeader>
						<CardTitle>
							<h3>Draft</h3>
						</CardTitle>
					</CardHeader>
					<CardContent>
						<ul className="flex flex-col divide-y">
							{drafts.map((d) => (
								<li
									key={d.source}
									className="flex items-center gap-3 py-2 first:pt-0 last:pb-0"
								>
									<span className="mr-auto min-w-0 font-medium break-words">
										{d.source}
									</span>
									<Button
										type="button"
										size="sm"
										disabled={data.busy}
										aria-label={`Resolve conflicts in ${d.source}`}
										onClick={(): void => {
											importFile(
												pendingAsFile(d.rows),
												d.source,
											)
										}}
									>
										Resolve Conflicts
									</Button>
									<ConfirmDelete
										action="Discard"
										target={`the draft ${d.source}`}
										detail={`Its ${String(d.rows.length)} ${d.rows.length === 1 ? "row is" : "rows are"} thrown away; none is enrolled.`}
										disabled={data.busy}
										onconfirm={(): void => {
											void run(() =>
												discardPendingEnrollments(
													d.source,
												),
											).then((ok): void => {
												if (ok) {
													toast(
														`Discarded ${d.source}.`,
													)
												}
											})
										}}
									/>
								</li>
							))}
						</ul>
					</CardContent>
				</Card>
			)}

			{!data.ready("enrollments", "courses") ? (
				<p className="text-muted-foreground">Loading…</p>
			) : (
				<>
					<Card>
						<CardHeader>
							<CardTitle>
								<h3>Place students</h3>
							</CardTitle>
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
							clash: "shares a period with another of the student’s (yes or no)",
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
							clash: "shares a period with another of the student’s (boolean)",
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
										clash={clashes.has(k)}
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
					copyAddresses={(studentIDs): void => {
						void copyStudentAddresses(
							studentIDs,
							(err, fallback): void => {
								data.report(err, fallback)
							},
						)
					}}
					upload={(drop, accept, later): void => {
						commitDraft(draft, drop, accept, later)
					}}
					hold={(): void => {
						holdDraft(draft)
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
	clash,
	label,
	ontoggle,
	ondelete,
}: {
	enrollment: Enrollment
	rowKey: string
	selected: boolean
	busy: boolean
	periods: string
	clash: boolean
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
				{clash && (
					<Badge variant="destructive" className="ml-2">
						Clash
					</Badge>
				)}
			</TableCell>
			<TableCell className="whitespace-nowrap">
				{policyLabel(enrollment)}
			</TableCell>
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
		a.clash === b.clash &&
		a.label === b.label,
)
