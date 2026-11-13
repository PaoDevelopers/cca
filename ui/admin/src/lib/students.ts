// Per-student derivations for the students table.
//
// Requirement satisfaction is deliberately not among them: it is a
// rule, the rules live in the database, and the server reports it
// through /admin/api/students/status. What is left here is joining
// and presentation.

import type { StudentStatus, AdminStudent } from "@common/adminApi"
import { toast } from "@common/components/ui/toast"
import type { Course, Enrollment } from "@common/types"

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

export function studentAddress(id: string): string {
	return `${id}@${studentDomain}`
}

// Puts the students' addresses on the clipboard, once each, for pasting
// into a mail's recipients.
export async function copyStudentAddresses(
	studentIDs: string[],
	report: (err: unknown, fallback: string) => void,
): Promise<void> {
	const addresses = [...new Set(studentIDs)].map(studentAddress)
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
		report(err, "Could not copy to the clipboard")
	}
}

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
