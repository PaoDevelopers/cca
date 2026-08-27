import type { ReactElement } from "react"
import type { Category } from "@common/types"
import type { CourseActions, CourseRow } from "@/lib/enrollment"
import { CourseCard } from "./CourseCard"
import { CourseTable } from "./CourseTable"

interface Props {
	rows: CourseRow[]
	categories: Category[]
	// What to say when there is nothing to list.
	empty: string
	view: "cards" | "table"
	actions: CourseActions
}

interface Group {
	// The category id, which is what the heading's id is built from:
	// names may contain spaces and would not be valid there.
	id: string
	name: string
	rows: CourseRow[]
}

// Rows under their category, in the order the server gives the
// categories. A course whose category is not in the list still gets a
// group, keyed by its raw id, so nothing silently disappears from a
// catalogue because an administrator removed a category.
function byCategory(rows: CourseRow[], categories: Category[]): Group[] {
	const groups = new Map<string, Group>()

	for (const category of categories) {
		groups.set(category.id, {
			id: category.id,
			name: category.name,
			rows: [],
		})
	}
	for (const row of rows) {
		const id = row.course.category_id
		let group = groups.get(id)
		if (group === undefined) {
			group = { id, name: id, rows: [] }
			groups.set(id, group)
		}
		group.rows.push(row)
	}

	return [...groups.values()].filter((g): boolean => g.rows.length > 0)
}

// A list of courses as cards or as a table, shared by the student's own
// selections and the whole catalogue.
export function CourseList({
	rows,
	categories,
	empty,
	view,
	actions,
}: Props): ReactElement {
	if (rows.length === 0) {
		return <p className="text-muted-foreground">{empty}</p>
	}

	if (view === "table") {
		return <CourseTable rows={rows} actions={actions} />
	}

	return (
		<div className="flex flex-col gap-8">
			{byCategory(rows, categories).map((group): ReactElement => (
				<section key={group.id} aria-labelledby={`cat-${group.id}`}>
					<h3
						id={`cat-${group.id}`}
						className="mb-4 text-lg text-muted-foreground"
					>
						{group.name}
					</h3>
					<div className="grid gap-5 md:grid-cols-2 2xl:grid-cols-3">
						{group.rows.map((row): ReactElement => (
							<CourseCard
								key={row.course.id}
								row={row}
								actions={actions}
							/>
						))}
					</div>
				</section>
			))}
		</div>
	)
}
