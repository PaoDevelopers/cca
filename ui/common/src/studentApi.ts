import { asArray, getJSON } from "./http"
import type {
	Category,
	Course,
	Eligibility,
	Enrollment,
	Grade,
	Period,
	StudentInfo,
} from "./types"

const jsonHeaders: HeadersInit = {
	"Content-Type": "application/json",
}

export function fetchUser(): Promise<StudentInfo> {
	return getJSON<StudentInfo>("/student/api/user_info")
}

export async function fetchCourses(): Promise<Course[]> {
	return asArray(await getJSON<Course[] | null>("/student/api/courses"))
}

export async function fetchPeriods(): Promise<Period[]> {
	return asArray(await getJSON<Period[] | null>("/student/api/periods"))
}

export async function fetchCategories(): Promise<Category[]> {
	return asArray(await getJSON<Category[] | null>("/student/api/categories"))
}

export async function fetchGrades(): Promise<Grade[]> {
	return asArray(await getJSON<Grade[] | null>("/student/api/grades"))
}

// Why each course the student does not hold is closed to them, for
// every course, in one call. The rules live in the database and are
// asked, never restated here: a second implementation in TypeScript
// drifts from the first, and the drift is noticed when it hides a
// rejection rather than when it is introduced.
export async function fetchEligibility(): Promise<Eligibility> {
	const value = await getJSON<Eligibility | null>("/student/api/eligibility")
	return value ?? {}
}

export async function fetchEnrollments(): Promise<Enrollment[]> {
	return asArray(
		await getJSON<Enrollment[] | null>("/student/api/my_enrollments"),
	)
}

// The three student writes. Each answers with the resulting enrollment
// set, so the client never has to guess what its change did.

export function enroll(courseId: string): Promise<Enrollment[]> {
	return writeEnrollments("PUT", { course_id: courseId })
}

// Declining an invitation is dropping it: an invitation is an
// enrollment the student may drop, not a separate kind of thing.
export function drop(courseId: string): Promise<Enrollment[]> {
	return writeEnrollments("DELETE", { course_id: courseId })
}

// Atomic, and not sugar for drop-then-enroll: the new course is judged
// with the old ones disregarded, which is the only way to swap between
// two courses that clash. On any rejection nothing changes.
export function swap(
	courseId: string,
	replacing: string[],
): Promise<Enrollment[]> {
	return writeEnrollments("POST", { course_id: courseId, replacing })
}

async function writeEnrollments(
	method: "PUT" | "DELETE" | "POST",
	body: unknown,
): Promise<Enrollment[]> {
	return asArray(
		await getJSON<Enrollment[] | null>("/student/api/my_enrollments", {
			method,
			headers: jsonHeaders,
			body: JSON.stringify(body),
		}),
	)
}
