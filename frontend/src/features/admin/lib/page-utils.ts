import { toast } from "sonner"

import type { Course, Selection, Student } from "@/types"

export function domID(prefix: string, value: string | number): string {
	return `${prefix}-${encodeURIComponent(String(value)).replaceAll("%", "")}`
}

export function getCourseSearchText(course: Course): string {
	return `${course.id} ${course.name} ${course.teacher} ${course.location} ${course.category_id}`
}

export function getStudentSearchText(student: Student): string {
	return `${student.id} ${student.name} ${student.grade}`
}

export function getSelectionSearchText(selection: Selection): string {
	return `${selection.student_id ?? ""} ${selection.student_name ?? ""} ${selection.course_id} ${selection.course_name ?? ""}`
}

export function incrementCount<Key extends string | number>(
	counts: Map<Key, number>,
	key: Key,
): void {
	counts.set(key, (counts.get(key) ?? 0) + 1)
}

export function countCoursesByCategory(
	courses: readonly Course[],
): ReadonlyMap<string, number> {
	const counts = new Map<string, number>()
	for (const course of courses) incrementCount(counts, course.category_id)
	return counts
}

export function countCoursesByPeriod(
	courses: readonly Course[],
): ReadonlyMap<string, number> {
	const counts = new Map<string, number>()
	for (const course of courses) {
		for (const periodID of course.period_ids) {
			incrementCount(counts, periodID)
		}
	}
	return counts
}

export function countSelectionsByStudent(
	selections: readonly Selection[],
): ReadonlyMap<number, number> {
	const counts = new Map<number, number>()
	for (const selection of selections) {
		if (selection.student_id !== undefined) {
			incrementCount(counts, selection.student_id)
		}
	}
	return counts
}

export async function runMutation(
	request: () => Promise<unknown>,
	refresh: () => Promise<void>,
	successMessage: string,
): Promise<boolean> {
	try {
		await request()
		await refresh()
		toast.success(successMessage)
		return true
	} catch (caught) {
		toast.error(
			caught instanceof Error ? caught.message : "The request failed.",
		)
		return false
	}
}
