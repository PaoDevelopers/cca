// One student's row, and their week under it when it is expanded.
import type { AdminStudent } from "@common/adminApi"
import { Badge } from "@common/components/ui/badge"
import { Button } from "@common/components/ui/button"
import { TableCell, TableRow } from "@common/components/ui/table"
import type { Course, Enrollment } from "@common/types"
import type { ReactElement } from "react"
import { AdminSchedule } from "./AdminSchedule"
import { ConfirmDelete } from "./ConfirmDelete"
import { CalendarIcon, ListIcon, PencilIcon, SignInIcon } from "./icons"

interface Props {
	student: AdminStudent
	budget: string
	requirements: string
	busy: boolean
	scheduleOpen: boolean
	periods: string[]
	courses: Course[]
	enrollments: Enrollment[]
	onedit: () => void
	ondelete: () => void
	onimpersonate: () => void
	ontoggleschedule: () => void
}

export function StudentRow({
	student,
	budget,
	requirements,
	busy,
	scheduleOpen,
	periods,
	courses,
	enrollments,
	onedit,
	ondelete,
	onimpersonate,
	ontoggleschedule,
}: Props): ReactElement {
	return (
		<>
			<TableRow data-state={scheduleOpen ? "selected" : undefined}>
				<th
					scope="row"
					className="px-3 py-2 text-left font-mono font-medium"
				>
					{student.id}
				</th>
				<TableCell>{student.name}</TableCell>
				<TableCell>{student.grade_id}</TableCell>
				<TableCell>{student.legal_sex}</TableCell>
				<TableCell className="tabular-nums">{budget}</TableCell>
				<TableCell>
					<Badge
						variant={
							requirements === "OK" ? "secondary" : "destructive"
						}
					>
						{requirements}
					</Badge>
				</TableCell>
				<TableCell>
					<div className="flex flex-wrap gap-0.5">
						<Button
							variant="ghost"
							size="icon"
							// Pressed-looking while the schedule is open under the
							// row, which is what "Close" used to say in words.
							className={
								scheduleOpen
									? "bg-accent text-foreground"
									: "text-muted-foreground hover:text-foreground"
							}
							aria-expanded={scheduleOpen}
							aria-label={
								scheduleOpen
									? `Close the schedule of ${student.id}`
									: `View the schedule of ${student.id}`
							}
							tooltip={
								scheduleOpen
									? `Close the schedule of ${student.id}`
									: `View the schedule of ${student.id}`
							}
							onClick={ontoggleschedule}
						>
							<CalendarIcon />
						</Button>
						<Button
							variant="ghost"
							size="icon"
							className="text-muted-foreground hover:text-foreground"
							aria-label={`View enrollments of ${student.id}`}
							tooltip={`View enrollments of ${student.id}`}
							onClick={(): void => {
								window.location.hash = `#/enrollments?q=${encodeURIComponent(
									`student:${student.id}`,
								)}`
							}}
						>
							<ListIcon />
						</Button>
						<Button
							variant="ghost"
							size="icon"
							className="text-muted-foreground hover:text-foreground"
							disabled={busy}
							aria-label={`Sign in as ${student.id}`}
							tooltip={`Sign in as ${student.id}`}
							onClick={onimpersonate}
						>
							<SignInIcon />
						</Button>
						<Button
							variant="ghost"
							size="icon"
							className="text-muted-foreground hover:text-foreground"
							disabled={busy}
							aria-label={`Edit ${student.id}`}
							tooltip={`Edit ${student.id}`}
							onClick={onedit}
						>
							<PencilIcon />
						</Button>
						<ConfirmDelete
							action="Delete"
							target={student.id}
							disabled={busy}
							onconfirm={ondelete}
						/>
					</div>
				</TableCell>
			</TableRow>
			{scheduleOpen && (
				<TableRow className="hover:bg-transparent">
					<TableCell colSpan={7} className="bg-muted/30">
						<AdminSchedule
							studentID={student.id}
							periods={periods}
							courses={courses}
							enrollments={enrollments}
						/>
					</TableCell>
				</TableRow>
			)}
		</>
	)
}
