export type LegalSex = "F" | "M" | "X"

// An entity ID is the technical key of a category, grade, period or
// course: uppercase, no spaces, chosen by administrators and spoken by
// every machine interface. The database enforces the grammar; nothing
// here restates it.
export type EntityID = string

// A student ID is the localpart of their school email, "s22537". The
// same string the roster carries and the sign-in produces.
export type StudentID = string

export type PeriodID = EntityID

// How long a course runs. Free text: the software never acts on its
// value, but these are what the master spreadsheet uses, so they are
// offered as suggestions rather than enforced as a type.
export const courseTermSuggestions = ["Season", "Semester", "Year"]

export interface Category {
	id: EntityID
	name: string
}

export interface Period {
	id: PeriodID
	name: string
	sort_order: number
}

export interface Requirement {
	id: number
	min_period_count: number
	category_ids: EntityID[]
}

export interface Grade {
	id: EntityID
	name: string

	// The enrollment window. Null means "no such bound": no opens_at
	// is a closed window, no closes_at is one that stays open until
	// somebody closes it. RFC 3339 strings.
	opens_at: string | null
	closes_at: string | null

	// Derived server-side from the two bounds, and the only thing to
	// branch on. Never recompute it here: display and enforcement must
	// read one definition, or a student is shown a window the server
	// will refuse.
	is_open: boolean

	// Null means no cap.
	max_budgeted_periods: number | null
	min_distinct_categories: number
	sort_order: number

	requirements: Requirement[]
}

export interface Course {
	id: EntityID
	name: string
	description: string
	category_id: EntityID
	teacher: string
	teacher_email: string
	location: string
	term: string
	cost: string
	// null means no cap: the course takes everyone. Distinct from 0,
	// which is a cap that admits nobody. See @common/capacity.
	max_students: number | null
	invite_only: boolean
	current_students: number
	period_ids: PeriodID[]

	// Empty means unrestricted on that axis.
	allowed_legal_sexes: LegalSex[]
	allowed_grade_ids: EntityID[]
}

// An enrollment's two policy bits are independent, and all four
// combinations mean something:
//
//	droppable + budgeted      the student's own pick
//	droppable + unbudgeted    an invitation they may decline
//	fixed     + unbudgeted    a placement they may not leave
//	fixed     + budgeted      a committed pick, charged and locked in
export interface Enrollment {
	student_id: StudentID
	student_name: string
	grade_id: EntityID
	course_id: EntityID
	course_name: string
	student_droppable: boolean
	counts_toward_budget: boolean
}

// One negotiable rule broken by a prospective enrollment. `code` names
// the violated fact absolutely and is what an administrator sends back
// to accept it; `detail` is the prose to show.
export interface Violation {
	student_id: StudentID | null
	// The five rules a prospective enrollment is judged against, plus
	// "overfull" — which is not one of them. The five are questions
	// about one student and one course; "overfull" is a fact about a
	// course alone, raised when an administrator shrinks its capacity
	// below what it already holds, and carries no student_id.
	rule: "legal_sex" | "grade" | "capacity" | "clash" | "budget" | "overfull"
	code: string
	other_course_id: EntityID | null
	period_id: PeriodID | null
	detail: string
}

// Why a student may not take each course, for the courses they do not
// already hold. Courses they do hold are absent too, so absence means
// "nothing to say here", not "free to take" — read it after
// partitioning on what the student holds, which is what App.svelte
// does.
export type Eligibility = Record<EntityID, Violation[]>

// A batch element the server could not read at all. Not a violation:
// nothing about it is negotiable.
export interface MalformedElement {
	// Which element of the batch, counting from 1. It comes from
	// PostgreSQL's WITH ORDINALITY, which is 1-based, so element 1 is
	// the first data row — the second line of the spreadsheet, since
	// the first is the header.
	index: number
	id: string
	sqlstate: string
	// The import column the value came from, spelt as the file's own
	// header spells it, or "" where the server could not attribute the
	// failure to one column. The database cannot supply this by
	// itself: a domain rejection is raised while casting a value and
	// names the domain, never the column it was bound for.
	field: string
	message: string
}

export interface StudentRequirement {
	id: number
	min_period_count: number
	satisfied_periods: number
	met: boolean
}

// What a student is and where they stand. Everything but the cap is
// advisory; the cap itself is enforced by the server, not here.
export interface StudentInfo {
	id: StudentID
	name: string
	grade_id: EntityID
	budgeted_periods_used: number
	max_budgeted_periods: number | null
	distinct_categories_used: number
	min_distinct_categories: number
	requirements: StudentRequirement[]
}
