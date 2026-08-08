// Between the wire's RFC 3339 instants and what <input
// type="datetime-local"> speaks, which is a wall-clock string with no
// zone at all.
//
// The browser's zone is the administrator's, and the school runs in
// one place, so interpreting a typed time as local and displaying an
// instant as local is what they mean. Storage stays absolute: the
// server holds timestamptz, so a zone change or a daylight-saving
// transition cannot move a window that was already set.

// "2026-09-01T08:00" in local time, for the input's value.
export function toLocalInput(iso: string | null): string {
	if (iso === null) {
		return ""
	}
	const at = new Date(iso)
	if (Number.isNaN(at.getTime())) {
		return ""
	}
	const pad = (n: number): string => String(n).padStart(2, "0")
	// The year is padded to four digits like everything else. A
	// three-digit year produced "500-01-01T00:00", which is not a
	// valid datetime-local value, so the box rendered empty while the
	// prose beside it read the date correctly — two different answers
	// on one card. Only reachable through the API, but the fix is the
	// same character either way.
	return (
		`${String(at.getFullYear()).padStart(4, "0")}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}` +
		`T${pad(at.getHours())}:${pad(at.getMinutes())}`
	)
}

// The inverse. An empty box means "no such bound", which is null on
// the wire and is meaningful: no opens_at is a closed window, no
// closes_at is one that never closes on its own.
export function fromLocalInput(value: string): string | null {
	if (value.trim() === "") {
		return null
	}
	const at = new Date(value)
	return Number.isNaN(at.getTime()) ? null : at.toISOString()
}

// For display beside the inputs.
export function formatInstant(iso: string | null): string {
	if (iso === null) {
		return "—"
	}
	const at = new Date(iso)
	return Number.isNaN(at.getTime()) ? "—" : at.toLocaleString()
}
