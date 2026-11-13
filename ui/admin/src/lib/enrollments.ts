// Per-enrollment derivations for the enrollments table.

import { capacityLabel, isFull } from "@common/capacity"
import type { Course, Enrollment } from "@common/types"

// The pair is the table's primary key; used for selection and keyed
// iteration.
export function enrollmentKey(e: Enrollment): string {
	return `${e.student_id}:${e.course_id}`
}

// "Name (n/max)" for a course, for the places admins pick courses.
export function courseLabel(
	courseByID: Map<string, Course>,
	id: string,
	name: string,
): string {
	const course = courseByID.get(id)
	return course === undefined
		? name
		: `${name} (${course.current_students}/${capacityLabel(
				course.max_students,
			)})`
}

// The two policy bits, as one short label. They are independent and
// all four combinations are legal, so this names each combination
// rather than pretending there are three kinds of enrollment.
export function policyLabel(e: Enrollment): string {
	if (e.student_droppable) {
		return e.counts_toward_budget ? "Own pick" : "Invitation"
	}
	return e.counts_toward_budget ? "Committed" : "Placed"
}

// The keys of every enrollment that shares a period with another of
// the same student's: the same pairs the database's clash rule names.
// Only an administrator can make one, by accepting it, so this is the
// list of clashes that were let through, live from what is stored.
export function clashingKeys(
	courseByID: Map<string, Course>,
	enrollments: Enrollment[],
): Set<string> {
	const bySlot = new Map<string, string[]>()
	for (const e of enrollments) {
		for (const period of courseByID.get(e.course_id)?.period_ids ?? []) {
			const slot = `${e.student_id}:${period}`
			const keys = bySlot.get(slot)
			if (keys === undefined) {
				bySlot.set(slot, [enrollmentKey(e)])
			} else {
				keys.push(enrollmentKey(e))
			}
		}
	}
	const out = new Set<string>()
	for (const keys of bySlot.values()) {
		if (keys.length > 1) {
			for (const k of keys) {
				out.add(k)
			}
		}
	}
	return out
}

// The enrolled course is nested, so expressions can reach course
// attributes the simple syntax cannot. A course that is no longer
// listed yields empty values rather than breaking the expression.
export function enrollmentCelContext(
	e: Enrollment,
	course: Course | undefined,
	clash: boolean,
): Record<string, unknown> {
	return {
		student: {
			id: e.student_id,
			name: e.student_name,
			grade: e.grade_id,
		},
		course: {
			id: e.course_id,
			name: e.course_name,
			category_id: course?.category_id ?? "",
			term: course?.term ?? "",
			teacher: course?.teacher ?? "",
			location: course?.location ?? "",
			cost: course?.cost ?? "",
			invite_only: course?.invite_only ?? false,
			periods: course?.period_ids ?? [],
			// null where the course has no cap; see courseCelContext
			// for why `full` carries the comparison instead.
			max_students: course?.max_students ?? null,
			full: course !== undefined && isFull(course),
			current_students: course?.current_students ?? 0,
		},
		droppable: e.student_droppable,
		budgeted: e.counts_toward_budget,
		clash,
	}
}
