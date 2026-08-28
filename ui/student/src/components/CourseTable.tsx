import type { ReactElement } from "react"
import { capacityLabel } from "@common/capacity"
import type { CourseActions, CourseRow } from "@/lib/enrollment"
import { EnrollmentActions } from "./EnrollmentActions"
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/ui/table"

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
			<Table className="min-w-[78rem]">
				<TableHeader>
					<TableRow>
						{columns.map((column): ReactElement => (
							<TableHead key={column} scope="col">
								{column}
							</TableHead>
						))}
						<TableHead className="w-96 min-w-96" scope="col">
							Status
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{rows.map((row): ReactElement => {
						const { course, state } = row
						const note =
							state.status !== ""
								? state.status
								: state.reasons.join("; ")
						const noteColor =
							state.status === "" && state.reasons.length > 0
								? "text-amber-700 dark:text-amber-400"
								: "text-muted-foreground"
						return (
							<TableRow key={course.id}>
								<TableHead scope="row">
									<code>{course.id}</code>
								</TableHead>
								<TableCell>{course.name}</TableCell>
								<TableCell>
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
								</TableCell>
								<TableCell>{course.location}</TableCell>
								<TableCell>{course.category_id}</TableCell>
								<TableCell>
									{course.period_ids.join(", ")}
								</TableCell>
								<TableCell>{course.term}</TableCell>
								<TableCell>{course.cost}</TableCell>
								<TableCell className="tabular-nums">
									{course.current_students}/
									{capacityLabel(course.max_students)}
								</TableCell>
								<TableCell className="w-96 min-w-96 max-w-96">
									<span className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3">
										<span
											className={`truncate ${noteColor}`}
											title={
												note === "" ? undefined : note
											}
										>
											{note}
										</span>
										<EnrollmentActions
											row={row}
											actions={actions}
											variant="table"
										/>
									</span>
								</TableCell>
							</TableRow>
						)
					})}
				</TableBody>
			</Table>
		</div>
	)
}
