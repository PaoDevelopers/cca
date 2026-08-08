// CEL filtering, the expressive alternative to ./filter.ts. The simple
// syntax matches flat strings and cannot reach across entities; CEL
// evaluates against a nested context, so an enrollment row can address
// its course's attributes.
//
// Errors are reported rather than swallowed: a typo like `course.nmae`
// should read as an error, not as an inexplicably empty table.

import { parse } from "@marcbachmann/cel-js"

export interface CompiledFilter {
	// Rows that throw do not match.
	matches: (row: Record<string, unknown>) => boolean
	// The first evaluation problem seen; read after filtering a list.
	readonly runtimeError: string | null
}

export interface CompileResult {
	// null when the expression could not be parsed at all.
	filter: CompiledFilter | null
	parseError: string | null
}

// cel-js errors carry multi-line source highlighting, far too tall for
// an inline message.
function firstLine(err: unknown): string {
	const message = err instanceof Error ? err.message : String(err)
	return message.split("\n")[0] ?? "Invalid expression"
}

// An empty expression is not an error, it just filters nothing.
export function compileFilter(expression: string): CompileResult {
	if (expression.trim() === "") {
		return {
			filter: { matches: (): boolean => true, runtimeError: null },
			parseError: null,
		}
	}

	let run: (context: Record<string, unknown>) => unknown
	try {
		run = parse(expression) as (context: Record<string, unknown>) => unknown
	} catch (err) {
		return { filter: null, parseError: firstLine(err) }
	}

	let runtimeError: string | null = null
	const filter: CompiledFilter = {
		matches(row: Record<string, unknown>): boolean {
			let result: unknown
			try {
				result = run(row)
			} catch (err) {
				runtimeError ??= firstLine(err)
				return false
			}
			if (typeof result !== "boolean") {
				runtimeError ??= `Expression returned ${typeof result}, expected true or false`
				return false
			}
			return result
		},
		get runtimeError(): string | null {
			return runtimeError
		},
	}
	return { filter, parseError: null }
}

export type FilterMode = "simple" | "cel"

export interface CelFilterResult<T> {
	rows: T[]
	error: string | null
	// True when the expression did not compile, so `rows` is every
	// item rather than the ones that matched.
	//
	// The distinction matters because "the filtered rows" is what the
	// bulk controls act on. An administrator narrowing a list of two
	// hundred courses down to three, mistyping the expression, then
	// selecting all and deleting would take the whole catalogue —
	// the error was on the screen, but so was every row, and a table
	// that looks unfiltered because the filter is broken looks
	// identical to one that is unfiltered because nothing was typed.
	//
	// Showing everything is still the right thing to *display*: a
	// half-typed expression should not blank the table under the
	// cursor. It is acting on it in bulk that must not be offered.
	unfiltered: boolean
}

// A parse error leaves the list unfiltered; a runtime error still
// filters. Both are reported for inline display.
export function filterByCel<T>(
	expression: string,
	items: T[],
	context: (item: T) => Record<string, unknown>,
): CelFilterResult<T> {
	const { filter, parseError } = compileFilter(expression)
	if (filter === null) {
		return { rows: items, error: parseError, unfiltered: true }
	}
	const rows = items.filter((item): boolean => filter.matches(context(item)))
	return { rows, error: filter.runtimeError, unfiltered: false }
}
