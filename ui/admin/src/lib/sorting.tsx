// Client-side column sorting for the admin tables. null is the
// server's order, which is each list's own.
import { TableHead } from "@common/components/ui/table"
import type { ReactElement } from "react"

export interface Sort<K extends string> {
	key: K
	desc: boolean
}

export function sortRows<T, K extends string>(
	rows: T[],
	sort: Sort<K> | null,
	value: (row: T, key: K) => string | number,
): T[] {
	if (sort === null) {
		return rows
	}
	return [...rows].sort((a, b): number => {
		const x = value(a, sort.key)
		const y = value(b, sort.key)
		const order =
			typeof x === "number" && typeof y === "number"
				? x - y
				: String(x).localeCompare(String(y), undefined, {
						numeric: true,
					})
		return sort.desc ? -order : order
	})
}

const glyph = { ascending: "↑", descending: "↓", none: "↕" } as const

// A column header that sorts its table: ascending, then descending,
// then back to the server's order.
export function SortableHead<K extends string>({
	column,
	label,
	sort,
	onSort,
}: {
	column: K
	label: string
	sort: Sort<K> | null
	onSort: (sort: Sort<K> | null) => void
}): ReactElement {
	let direction: keyof typeof glyph = "none"
	if (sort?.key === column) {
		direction = sort.desc ? "descending" : "ascending"
	}
	return (
		<TableHead scope="col" aria-sort={direction}>
			<button
				type="button"
				className="inline-flex items-center gap-1 hover:text-foreground"
				onClick={(): void => {
					if (direction === "none") {
						onSort({ key: column, desc: false })
					} else {
						onSort(
							direction === "ascending"
								? { key: column, desc: true }
								: null,
						)
					}
				}}
			>
				{label}
				<span aria-hidden="true" className="text-xs">
					{glyph[direction]}
				</span>
			</button>
		</TableHead>
	)
}
