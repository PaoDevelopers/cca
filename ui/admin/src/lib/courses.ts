// Per-course derivations for the courses table.

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
		max_students: c.max_students,
		current_students: c.current_students,
		allowed_grades: c.allowed_grade_ids,
		allowed_legal_sexes: c.allowed_legal_sexes,
	}
}
