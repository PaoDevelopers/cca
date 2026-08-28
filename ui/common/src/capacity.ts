import type { Course } from "./types"

// A course's capacity, when it has one.
//
// max_students is null for a course that takes everyone. That is a
// real setting, not a missing one, and it is deliberately not 0: 0 is
// also a real setting and means the opposite — a course nobody may
// enroll in. Everything that reads a capacity goes through here, so
// the two never get conflated again in a comparison written in a
// hurry.

// What a capacity looks like beside a count: "12/20", or "12/∞" where
// there is no cap. The sign is the shortest thing that reads as "no
// limit" in a column of numbers.
export function capacityLabel(max: number | null): string {
	return max === null ? "∞" : String(max)
}

// The same, for a screen reader and for anywhere else the text is
// spoken rather than seen. "∞" is a symbol a reader may render as
// "infinity" or skip altogether, and neither is what the seat count
// means.
export function capacitySpoken(max: number | null): string {
	return max === null ? "no limit" : String(max)
}

// Whether one more student would not fit. An uncapped course is never
// full: the comparison it would otherwise make is against null, which
// in JavaScript coerces to 0 and would call every such course full
// from its first enrollee. That is the mistake this exists to prevent.
export function isFull(
	course: Pick<Course, "current_students" | "max_students">,
): boolean {
	return (
		course.max_students !== null &&
		course.current_students >= course.max_students
	)
}
