import type { MalformedElement, Violation } from "./types"

// Every API error arrives in one envelope:
//
//	{"error": {"code": "conflict", "message": "..."}}
//
// The codes are the closed set in internal/web/api_error.go. Branch on
// those, not on the message text, which is prose and may change.
//
// Two of them carry a machine payload beside the message, because the
// client has to do something with it rather than only show it.
export class ApiError extends Error {
	public readonly code: string
	public readonly violations: Violation[]
	public readonly malformed: MalformedElement[]

	public constructor(
		code: string,
		message: string,
		violations: Violation[] = [],
		malformed: MalformedElement[] = [],
	) {
		super(message)
		this.code = code
		this.violations = violations
		this.malformed = malformed
	}
}

// A write refused because it would break negotiable rules. The
// administrator may confirm, and the retry names the codes they
// accepted. Students never see this: they accept nothing.
export function violationsOf(err: unknown): Violation[] {
	return err instanceof ApiError && err.code === "violations"
		? err.violations
		: []
}

export function isViolationError(err: unknown): boolean {
	return violationsOf(err).length > 0
}

export function malformedOf(err: unknown): MalformedElement[] {
	return err instanceof ApiError && err.code === "malformed"
		? err.malformed
		: []
}

// A 401. The app shows a sign-in message; it never navigates by itself.
export class AuthError extends ApiError {
	public constructor(message: string) {
		super("unauthenticated", message)
	}
}

// The prose to put in front of a person.
//
// A violation error's own message is a count — "3 unaccepted
// violation(s)" — because the server's message field is a summary and
// the substance travels in the payload, where an administrator's
// accept dialog reads it. A student has no accept dialog: the write is
// simply refused, and the count is all they were being told. The
// reasons were on the wire the whole time.
export function errorMessage(err: unknown, fallback: string): string {
	const details = violationsOf(err)
		.map((v): string => v.detail)
		.filter((d): boolean => d !== "")
	if (details.length > 0) {
		return details.join("; ")
	}

	return err instanceof Error ? err.message : fallback
}

// A malformed batch reports every bad element at once, so the message
// lists them rather than naming only the first.
//
// The row numbers are the ones in the administrator's spreadsheet, so
// they can go to the line and fix it. index is 1-based over the data
// rows and the header is line 1, so the line is index + 1. It used to
// be index + 2, which named the row below the broken one — and named
// it plausibly, since a spreadsheet has a row there too.
//
// The column is named alongside it, for the same reason. Without it a
// file that was blank in one column throughout came back as fifty
// identical sentences — "This must not be empty" — with nothing to say
// which of the fourteen columns was meant.
export function batchErrorMessage(err: unknown, fallback: string): string {
	const bad = malformedOf(err)
	if (bad.length === 0) {
		return errorMessage(err, fallback)
	}
	const lines = bad.map(
		(element) =>
			`row ${String(element.index + 1)}${
				element.id === "" ? "" : ` (${element.id})`
			}${
				element.field === "" ? "" : `, column ${element.field}`
			}: ${element.message}`,
	)
	return [`${String(bad.length)} row(s) could not be read:`, ...lines].join(
		"\n",
	)
}

// Reads the body, so the response must not have been consumed yet.
async function responseError(response: Response): Promise<ApiError> {
	if (response.status === 401) {
		return new AuthError("Not signed in")
	}
	const body = await response.text()
	const parsed = parseEnvelope(body)
	if (parsed !== null) {
		return new ApiError(
			parsed.code,
			parsed.message,
			parsed.violations,
			parsed.malformed,
		)
	}
	// Not our envelope, so it came from something other than a handler
	// — a proxy, say.
	return new ApiError(
		"internal",
		body.length > 0 ? body : response.statusText,
	)
}

interface Envelope {
	code: string
	message: string
	violations: Violation[]
	malformed: MalformedElement[]
}

function parseEnvelope(body: string): Envelope | null {
	if (body.length === 0) {
		return null
	}
	try {
		const parsed: unknown = JSON.parse(body)
		if (
			parsed !== null &&
			typeof parsed === "object" &&
			"error" in parsed &&
			parsed.error !== null &&
			typeof parsed.error === "object" &&
			"code" in parsed.error &&
			typeof parsed.error.code === "string" &&
			"message" in parsed.error &&
			typeof parsed.error.message === "string"
		) {
			return {
				code: parsed.error.code,
				message: parsed.error.message,
				violations: readArray<Violation>(parsed.error, "violations"),
				malformed: readArray<MalformedElement>(
					parsed.error,
					"malformed",
				),
			}
		}
	} catch {
		// not JSON
	}
	return null
}

// How long a request may take before it is abandoned.
//
// There was no limit at all, and bare fetch has none: a request whose
// connection is gone but not closed never settles, and neither does
// anything waiting on it. One held-open write disabled every button in
// the admin app — still disabled at ninety-five seconds, with no
// error, no spinner and nothing to say why — and in the student app it
// disabled all hundred and fifty Enroll buttons at once, which during
// a selection window is the whole function of the site.
//
// The server's own write timeout does not help: it is the response
// that never arrives, not the work.
//
// Generous, because it is a backstop and not a performance budget. The
// slowest real read measured on a full school is well under a second.
const requestTimeout = 30000

// A timeout that reads as one.
//
// AbortSignal.timeout rejects with a TimeoutError DOMException, whose
// message is "signal timed out" — true, and no use to a person waiting
// on an enrolment. Every other failure in this file arrives as an
// ApiError with prose, so this one does too.
export class TimeoutError extends ApiError {
	public constructor() {
		super(
			"timeout",
			"The server did not answer in time. Check your connection and try again.",
		)
	}
}

async function request(url: string, init: RequestInit): Promise<Response> {
	try {
		return await fetch(url, {
			...init,
			credentials: "include",
			signal: AbortSignal.timeout(requestTimeout),
		})
	} catch (err) {
		if (err instanceof DOMException && err.name === "TimeoutError") {
			throw new TimeoutError()
		}

		throw err
	}
}

export async function getJSON<T>(url: string, init?: RequestInit): Promise<T> {
	const response = await request(url, { ...init })
	if (!response.ok) {
		throw await responseError(response)
	}
	return (await response.json()) as T
}

// The two payload fields are omitted unless their own code is in
// force, so an absent field is the ordinary case, not an error.
function readArray<T>(source: object, field: string): T[] {
	if (!(field in source)) {
		return []
	}
	const value = (source as Record<string, unknown>)[field]
	return Array.isArray(value) ? (value as T[]) : []
}

export function asArray<T>(value: T[] | null | undefined): T[] {
	return Array.isArray(value) ? value : []
}

// A multipart spreadsheet submission; expects no body back.
export async function sendForm(url: string, form: FormData): Promise<void> {
	const response = await request(url, {
		method: "POST",
		body: form,
	})
	if (!response.ok) {
		throw await responseError(response)
	}
}

// A mutation that returns no body (204).
export async function sendJSON(
	url: string,
	method: string,
	body?: unknown,
): Promise<void> {
	const response = await request(url, {
		method,
		...(body === undefined
			? {}
			: {
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify(body),
				}),
	})
	if (!response.ok) {
		throw await responseError(response)
	}
}
