import { apiRequest } from "./client"

export function studentRequest<T>(
	path: string,
	init: RequestInit = {},
): Promise<T> {
	return apiRequest<T>(`/api/v1/student${path}`, init)
}
