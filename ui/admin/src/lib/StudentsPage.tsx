import {
	deleteStudent,
	startStudentSession,
	upsertStudents,
	type AdminStudent,
} from "@common/adminApi"
import { filterByCel, type FilterMode } from "@common/cel"
import { Button } from "@common/components/ui/button"
import { DropdownMenuItem } from "@common/components/ui/dropdown-menu"
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
import type { LegalSex } from "@common/types"
import { useEffect, useState, type ReactElement, type ReactNode } from "react"
import { DataTools } from "./DataTools"
import { useAdminData } from "./data"
import { FilterBar } from "./FilterBar"
import { StudentEditor } from "./StudentEditor"
import { StudentRow } from "./StudentRow"
import {
	coursesByStudent,
	statusByStudent,
	studentCelContext,
} from "./students"

interface Props {
	// The route's ?q=, owned by the shell so the box and the URL stay
	// one value.
	filter: string
	setFilter: (filter: string) => void
}

const sexes: LegalSex[] = ["F", "M", "X"]

// A student's address is their id under the student domain. There is no
// email column to read: 0002_types.sql defines the key as "the
// localpart of the school email address", and the auth boundary is
// where the two halves were last together.
//
// That same file allows a staff localpart as a student key, for a
// teacher enrolled to test with. Those really live under ykpaoschool.cn
// and nothing stored here tells them apart, so a roster holding one
// yields one wrong address — visible in the To: field, where it can be
// fixed.
const studentDomain = "stu.ykpaoschool.cn"

type SortKey = "id" | "name" | "grade" | "sex" | "budget" | "requirements"
type Sort = { key: SortKey; desc: boolean } | null

// Ascending, then descending, then back to the server's order.
function nextSort(sort: Sort, key: SortKey): Sort {
	if (sort?.key !== key) {
		return { key, desc: false }
	}
	return sort.desc ? null : { key, desc: true }
}

function ariaSort(
	sort: Sort,
	key: SortKey,
): "ascending" | "descending" | "none" {
	if (sort?.key !== key) {
		return "none"
	}
	return sort.desc ? "descending" : "ascending"
}

const sortGlyph = { ascending: "↑", descending: "↓", none: "↕" } as const

const columns: Array<{ key: SortKey; label: string }> = [
	{ key: "id", label: "ID" },
	{ key: "name", label: "Name" },
	{ key: "grade", label: "Grade" },
	{ key: "sex", label: "Legal sex" },
	{ key: "budget", label: "Budget" },
	{ key: "requirements", label: "Requirements" },
]

export function StudentsPage({ filter, setFilter }: Props): ReactElement {
	const data = useAdminData()

	useEffect((): void => {
		data.want("students", "grades", "courses", "periods", "enrollments")
	}, [data])

	// The student whose weekly schedule is expanded under their row.
	const [scheduleFor, setScheduleFor] = useState<string | null>(null)
	const [mode, setMode] = useState<FilterMode>("simple")
	// The student in the Sheet: "new" to add one, an id to edit, null
	// when it is closed.
	const [editing, setEditing] = useState<string | null>(null)
	// null keeps the server's order.
	const [sort, setSort] = useState<Sort>(null)

	if (
		!data.ready("students", "grades", "courses", "periods", "enrollments")
	) {
		return (
			<Section>
				<p className="text-muted-foreground">Loading…</p>
			</Section>
		)
	}

	const gradeIDs = data.grades.map((g): string => g.id)

	const run = (action: () => Promise<void>): Promise<boolean> =>
		data.run(action, "students")

	// Changing a student's grade or legal sex can invalidate the
	// placements they already hold, so an edit goes through the accept
	// protocol. Changing only their name cannot, and the server re-judges
	// nothing in that case — which is why re-importing an unchanged
	// roster never resurfaces an accepted placement.
	const runAccepting = (
		action: (accept: string[]) => Promise<void>,
	): Promise<boolean> => data.runAccepting(action, "students", "enrollments")

	// Sign in as a student and look at their page.
	//
	// The tab is opened on the click itself, before anything is awaited:
	// a window.open that happens after a fetch resolves is a pop-up as
	// far as the browser is concerned, and is blocked. So it opens blank,
	// and is pointed at the student area once the cookie is actually set
	// — or closed again if minting was refused, rather than left sitting
	// on a blank page.
	//
	// Only the student cookie is written, so this tab keeps its
	// administrator session and nothing here has to be signed into again
	// afterwards.
	const impersonate = (id: string): void => {
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

	const coursesOf = coursesByStudent(data.courses, data.enrollments)
	const statusOf = statusByStudent(data.studentStatus)

	const requirementsText = (id: string): string =>
		(statusOf.get(id)?.requirements_met ?? true) ? "OK" : "Unsatisfied"

	// "2/4 periods", or just the count when the grade has no cap.
	const budgetText = (id: string): string => {
		const status = statusOf.get(id)
		if (status === undefined) {
			return ""
		}
		return status.max_budgeted_periods === null
			? String(status.budgeted_periods_used)
			: `${String(status.budgeted_periods_used)}/${String(status.max_budgeted_periods)}`
	}

	const celResult =
		mode === "cel"
			? filterByCel(filter, data.students, (s) =>
					studentCelContext(
						s,
						coursesOf.get(s.id) ?? [],
						statusOf.get(s.id),
					),
				)
			: { rows: data.students, error: null, unfiltered: false }

	const filtered =
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
				)

	const sortValue = (s: AdminStudent, key: SortKey): string | number => {
		switch (key) {
			case "id":
				return s.id
			case "name":
				return s.name
			case "grade":
				return s.grade_id
			case "sex":
				return s.legal_sex
			case "budget":
				return statusOf.get(s.id)?.budgeted_periods_used ?? 0
			case "requirements":
				return requirementsText(s.id)
		}
	}

	const shown =
		sort === null
			? filtered
			: [...filtered].sort((a, b): number => {
					const x = sortValue(a, sort.key)
					const y = sortValue(b, sort.key)
					const order =
						typeof x === "number" && typeof y === "number"
							? x - y
							: String(x).localeCompare(String(y), undefined, {
									numeric: true,
								})
					return sort.desc ? -order : order
				})

	// The listed students, not every student: an administrator who has
	// filtered to one grade means that grade.
	const addresses = filtered.map((s): string => `${s.id}@${studentDomain}`)

	const copyAddresses = async (): Promise<void> => {
		try {
			// Semicolons: Outlook splits recipients on them. It splits on
			// commas only if that setting has been turned on, and it is
			// off by default — a comma-separated paste then lands as one
			// long unresolvable recipient instead of a class.
			await navigator.clipboard.writeText(addresses.join("; "))
			toast(`Copied ${String(addresses.length)}.`)
		} catch (err) {
			// Writing to the clipboard needs a secure context, which a
			// plain-HTTP deployment is not. Better said out loud than left
			// as a paste of whatever was in there before.
			data.report(err, "Could not copy to the clipboard")
		}
	}

	const editingStudent =
		editing === null || editing === "new"
			? null
			: (data.students.find((s): boolean => s.id === editing) ?? null)

	return (
		<Section
			actions={
				<>
					<Button
						size="sm"
						onClick={(): void => {
							setEditing("new")
						}}
					>
						Add student
					</Button>
					<DataTools
						section="students"
						exportHref="/admin/api/students/export"
						importAction="/admin/api/students/import"
						busy={data.busy}
						run={run}
						extra={
							<DropdownMenuItem
								disabled={addresses.length === 0}
								onClick={(): void => {
									void copyAddresses()
								}}
							>
								Copy students’ email addresses
							</DropdownMenuItem>
						}
					/>
				</>
			}
		>
			<FilterBar
				query={filter}
				onQuery={setFilter}
				mode={mode}
				onMode={setMode}
				error={mode === "cel" ? celResult.error : null}
				summary={
					filtered.length === data.students.length
						? `${String(data.students.length)} students`
						: `${String(filtered.length)} of ${String(data.students.length)} students shown`
				}
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

			<Table>
				<TableHeader>
					<TableRow className="hover:bg-transparent">
						{columns.map(({ key, label }) => (
							<TableHead
								key={key}
								scope="col"
								aria-sort={ariaSort(sort, key)}
							>
								<button
									type="button"
									className="inline-flex items-center gap-1 hover:text-foreground"
									onClick={(): void => {
										setSort(nextSort(sort, key))
									}}
								>
									{label}
									<span
										aria-hidden="true"
										className="text-xs"
									>
										{sortGlyph[ariaSort(sort, key)]}
									</span>
								</button>
							</TableHead>
						))}
						<TableHead scope="col">Actions</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{shown.length === 0 ? (
						<TableRow>
							<TableCell
								colSpan={7}
								className="text-muted-foreground"
							>
								{data.students.length === 0
									? "No students yet."
									: "No students match your filter."}
							</TableCell>
						</TableRow>
					) : (
						shown.map((student) => (
							<StudentRow
								key={student.id}
								student={student}
								busy={data.busy}
								courses={data.courses}
								enrollments={data.enrollments}
								periods={data.periodIDs}
								budget={budgetText(student.id)}
								requirements={requirementsText(student.id)}
								scheduleOpen={scheduleFor === student.id}
								onedit={(): void => {
									setEditing(student.id)
								}}
								ondelete={(): void => {
									void run(() =>
										deleteStudent(student.id),
									).then((ok): void => {
										if (ok) {
											toast(`Deleted ${student.id}.`)
										}
									})
								}}
								onimpersonate={(): void => {
									impersonate(student.id)
								}}
								ontoggleschedule={(): void => {
									setScheduleFor(
										scheduleFor === student.id
											? null
											: student.id,
									)
								}}
							/>
						))
					)}
				</TableBody>
			</Table>

			{editing !== null &&
				(editing === "new" || editingStudent !== null) && (
					<StudentEditor
						// A fresh editor, and a fresh draft, per student.
						key={editing}
						student={editingStudent}
						gradeIDs={gradeIDs}
						sexes={sexes}
						budget={
							editingStudent === null
								? ""
								: budgetText(editingStudent.id)
						}
						requirements={
							editingStudent === null
								? ""
								: requirementsText(editingStudent.id)
						}
						busy={data.busy}
						onclose={(): void => {
							setEditing(null)
						}}
						onsave={(draft): void => {
							// The same upsert the import uses: adding a student
							// and correcting one are the same intention.
							void runAccepting(async (accept): Promise<void> => {
								await upsertStudents([draft], accept)
								setEditing(null)
								toast(
									editing === "new"
										? `Added ${draft.id}.`
										: `Saved ${draft.id}.`,
								)
							})
						}}
					/>
				)}
		</Section>
	)
}

// The heading is also where the data layer can put the focus when the
// control that started a write no longer exists, hence tabIndex.
function Section({
	actions,
	children,
}: {
	actions?: ReactElement
	children: ReactNode
}): ReactElement {
	return (
		<section
			aria-labelledby="students-heading"
			className="flex flex-col gap-4"
		>
			<div className="flex flex-wrap items-center justify-between gap-2">
				<h2
					id="students-heading"
					tabIndex={-1}
					className="text-xl font-semibold tracking-tight outline-none"
				>
					Students
				</h2>
				{actions !== undefined && (
					<div className="flex flex-wrap items-center gap-2">
						{actions}
					</div>
				)}
			</div>
			{children}
		</section>
	)
}
