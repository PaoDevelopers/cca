// Per-student derivations for the students table.
//
// Requirement satisfaction is deliberately not among them: it is a
// rule, the rules live in the database, and the server reports it
// through /admin/api/students/status. What is left here is joining
// and presentation.

import type { StudentStatus, AdminStudent } from "@common/adminApi"
import type { Course, Enrollment } from "@common/types"

// Enrollments naming a course that is no longer listed are skipped
// rather than counted blind.
export function coursesByStudent(
	courses: Course[],
	enrollments: Enrollment[],
): Map<string, Course[]> {
	const byID = new Map(courses.map((c): [string, Course] => [c.id, c]))
	const out = new Map<string, Course[]>()
	for (const e of enrollments) {
		const course = byID.get(e.course_id)
		if (course === undefined) {
			continue
		}
		const list = out.get(e.student_id)
		if (list === undefined) {
			out.set(e.student_id, [course])
		} else {
			list.push(course)
		}
	}
	return out
}

export function statusByStudent(
	status: StudentStatus[],
): Map<string, StudentStatus> {
	return new Map(
		status.map((s): [string, StudentStatus] => [s.student_id, s]),
	)
}

// Carries what the student is enrolled in and where they stand, so an
// expression can ask about courses and standing and not only the
// student row.
export function studentCelContext(
	student: AdminStudent,
	enrolled: Course[],
	status: StudentStatus | undefined,
): Record<string, unknown> {
	return {
		id: student.id,
		name: student.name,
		grade: student.grade_id,
		legal_sex: student.legal_sex,
		requirements_ok: status?.requirements_met ?? true,
		budgeted_periods: status?.budgeted_periods_used ?? 0,
		max_budgeted_periods: status?.max_budgeted_periods ?? null,
		course_ids: enrolled.map((c): string => c.id),
		categories: [...new Set(enrolled.map((c): string => c.category_id))],
		period_count: enrolled.reduce(
			(total, c): number => total + c.period_ids.length,
			0,
		),
		enrollment_count: enrolled.length,
	}
}
