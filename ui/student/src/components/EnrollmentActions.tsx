import { useCallback, useState, type ReactElement } from "react"
import { ArrowLeftRight, Minus, Plus, X } from "lucide-react"
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

	// Dropping and swapping ask for an inline confirmation first.
	const [confirmingDrop, setConfirmingDrop] = useState(false)
	const [confirmingSwap, setConfirmingSwap] = useState(false)

	// Arming a confirmation destroys the button that was pressed and
	// mounts two new ones, so the focus falls to <body>: a student hears
	// nothing, and the button they must now press is two tab stops
	// further on with nothing to say it appeared.
	const focusConfirm = useCallback((node: HTMLButtonElement | null): void => {
		node?.focus()
	}, [])

	const icon = variant === "card"
	const size = icon ? "icon" : "sm"

	// The accessible names put the count where the visible text has it,
	// and the course name after. They used to read "Enroll in Chess",
	// which overrides the button's own text, so the seat count was
	// simply not there for anyone using a screen reader.
	const count = icon
		? ` (${String(course.current_students)}/${String(course.max_students)})`
		: ""

	if (!windowOpen) {
		// The window is the gate; the server refuses regardless, so this
		// only spares the student a rejection they can do nothing about.
		return <Closed>Enrollment closed</Closed>
	}
	if (state.fixed) {
		return <Closed>Cannot be dropped</Closed>
	}

	if (state.selected) {
		return confirmingDrop ? (
			<Pair>
				<Button
					variant="outline"
					size={size}
					aria-label={`Cancel dropping ${course.name}`}
					onClick={(): void => {
						setConfirmingDrop(false)
					}}
				>
					{icon ? <X aria-hidden="true" /> : "X"}
				</Button>
				<Button
					variant="destructive"
					size="sm"
					disabled={updating}
					aria-label={`Confirm dropping ${course.name}`}
					data-course-action={course.id}
					ref={focusConfirm}
					onClick={(): void => {
						setConfirmingDrop(false)
						ondrop(course)
					}}
				>
					Confirm drop
				</Button>
			</Pair>
		) : (
			<Button
				variant="outline"
				size={size}
				disabled={updating}
				aria-label={`Drop${count} ${course.name}`}
				data-course-action={course.id}
				onClick={(): void => {
					setConfirmingDrop(true)
				}}
			>
				{icon ? <Minus aria-hidden="true" /> : `Drop${count}`}
			</Button>
		)
	}

	// Swapping is one operation, not a drop followed by an enroll: the
	// new course is judged with the old ones disregarded, which is the
	// only way to move between two courses that clash.
	if (row.canSwap && onswap !== undefined) {
		return confirmingSwap ? (
			<Pair>
				<Button
					variant="outline"
					size={size}
					aria-label={`Cancel swapping into ${course.name}`}
					data-course-action={course.id}
					onClick={(): void => {
						setConfirmingSwap(false)
					}}
				>
					{icon ? <X aria-hidden="true" /> : "X"}
				</Button>
				<Button
					size="sm"
					disabled={updating}
					aria-label={`Confirm swapping into ${course.name}`}
					data-course-action={course.id}
					ref={focusConfirm}
					onClick={(): void => {
						setConfirmingSwap(false)
						onswap(course)
					}}
				>
					Confirm swap
				</Button>
			</Pair>
		) : (
			<Button
				variant="outline"
				size={size}
				disabled={updating}
				aria-label={`Swap in${count}, replacing the courses ${course.name} clashes with`}
				data-course-action={course.id}
				onClick={(): void => {
					setConfirmingSwap(true)
				}}
			>
				{icon ? <ArrowLeftRight aria-hidden="true" /> : "Swap"}
			</Button>
		)
	}

	return (
		<Button
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
			{icon ? <Plus aria-hidden="true" /> : `Enroll${count}`}
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

function Pair({ children }: { children: ReactElement[] }): ReactElement {
	return <span className="flex items-center gap-1">{children}</span>
}
