// Per-course derivations for the courses table.

import { isFull } from "@common/capacity"
import type { Course } from "@common/types"

// The whole record, so capacity and scheduling questions are
// expressible.
export function courseCelContext(c: Course): Record<string, unknown> {
	return {
		id: c.id,
		name: c.name,
		description: c.description,
		category_id: c.category_id,
		term: c.term,
		cost: c.cost,
		teacher: c.teacher,
		teacher_email: c.teacher_email,
		location: c.location,
		invite_only: c.invite_only,
		periods: c.period_ids,
		// null for a course with no cap, which is what it is. The
		// comparison an administrator would otherwise write against
		// it — current_students >= max_students — is offered as
		// `full` instead, because CEL has no arithmetic for null and
		// one uncapped course in the list would fail the whole
		// expression rather than simply not matching.
		max_students: c.max_students,
		full: isFull(c),
		current_students: c.current_students,
		allowed_grades: c.allowed_grade_ids,
		allowed_legal_sexes: c.allowed_legal_sexes,
	}
}
