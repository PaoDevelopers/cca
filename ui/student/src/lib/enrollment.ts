// The derivations behind the student view.
//
// What is *not* here matters as much as what is: whether a course may
// be taken is not computed in the browser. Clash, capacity, budget,
// grade and legal-sex are rules, they are defined in the database, and
// the server answers them for the whole catalogue in one call
// (/student/api/eligibility). A second implementation in TypeScript
// would drift from the first, and the drift is noticed when it hides a
// rejection rather than when it is introduced.
//
// What is left here is arrangement: reading the server's verdicts and
// filtering the list.

import { isFull } from "@common/capacity"
import type {
	Course,
	Eligibility,
	Enrollment,
	EntityID,
	Violation,
} from "@common/types"

// The server's verdict for one course: why the student may not take
// it, or an empty list if they may.
export function violationsFor(
	eligibility: Eligibility,
	course: Course,
): Violation[] {
	return eligibility[course.id] ?? []
}

// The courses a swap into this one would have to drop, named by the
// server's clash violations rather than recomputed from the periods.
export function conflictingEnrollments(
	eligibility: Eligibility,
	course: Course,
): string[] {
	return [
		...new Set(
			violationsFor(eligibility, course)
				.filter((v): boolean => v.rule === "clash")
				.map((v): string => v.other_course_id ?? "")
				.filter((id): boolean => id !== ""),
		),
	]
}

// Whether to offer Swap: whether dropping what this course clashes
// with could plausibly clear the way.
//
// "Plausibly" is the whole of it. The server decides — self_swap
// judges the new course with the named ones disregarded, and refuses
// if it still does not fit. What this decides is only whether to put
// the button on the screen, and it can be wrong in two directions:
//
//   Offering a swap that cannot succeed wastes the student's click and
//   answers with a rejection.
//   Withholding one that would have succeeded is worse — the student
//   is told, silently, that a course is out of reach when it is not,
//   and has no way to discover otherwise.
//
// So this refuses only where the swap is impossible whatever gets
// dropped, and otherwise offers it and lets the server answer:
//
//   clash    cleared by dropping the other course. The point.
//   budget   the cap counts periods the student holds, and the swap
//            gives some back, so it may well fit afterwards. Whether
//            it does is arithmetic over the rules, and doing that
//            arithmetic here is exactly the second implementation
//            this file exists to avoid.
//   others   legal_sex, grade and capacity are facts about the course
//            and the student, unchanged by what else they hold.
//
// Two gates are checked because they are not violations at all, do not
// appear in the eligibility map, and refuse the whole operation:
// an invite-only course (YKG02), and a clashing enrollment the student
// may not drop (YKG03).
export function swappable(
	eligibility: Eligibility,
	course: Course,
	held: Enrollment[],
): boolean {
	const violations = violationsFor(eligibility, course)
	if (violations.length === 0) {
		return false
	}

	// A gate, not a negotiable rule: no rearrangement opens it.
	if (course.invite_only) {
		return false
	}

	const clashes = conflictingEnrollments(eligibility, course)
	if (clashes.length === 0) {
		return false
	}

	if (
		!violations.every(
			(v): boolean => v.rule === "clash" || v.rule === "budget",
		)
	) {
		return false
	}

	// Every course the swap would have to drop must be one the student
	// is allowed to drop. An administrator's fixed placement is not.
	const droppable = new Map<EntityID, boolean>(
		held.map((e): [EntityID, boolean] => [
			e.course_id,
			e.student_droppable,
		]),
	)
	return clashes.every((id): boolean => droppable.get(id) === true)
}

// What one course currently is to this student, in the four terms the
// card and the action buttons both need.
//
// Shared rather than worked out twice: the Vue layout puts the buttons
// in the card header and the reasons at its foot, so two components
// need the same verdict, and two derivations of "barred" that disagree
// would show a button the server refuses.
interface EnrollmentState {
	selected: boolean
	// An enrollment the student may not drop: an administrator placed
	// them and did not leave the door open.
	fixed: boolean
	barred: boolean
	reasons: string[]
	// The word for what they hold, empty when they hold nothing.
	status: string
}

function enrollmentState(
	course: Course,
	enrollment: Enrollment | null,
	violations: Violation[],
	canSwap: boolean,
): EnrollmentState {
	const selected = enrollment !== null
	const fixed = enrollment !== null && !enrollment.student_droppable

	// Invite-only is a gate rather than a negotiable rule, so it does
	// not appear among the violations and is read from the course.
	const barred = course.invite_only || (violations.length > 0 && !canSwap)

	const reasons = violations.map((v): string => v.detail)
	if (course.invite_only) {
		reasons.push("invitation required")
	}

	let status = ""
	if (fixed) {
		status = "Placed by an administrator"
	} else if (enrollment !== null && !enrollment.counts_toward_budget) {
		status = "Invited"
	} else if (selected) {
		status = "Selected"
	}

	return { selected, fixed, barred, reasons, status }
}

// One course as every component below needs it: the course itself and
// the verdict derived from what the student holds and what the server said.
//
// Built once per render and passed down whole. The card, the table row
// and the action buttons all used to take these as four separate props
// and recompute enrollmentState independently, which meant a change to
// what "barred" means had to be made in more than one place to take
// effect.
export interface CourseRow {
	course: Course
	canSwap: boolean
	state: EnrollmentState
}

// The handlers and the two flags that every course in a list shares.
// One object rather than five props threaded through three components.
export interface CourseActions {
	// Whether the student's own enrollment window is open. Closed, they
	// may look but not act.
	windowOpen: boolean
	// Whether any write is in flight; all of them disable while one is.
	updating: boolean
	onenroll: (course: Course) => void
	ondrop: (course: Course) => void
	onswap?: ((course: Course) => void) | undefined
}

export function courseRows(
	courses: Course[],
	enrollmentOf: (course: Course) => Enrollment | null,
	violationsOf: (course: Course) => Violation[],
	canSwapOf: (course: Course) => boolean,
): CourseRow[] {
	return courses.map((course): CourseRow => {
		const enrollment = enrollmentOf(course)
		const violations = violationsOf(course)
		const canSwap = canSwapOf(course)
		return {
			course,
			canSwap,
			state: enrollmentState(course, enrollment, violations, canSwap),
		}
	})
}

export interface CourseFilter {
	search: string
	categories: string[]
	periods: string[]
	hideFull: boolean
	hideInviteOnly: boolean
	hideIncompatible: boolean
	hideConflicting: boolean
}

export function filterCourses(
	courses: Course[],
	filter: CourseFilter,
	violations: (course: Course) => Violation[],
	selected: (course: Course) => boolean,
): Course[] {
	const query = filter.search.trim().toLowerCase()
	return courses.filter((c): boolean => {
		if (
			query !== "" &&
			!(
				c.name.toLowerCase().includes(query) ||
				c.id.toLowerCase().includes(query) ||
				c.description.toLowerCase().includes(query) ||
				c.teacher.toLowerCase().includes(query) ||
				c.location.toLowerCase().includes(query)
			)
		) {
			return false
		}
		if (
			filter.categories.length > 0 &&
			!filter.categories.includes(c.category_id)
		) {
			return false
		}
		if (
			filter.periods.length > 0 &&
			!c.period_ids.some((p): boolean => filter.periods.includes(p))
		) {
			return false
		}
		// These four switches describe whether a course is available to
		// take. They must not make a course the student already holds
		// disappear from the combined catalogue.
		if (selected(c)) {
			return true
		}
		// Fullness is the one verdict the client may compute: it is
		// arithmetic over a count the browser already has from the
		// realtime stream, not a restatement of a rule, and it is the
		// one that changes at enrollment speed.
		if (filter.hideFull && isFull(c)) {
			return false
		}
		if (filter.hideInviteOnly && c.invite_only) {
			return false
		}
		const courseViolations = violations(c)
		if (
			filter.hideIncompatible &&
			courseViolations.some(
				(v): boolean => v.rule === "legal_sex" || v.rule === "grade",
			)
		) {
			return false
		}
		if (
			filter.hideConflicting &&
			courseViolations.some((v): boolean => v.rule === "clash")
		) {
			return false
		}
		return true
	})
}
