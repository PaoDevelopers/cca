import { apiRequest } from "./client"

export function adminRequest<T>(
	path: string,
	init: RequestInit = {},
): Promise<T> {
	return apiRequest<T>(`/api/v1/admin${path}`, init)
}
