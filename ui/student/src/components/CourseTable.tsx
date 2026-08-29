import type { ReactElement } from "react"
import { capacityLabel } from "@common/capacity"
import type { CourseActions, CourseRow } from "@/lib/enrollment"
import { EnrollmentActions } from "./EnrollmentActions"

const columns = [
	"ID",
	"Title",
	"Teacher",
	"Location",
	"Category",
	"Periods",
	"Term",
	"Cost",
	"Students",
]

const rowClass = "border-b transition-colors hover:bg-muted/50"
const headClass =
	"h-10 px-2 text-left align-middle font-medium whitespace-nowrap text-foreground"
const cellClass = "p-2 align-middle whitespace-nowrap"

// No empty state: the caller shows its own message rather than
// rendering a table with one apologetic row in it.
export function CourseTable({
	rows,
	actions,
}: {
	rows: CourseRow[]
	actions: CourseActions
}): ReactElement {
	return (
		<div className="overflow-x-auto rounded-lg border">
			<table className="w-full text-sm">
				<thead>
					<tr className={rowClass}>
						{columns.map((column): ReactElement => (
							<th key={column} scope="col" className={headClass}>
								{column}
							</th>
						))}
						<th className={headClass} scope="col">
							Action
						</th>
						<th className={headClass} scope="col">
							Status
						</th>
					</tr>
				</thead>
				<tbody>
					{rows.map((row): ReactElement => {
						const { course, state } = row
						const note =
							state.status !== ""
								? state.status
								: state.reasons.join("; ")
						const noteColor =
							state.fixed ||
							(state.status === "" && state.reasons.length > 0)
								? "text-amber-700 dark:text-amber-400"
								: "text-muted-foreground"
						return (
							<tr
								key={course.id}
								className={`${rowClass} last:border-0`}
							>
								<th scope="row" className={headClass}>
									<code>{course.id}</code>
								</th>
								<td className={cellClass}>{course.name}</td>
								<td className={cellClass}>
									{course.teacher_email !== "" ? (
										<a
											className="underline underline-offset-2"
											href={`mailto:${course.teacher_email}`}
										>
											{course.teacher}
										</a>
									) : (
										course.teacher
									)}
								</td>
								<td className={cellClass}>{course.location}</td>
								<td className={cellClass}>
									{course.category_id}
								</td>
								<td className={cellClass}>
									{course.period_ids.join(", ")}
								</td>
								<td className={cellClass}>{course.term}</td>
								<td className={cellClass}>{course.cost}</td>
								<td className={`${cellClass} tabular-nums`}>
									{course.current_students}/
									{capacityLabel(course.max_students)}
								</td>
								<td className={cellClass}>
									<EnrollmentActions
										row={row}
										actions={actions}
										variant="table"
									/>
								</td>
								<td className={cellClass}>
									<span className={noteColor}>{note}</span>
								</td>
							</tr>
						)
					})}
				</tbody>
			</table>
		</div>
	)
}
