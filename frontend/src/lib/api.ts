import type { Category, Choice, Course, Period, Student } from "../types"

type HTTPMethod = "PUT" | "DELETE"

const jsonHeaders: HeadersInit = {
	"Content-Type": "application/json",
}

function asArray<T>(value: T[] | null | undefined): T[] {
	return Array.isArray(value) ? value : []
}

async function getJSON<T>(url: string, init?: RequestInit): Promise<T> {
	const response = await fetch(url, {
		...init,
		redirect: "manual",
	})
	handleRedirect(response)
	if (!response.ok) {
		const message = await response.text()
		throw new Error(message || response.statusText)
	}
	return (await response.json()) as T
}

function handleRedirect(response: Response): void {
	if (
		response.type === "opaqueredirect" ||
		(response.status >= 300 && response.status < 400)
	) {
		if (typeof window !== "undefined") {
			window.location.assign("/")
		}
		throw new Error("Redirecting to /")
	}
}

export async function fetchUser(): Promise<Student> {
	const user = await getJSON<Student>("/student/api/user_info")
	return user
}

export async function fetchCourses(): Promise<Course[]> {
	const data = await getJSON<Course[] | null>("/student/api/courses")
	const list = asArray(data)
	return list
}

export async function fetchPeriods(): Promise<Period[]> {
	const data = await getJSON<Period[] | null>("/student/api/periods")
	const list = asArray(data)
	const normalized = list.map((entry) =>
		typeof entry === "string" ? { id: entry } : entry,
	)
	return normalized
}

export async function fetchCategories(): Promise<Category[]> {
	const data = await getJSON<Category[] | null>("/student/api/categories")
	const list = asArray(data)
	const normalized = list.map((entry) =>
		typeof entry === "string" ? { id: entry } : entry,
	)
	return normalized
}

export async function fetchSelections(): Promise<Choice[]> {
	const data = await getJSON<Choice[] | null>("/student/api/my_selections")
	const list = asArray(data)
	return list
}

export async function mutateSelection(
	method: HTTPMethod,
	courseId: string,
): Promise<Choice[]> {
	const data = await getJSON<Choice[] | null>("/student/api/my_selections", {
		method,
		headers: jsonHeaders,
		body: JSON.stringify(courseId),
	})
	const list = asArray(data)
	return list
}
