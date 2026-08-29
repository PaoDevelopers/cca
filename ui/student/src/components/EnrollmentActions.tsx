import {
	cloneElement,
	useId,
	useRef,
	type ComponentProps,
	type ReactElement,
} from "react"
import { capacitySpoken } from "@common/capacity"
import type { CourseActions, CourseRow } from "@/lib/enrollment"
import { Button } from "@/components/ui/button"

interface Props {
	row: CourseRow
	actions: CourseActions
	// "card" is the compact header form: an icon button, with the card
	// itself printing the status and the reasons. "table" spells the
	// action out and carries the status beside it, as the column needs.
	//
	// It also decides where the seat count goes. In card view the button
	// is an icon, so the count rides in the accessible name; the table
	// has a column for it.
	variant: "card" | "table"
	// The element listing why this course is barred, when the card owns
	// it. Wired to the enroll button through aria-describedby.
	reasonsId?: string | undefined
}

// What the student may do with one course, and why not when they may
// not.
//
// The reasons come from the server as violations, never recomputed
// here: the same function that judges the write judges the display, so
// a course that looks available cannot be refused, and one that looks
// barred cannot secretly be allowed.
export function EnrollmentActions({
	row,
	actions,
	variant,
	reasonsId,
}: Props): ReactElement {
	const { course, state } = row
	const { windowOpen, updating, onenroll, ondrop, onswap } = actions

	const icon = variant === "card"
	const size = icon ? "icon" : "sm"
	const actionWidth = icon ? undefined : "w-16"

	// The accessible names put the count where the visible text has it,
	// and the course name after. They used to read "Enroll in Chess",
	// which overrides the button's own text, so the seat count was
	// simply not there for anyone using a screen reader.
	const count = icon
		? ` (${String(course.current_students)}/${capacitySpoken(
				course.max_students,
			)})`
		: ""

	if (!windowOpen) {
		// The window is the gate; the server refuses regardless, so this
		// only spares the student a rejection they can do nothing about.
		return <Closed>Enrollment closed</Closed>
	}
	if (state.fixed) {
		return (
			<Button
				className={actionWidth}
				size={size}
				disabled
				aria-label={`Drop${count} ${course.name}: placed by an administrator and cannot be dropped`}
			>
				{icon ? "−" : `Drop${count}`}
			</Button>
		)
	}

	if (state.selected) {
		return (
			<ConfirmAction
				trigger={
					<Button
						className={actionWidth}
						size={size}
						disabled={updating}
						aria-label={`Drop${count} ${course.name}`}
						data-course-action={course.id}
					>
						{icon ? "−" : `Drop${count}`}
					</Button>
				}
				title={`Drop ${course.name}?`}
				description="This will remove the course from your selections."
				confirmLabel="Confirm drop"
				confirmAriaLabel={`Confirm dropping ${course.name}`}
				destructive
				updating={updating}
				courseId={course.id}
				onConfirm={(): void => {
					ondrop(course)
				}}
			/>
		)
	}

	// Swapping is one operation, not a drop followed by an enroll: the
	// new course is judged with the old ones disregarded, which is the
	// only way to move between two courses that clash.
	if (row.canSwap && onswap !== undefined) {
		return (
			<ConfirmAction
				trigger={
					<Button
						className={actionWidth}
						variant="outline"
						size={size}
						disabled={updating}
						aria-label={`Swap in${count}, replacing the courses ${course.name} clashes with`}
						data-course-action={course.id}
					>
						{icon ? "⇄" : "Swap"}
					</Button>
				}
				title={`Swap ${row.swapFrom} into ${course.name}?`}
				description="This will replace the conflicting selections with this course."
				confirmLabel="Confirm swap"
				confirmAriaLabel={`Confirm swapping into ${course.name}`}
				updating={updating}
				courseId={course.id}
				onConfirm={(): void => {
					onswap(course)
				}}
			/>
		)
	}

	return (
		<Button
			className={actionWidth}
			variant="outline"
			size={size}
			disabled={updating || state.barred}
			aria-label={`Enroll${count} in ${course.name}`}
			aria-describedby={
				state.barred && state.reasons.length > 0 ? reasonsId : undefined
			}
			data-course-action={course.id}
			onClick={(): void => {
				onenroll(course)
			}}
		>
			{icon ? "+" : `Enroll${count}`}
		</Button>
	)
}

function Closed({ children }: { children: string }): ReactElement {
	return (
		<span className="whitespace-nowrap text-xs text-muted-foreground">
			{children}
		</span>
	)
}

function ConfirmAction({
	trigger,
	title,
	description,
	confirmLabel,
	confirmAriaLabel,
	destructive = false,
	updating,
	courseId,
	onConfirm,
}: {
	trigger: ReactElement<ComponentProps<typeof Button>>
	title: string
	description: string
	confirmLabel: string
	confirmAriaLabel: string
	destructive?: boolean
	updating: boolean
	courseId: string
	onConfirm: () => void
}): ReactElement {
	const dialog = useRef<HTMLDialogElement>(null)
	const titleId = useId()
	const descriptionId = useId()

	return (
		<>
			{cloneElement(trigger, {
				"aria-haspopup": "dialog",
				onClick: (): void => {
					dialog.current?.showModal()
				},
			})}
			<dialog
				ref={dialog}
				role="alertdialog"
				aria-labelledby={titleId}
				aria-describedby={descriptionId}
				className="fixed top-1/2 left-1/2 z-50 m-0 w-full max-w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 gap-4 rounded-lg border bg-background p-6 text-foreground shadow-lg backdrop:bg-black/50 open:grid sm:max-w-lg"
			>
				<div className="flex flex-col gap-2 text-center sm:text-left">
					<h2 id={titleId} className="text-lg font-semibold">
						{title}
					</h2>
					<p
						id={descriptionId}
						className="text-sm text-muted-foreground"
					>
						{description}
					</p>
				</div>
				<div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
					<Button
						variant="outline"
						size="sm"
						onClick={(): void => {
							dialog.current?.close()
						}}
					>
						Cancel
					</Button>
					<Button
						variant={destructive ? "destructive" : "default"}
						size="sm"
						disabled={updating}
						aria-label={confirmAriaLabel}
						data-course-action={courseId}
						onClick={(): void => {
							dialog.current?.close()
							onConfirm()
						}}
					>
						{confirmLabel}
					</Button>
				</div>
			</dialog>
		</>
	)
}
