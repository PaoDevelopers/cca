// One student's week, with the details an administrator wants.
import type { Course, Enrollment } from "@common/types"
import type { ReactElement } from "react"
import { policyLabel } from "./enrollments"

interface Props {
	studentID: string
	periods: string[]
	courses: Course[]
	enrollments: Enrollment[]
}

interface Entry {
	id: string
	name: string
	teacher: string
	location: string
	category: string
	policy: string
}

interface Row {
	// Identifies the row for keying; period IDs and course IDs are
	// drawn from the same namespace as each other's labels, so the
	// prefix keeps a period called "Unscheduled" distinct.
	key: string
	label: string
	entries: Entry[]
}

export function AdminSchedule({
	studentID,
	periods,
	courses,
	enrollments,
}: Props): ReactElement {
	const courseByID = new Map(courses.map((c): [string, Course] => [c.id, c]))
	const mine = enrollments.filter((e): boolean => e.student_id === studentID)

	const entry = (enrollment: Enrollment): Entry => {
		const course = courseByID.get(enrollment.course_id)
		return {
			id: enrollment.course_id,
			name: enrollment.course_name,
			teacher: course?.teacher ?? "",
			location: course?.location ?? "",
			category: course?.category_id ?? "",
			policy: policyLabel(enrollment),
		}
	}

	const rows = periods.map((period): Row => ({
		key: `period:${period}`,
		label: period,
		entries: mine
			.filter(
				(e): boolean =>
					courseByID.get(e.course_id)?.period_ids.includes(period) ??
					false,
			)
			.map(entry),
	}))
	// One row per unscheduled course rather than one row listing them
	// all: they share no period, so nothing is said by putting them on
	// the same line.
	for (const enrollment of mine) {
		const course = courseByID.get(enrollment.course_id)
		if (course === undefined || course.period_ids.length === 0) {
			rows.push({
				key: `unscheduled:${enrollment.course_id}`,
				label: "Unscheduled",
				entries: [entry(enrollment)],
			})
		}
	}

	const joined = (row: Row, field: keyof Entry): string =>
		row.entries.map((e): string => e[field]).join(", ")

	return (
		<div className="overflow-x-auto rounded-md border bg-background">
			<table className="w-full text-sm">
				<thead className="bg-muted/50 text-left text-muted-foreground">
					<tr>
						{[
							"Period",
							"ID",
							"Title",
							"Teacher",
							"Location",
							"Category",
							"Policy",
						].map((label) => (
							<th
								key={label}
								scope="col"
								className="px-3 py-2 font-medium"
							>
								{label}
							</th>
						))}
					</tr>
				</thead>
				<tbody>
					{rows.length === 0 ? (
						<tr>
							<td colSpan={7} className="px-3 py-2">
								No periods defined.
							</td>
						</tr>
					) : (
						rows.map((row) => {
							const empty =
								row.entries.length === 0 ? "Empty" : undefined
							return (
								<tr key={row.key} className="border-t">
									<th
										scope="row"
										className="px-3 py-2 text-left font-medium"
									>
										{row.label}
									</th>
									<td
										aria-label={empty}
										className="px-3 py-2"
									>
										{row.entries.map((e, position) => (
											<span key={e.id}>
												{position > 0 && ", "}
												<code>{e.id}</code>
											</span>
										))}
									</td>
									{(
										[
											"name",
											"teacher",
											"location",
											"category",
											"policy",
										] as const
									).map((field) => (
										<td
											key={field}
											aria-label={empty}
											className="px-3 py-2"
										>
											{joined(row, field)}
										</td>
									))}
								</tr>
							)
						})
					)}
				</tbody>
			</table>
		</div>
	)
}
