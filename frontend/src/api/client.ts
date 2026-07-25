import type { APIErrorEnvelope } from "@/types"

import { APIRequestError } from "./types"

export async function apiRequest<T>(
	path: string,
	init: RequestInit = {},
): Promise<T> {
	const headers = new Headers(init.headers)
	if (init.body !== undefined && !(init.body instanceof FormData)) {
		headers.set("Content-Type", "application/json")
	}
	const response = await fetch(path, {
		...init,
		headers,
		credentials: "same-origin",
		redirect: "manual",
	})
	if (response.status === 401) {
		window.location.assign("/")
		throw new APIRequestError(
			401,
			"unauthenticated",
			"Please sign in again.",
		)
	}
	if (!response.ok) {
		let code = "request_failed"
		let message = `Request failed with status ${response.status}.`
		try {
			const payload = (await response.json()) as APIErrorEnvelope
			if (typeof payload.error?.code === "string")
				code = payload.error.code
			if (typeof payload.error?.message === "string") {
				message = payload.error.message
			}
		} catch {
			// Keep the generic message when the response is not JSON.
		}
		throw new APIRequestError(response.status, code, message)
	}
	if (response.status === 204) return undefined as T
	return (await response.json()) as T
}

export function jsonBody(value: unknown): string {
	return JSON.stringify(value)
}
