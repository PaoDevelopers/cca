import { asArray, getJSON, sendForm, sendJSON } from "./http"
import type {
	Category,
	Course,
	EntityID,
	Enrollment,
	Grade,
	LegalSex,
	Period,
	PeriodID,
	StudentID,
} from "./types"

// Writes that can surface negotiable violations take an `accept` list:
// the codes from a previous attempt's 409 that the administrator has
// confirmed. Everything else has no such parameter, because nothing
// else can break a rule — that is the test working, and it is why the
// override checkbox is gone.

// Categories.

export async function fetchCategories(): Promise<Category[]> {
	return asArray(await getJSON<Category[] | null>("/admin/api/categories"))
}

export function createCategory(id: EntityID, name: string): Promise<void> {
	return sendJSON("/admin/api/categories", "POST", { id, name })
}

export function renameCategory(id: EntityID, name: string): Promise<void> {
	return sendJSON(`/admin/api/categories/${encodeURIComponent(id)}`, "PUT", {
		name,
	})
}

export function deleteCategory(id: EntityID): Promise<void> {
	return sendJSON(`/admin/api/categories/${encodeURIComponent(id)}`, "DELETE")
}

// Periods.

export async function fetchPeriods(): Promise<Period[]> {
	return asArray(await getJSON<Period[] | null>("/admin/api/periods"))
}

export function createPeriod(id: PeriodID, name: string): Promise<void> {
	return sendJSON("/admin/api/periods", "POST", { id, name })
}

export function renamePeriod(id: PeriodID, name: string): Promise<void> {
	return sendJSON(`/admin/api/periods/${encodeURIComponent(id)}`, "PUT", {
		name,
	})
}

// Declarative: the request names the whole order, which is what the
// administrator is looking at. Idempotent, so a double-click is
// harmless, and there is no relative "move up" that reorders twice.
export function setPeriodOrder(ids: PeriodID[]): Promise<void> {
	return sendJSON("/admin/api/periods/order", "POST", { ids })
}

export function deletePeriod(id: PeriodID): Promise<void> {
	return sendJSON(`/admin/api/periods/${encodeURIComponent(id)}`, "DELETE")
}

// Grades.

export async function fetchGrades(): Promise<Grade[]> {
	return asArray(await getJSON<Grade[] | null>("/admin/api/grades"))
}

export function createGrade(
	id: EntityID,
	name: string,
	maxBudgetedPeriods: number | null,
	minDistinctCategories: number,
): Promise<void> {
	return sendJSON("/admin/api/grades", "POST", {
		id,
		name,
		max_budgeted_periods: maxBudgetedPeriods,
		min_distinct_categories: minDistinctCategories,
	})
}

export function updateGrade(
	id: EntityID,
	name: string,
	minDistinctCategories: number,
): Promise<void> {
	return sendJSON(`/admin/api/grades/${encodeURIComponent(id)}`, "PUT", {
		name,
		min_distinct_categories: minDistinctCategories,
	})
}

// Null bounds mean what their absence says: no opens_at is a closed
// window, no closes_at is one that stays open until someone closes it.
// Nothing fires at either bound; openness is derived wherever it is
// read.
// Sets one or both bounds of a grade's enrollment window.
//
// A bound that is left out is left alone, and null clears it — three
// states, not two, because "no such bound" is itself a value. Only the
// boxes the administrator actually edited are sent, so a card built
// before somebody else moved the other bound cannot carry the stale
// value it was built with back into the database.
export function setGradeWindow(
	id: EntityID,
	bounds: { opens_at?: string | null; closes_at?: string | null },
): Promise<void> {
	return sendJSON(
		`/admin/api/grades/${encodeURIComponent(id)}/window`,
		"PUT",
		bounds,
	)
}

// The two manual levers. Neither takes an instant, deliberately: the
// only clock that decides whether a window is open is the database's,
// and every reading taken here was a different clock's, rounded to the
// minute by the box it came out of. They send nothing and let the
// statement that writes the bound read the time.
//
// Opening leaves a scheduled closing time alone — starting early says
// nothing about when to stop — and is refused (409) if that closing
// time has already passed, since the row would then say the window
// shuts before it starts.
export function openWindowNow(id: EntityID): Promise<void> {
	return sendJSON(
		`/admin/api/grades/${encodeURIComponent(id)}/window/open`,
		"POST",
		{},
	)
}

// Leaves opens_at alone, so the card can still say when the window
// ran. A window that was not running is not an error to close.
export function closeWindowNow(id: EntityID): Promise<void> {
	return sendJSON(
		`/admin/api/grades/${encodeURIComponent(id)}/window/close`,
		"POST",
		{},
	)
}

// Shuts every window that is open right now, leaving the schedules
// alone: a grade that has not opened yet keeps its future opens_at.
//
// One call rather than a walk over the cards, because the walk is
// slowest exactly when it matters — the end of a season, with students
// still acting in the grades not yet reached.
export function closeAllWindows(): Promise<void> {
	return sendJSON("/admin/api/grades/close", "POST", {})
}

// The one grade field that is an input to a negotiable rule: lowering
// the cap can put students already enrolled over it.
export function setGradeBudget(
	id: EntityID,
	maxBudgetedPeriods: number | null,
	accept: string[] = [],
): Promise<void> {
	return sendJSON(
		`/admin/api/grades/${encodeURIComponent(id)}/budget`,
		"PUT",
		{ max_budgeted_periods: maxBudgetedPeriods, accept },
	)
}

export function setGradeOrder(ids: EntityID[]): Promise<void> {
	return sendJSON("/admin/api/grades/order", "POST", { ids })
}

// One form, one save: the whole requirement set is replaced, so the
// operation is idempotent and there is no create-then-delete dance.
export function setGradeRequirements(
	id: EntityID,
	requirements: Array<{ min_period_count: number; category_ids: EntityID[] }>,
): Promise<void> {
	return sendJSON(
		`/admin/api/grades/${encodeURIComponent(id)}/requirements`,
		"PUT",
		{ requirements },
	)
}

export function deleteGrade(id: EntityID): Promise<void> {
	return sendJSON(`/admin/api/grades/${encodeURIComponent(id)}`, "DELETE")
}

// Courses.

// A course is one form and one save button, so its attributes, its
// periods, its two restriction lists and its capacity all travel
// together and are judged together.
export interface CourseInput {
	id: EntityID
	name: string
	description: string
	category_id: EntityID
	teacher: string
	teacher_email: string
	location: string
	term: string
	cost: string
	// null asks for a course with no cap; see @common/capacity.
	max_students: number | null
	invite_only: boolean
	period_ids: PeriodID[]
	allowed_legal_sexes: LegalSex[]
	allowed_grade_ids: EntityID[]
	accept: string[]
}

export async function fetchAdminCourses(): Promise<Course[]> {
	return asArray(await getJSON<Course[] | null>("/admin/api/courses"))
}

// Creating cannot violate anything: a new course has no enrollees to
// re-judge.
export function createCourse(input: CourseInput): Promise<void> {
	return sendJSON("/admin/api/courses", "POST", input)
}

export function updateCourse(id: EntityID, input: CourseInput): Promise<void> {
	return sendJSON(
		`/admin/api/courses/${encodeURIComponent(id)}`,
		"PUT",
		input,
	)
}

// Its own operation, not an attribute edit: it cascades through
// enrollments and invalidates any outstanding accept code naming the
// course.
export function renameCourseID(id: EntityID, newID: EntityID): Promise<void> {
	return sendJSON(`/admin/api/courses/${encodeURIComponent(id)}/id`, "PUT", {
		id: newID,
	})
}

export function deleteCourse(id: EntityID): Promise<void> {
	return sendJSON(`/admin/api/courses/${encodeURIComponent(id)}`, "DELETE")
}

// Students.

export interface AdminStudent {
	id: StudentID
	name: string
	grade_id: EntityID
	legal_sex: LegalSex
}

export async function fetchStudents(): Promise<AdminStudent[]> {
	return asArray(await getJSON<AdminStudent[] | null>("/admin/api/students"))
}

// One call for the roster import and the single-student edit alike:
// both mean "make these students be so". Idempotent, so re-sending
// either changes nothing, and every malformed element comes back at
// once rather than one per attempt.
export function upsertStudents(
	students: AdminStudent[],
	accept: string[] = [],
): Promise<void> {
	return sendJSON("/admin/api/students", "PUT", { students, accept })
}

export function deleteStudent(id: StudentID): Promise<void> {
	return sendJSON(`/admin/api/students/${encodeURIComponent(id)}`, "DELETE")
}

// Signs this browser in as a student, for looking at what they are
// being shown. It writes the student cookie only, so the admin session
// in the tab that asked for it survives; the student area then opens
// in a tab of its own.
export function startStudentSession(id: StudentID): Promise<void> {
	return sendJSON(
		`/admin/api/students/${encodeURIComponent(id)}/session`,
		"POST",
	)
}

// Enrollments.

export async function fetchEnrollments(): Promise<Enrollment[]> {
	return asArray(await getJSON<Enrollment[] | null>("/admin/api/enrollments"))
}

// One course, many students: that is the unit the database locks and
// judges as a whole. The student order matters — they compete for the
// same seats, and earlier entries win when a course fills mid-batch —
// so it is never sorted or deduplicated on the way.
export function placeEnrollments(
	courseID: EntityID,
	studentIDs: StudentID[],
	studentDroppable: boolean,
	countsTowardBudget: boolean,
	accept: string[] = [],
): Promise<void> {
	return sendJSON("/admin/api/enrollments", "POST", {
		course_id: courseID,
		student_ids: studentIDs,
		student_droppable: studentDroppable,
		counts_toward_budget: countsTowardBudget,
		accept,
	})
}

// Change the policy of enrollments that already exist without giving
// up the seats. Remove-and-replace would do neither atomically.
export function setEnrollmentPolicy(
	courseID: EntityID,
	studentIDs: StudentID[],
	studentDroppable: boolean,
	countsTowardBudget: boolean,
	accept: string[] = [],
): Promise<void> {
	return sendJSON("/admin/api/enrollments/policy", "PUT", {
		course_id: courseID,
		student_ids: studentIDs,
		student_droppable: studentDroppable,
		counts_toward_budget: countsTowardBudget,
		accept,
	})
}

// Removal is monotone: it can only shrink the violation set, so there
// is nothing to accept. The droppability bit binds students, not
// administrators.
export function removeEnrollments(
	courseID: EntityID,
	studentIDs: StudentID[],
): Promise<void> {
	return sendJSON("/admin/api/enrollments", "DELETE", {
		course_id: courseID,
		student_ids: studentIDs,
	})
}

// Imports and rollover.

export function importSpreadsheet(url: string, file: File): Promise<void> {
	const form = new FormData()
	form.set("spreadsheet", file)
	return sendForm(url, form)
}

export function clearSection(section: string): Promise<void> {
	return sendJSON(`/admin/api/data/${encodeURIComponent(section)}`, "DELETE")
}

// Where every student stands: their budget, their category spread, and
// each requirement group with whether it is met. A read model, not a
// computation — the rule is the database's and is asked, not restated.
export interface StudentStatus {
	student_id: StudentID
	grade_id: EntityID
	budgeted_periods_used: number
	max_budgeted_periods: number | null
	distinct_categories_used: number
	min_distinct_categories: number
	requirements: Array<{
		id: number
		min_period_count: number
		satisfied_periods: number
		met: boolean
	}>
	requirements_met: boolean
}

export async function fetchStudentStatus(): Promise<StudentStatus[]> {
	return asArray(
		await getJSON<StudentStatus[] | null>("/admin/api/students/status"),
	)
}
