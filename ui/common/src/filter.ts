// Shared list-filter syntax: whitespace-separated terms must all
// match. A bare term matches any field; a `field:value` term matches
// only that field; commas inside the value are alternatives
// (`grade:y9,y10` means Y9 or Y10). Matching is case-insensitive.
//
// A field given as a plain string matches on substrings (names,
// titles); `{ exact: … }` fields (IDs, grades, types) only match whole
// values. Unknown field names fall back to matching the whole term
// against any field.
//
// This stays the default because it is better for interactive
// narrowing — `bak` rather than `name.contains("bak")`. CEL (./cel.ts)
// is opt-in per list, for the cross-entity questions this syntax
// cannot express at all.
export type FilterFieldSpec = string | { exact: string } | { exact: string[] }

function fieldMatches(spec: FilterFieldSpec, value: string): boolean {
	if (typeof spec === "string") {
		return spec.toLowerCase().includes(value)
	}
	if (typeof spec.exact === "string") {
		return spec.exact.toLowerCase() === value
	}
	return spec.exact.some(
		(candidate): boolean => candidate.toLowerCase() === value,
	)
}

export function matchesFilter(
	query: string,
	fields: Record<string, FilterFieldSpec>,
): boolean {
	const terms = query
		.trim()
		.toLowerCase()
		.split(/\s+/)
		.filter((term): boolean => term !== "")
	return terms.every((term): boolean => {
		const colon = term.indexOf(":")
		if (colon > 0) {
			const field = fields[term.slice(0, colon)]
			if (field !== undefined) {
				return term
					.slice(colon + 1)
					.split(",")
					.filter((value): boolean => value !== "")
					.some((value): boolean => fieldMatches(field, value))
			}
		}
		return Object.values(fields).some((field): boolean =>
			fieldMatches(field, term),
		)
	})
}
