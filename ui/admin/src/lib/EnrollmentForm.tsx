// Placing students into one course. One course, because that is the
// unit the database locks and judges as a whole — and because its batch
// order is significant: students compete for the same seats, and the
// ones named first win when the course fills.
import { placeEnrollments } from "@common/adminApi"
import { Button } from "@common/components/ui/button"
import { Checkbox, Input, Label } from "@common/components/ui/input"
import { toast } from "@common/components/ui/toast"
import type { Course } from "@common/types"
import { useId, useState, type ReactElement } from "react"
import { courseLabel } from "./enrollments"

interface Props {
	courses: Course[]
	courseByID: Map<string, Course>
	busy: boolean
	// Placement can break every negotiable rule, so it goes through the
	// accept protocol rather than an override switch.
	runAccepting: (
		action: (accept: string[]) => Promise<void>,
	) => Promise<boolean>
}

export function EnrollmentForm({
	courses,
	courseByID,
	busy,
	runAccepting,
}: Props): ReactElement {
	const uid = useId()
	const [newStudents, setNewStudents] = useState("")
	const [newCourse, setNewCourse] = useState("")
	// The two policy bits, chosen independently. The old three-valued
	// type could not say "the student's own committed pick"; these can.
	const [droppable, setDroppable] = useState(true)
	const [budgeted, setBudgeted] = useState(false)

	return (
		<form
			className="flex flex-wrap items-end gap-3"
			onSubmit={(event): void => {
				event.preventDefault()
				// Not sorted and not deduplicated: the order the
				// administrator typed is the order seats are handed out in.
				const ids = newStudents
					.split(/[\s,]+/)
					.filter((part): boolean => part !== "")
				const course = newCourse.trim()
				if (ids.length === 0 || course === "") {
					return
				}
				void runAccepting(async (accept): Promise<void> => {
					await placeEnrollments(
						course,
						ids,
						droppable,
						budgeted,
						accept,
					)
					setNewStudents("")
					setNewCourse("")
				}).then((ok): void => {
					if (ok) {
						toast(
							`Placed ${String(ids.length)} ${ids.length === 1 ? "student" : "students"} in ${course}.`,
						)
					}
				})
			}}
		>
			<Label className="min-w-64 flex-1 flex-col items-start">
				Student IDs (comma or space separated)
				<Input
					type="text"
					value={newStudents}
					placeholder="s22537 s23321"
					required
					onChange={(event): void => {
						setNewStudents(event.currentTarget.value)
					}}
				/>
			</Label>
			<Label className="flex-col items-start">
				Course
				<Input
					type="text"
					list={`${uid}-courses`}
					value={newCourse}
					required
					onChange={(event): void => {
						setNewCourse(event.currentTarget.value)
					}}
				/>
			</Label>
			<datalist id={`${uid}-courses`}>
				{courses.map((course) => (
					<option key={course.id} value={course.id}>
						{courseLabel(courseByID, course.id, course.name)}
					</option>
				))}
			</datalist>
			<Label className="h-9">
				<Checkbox
					checked={droppable}
					onChange={(event): void => {
						setDroppable(event.currentTarget.checked)
					}}
				/>
				Student may drop
			</Label>
			<Label className="h-9">
				<Checkbox
					checked={budgeted}
					onChange={(event): void => {
						setBudgeted(event.currentTarget.checked)
					}}
				/>
				Counts toward their budget
			</Label>
			<Button size="default" disabled={busy}>
				Place
			</Button>
		</form>
	)
}
