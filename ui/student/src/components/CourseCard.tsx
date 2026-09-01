import {
	useEffect,
	useId,
	useRef,
	useState,
	type ReactElement,
	type ReactNode,
} from "react"
import { capacityLabel } from "@common/capacity"
import type { CourseActions, CourseRow } from "@/lib/enrollment"
import { EnrollmentActions } from "./EnrollmentActions"

// One labelled fact. The label left, the value flush right: "Kitchen"
// alone does not say it is a location.
function Fact({
	label,
	children,
}: {
	label: string
	children: ReactNode
}): ReactElement {
	return (
		<div className="flex items-baseline justify-between gap-3">
			<dt className="shrink-0 text-muted-foreground">{label}</dt>
			<dd className="truncate text-right font-medium">{children}</dd>
		</div>
	)
}

export function CourseCard({
	row,
	actions,
}: {
	row: CourseRow
	actions: CourseActions
}): ReactElement {
	// The card is labelled by its course name, which is not the
	// article's first child.
	const uid = useId()
	const { course, state } = row

	const [expanded, setExpanded] = useState(false)
	const [clipped, setClipped] = useState(false)
	const description = useRef<HTMLParagraphElement>(null)

	// Whether the clamp is actually hiding anything, so that a course
	// described in four words does not get a control that reveals
	// nothing.
	//
	// Measured rather than guessed from a character count: how many
	// lines the text takes depends on the column width, and the cards
	// reflow from three across to one. Hence a ResizeObserver and not a
	// single look on mount.
	useEffect((): (() => void) => {
		const element = description.current

		const measure = (): void => {
			if (element !== null) {
				setClipped(element.scrollHeight > element.clientHeight)
			}
		}

		measure()

		const observer = new ResizeObserver(measure)
		if (element !== null) {
			observer.observe(element)
		}

		return (): void => {
			observer.disconnect()
		}
	}, [course.description])
	const note = state.status !== "" ? state.status : state.reasons.join("; ")
	const noteColor =
		state.fixed || (state.status === "" && state.reasons.length > 0)
			? "text-amber-700 dark:text-amber-400"
			: "text-muted-foreground"

	return (
		<article
			className="card flex flex-col gap-3 rounded-xl border bg-card p-4"
			aria-labelledby={`${uid}-title`}
		>
			<div className="flex items-start justify-between gap-3">
				<div className="min-w-0">
					<h4
						id={`${uid}-title`}
						className="truncate text-[15px] font-semibold"
					>
						{course.name}
					</h4>
					<code className="text-xs text-muted-foreground">
						{course.id}
					</code>
				</div>
				<EnrollmentActions
					row={row}
					actions={actions}
					variant="card"
					reasonsId={`${uid}-reasons`}
				/>
			</div>

			{/*
				Clamped rather than left to run: a course with a paragraph
				of description made its card twice the height of its
				neighbours, and the grid row stretched to match.

				It used to be clamped and nothing else, with the full text
				only in a title attribute — which is a tooltip, so on a
				phone there was no way to read the rest of it at all. The
				button gives it back. Expanding does stretch its grid row,
				but that is a row the student asked to be taller.
			*/}
			{course.description !== "" && (
				<div className="flex flex-col items-start gap-1">
					<p
						ref={description}
						id={`${uid}-description`}
						className={
							expanded ? "text-sm" : "line-clamp-2 text-sm"
						}
					>
						{course.description}
					</p>
					{/*
						Kept while expanded as well as while clipped: once
						open the paragraph fits by definition, and dropping
						the control then would leave no way back.
					*/}
					{(clipped || expanded) && (
						<button
							type="button"
							aria-expanded={expanded}
							aria-controls={`${uid}-description`}
							className="cursor-pointer text-xs text-muted-foreground underline underline-offset-2 hover:text-foreground"
							onClick={(): void => {
								setExpanded((open): boolean => !open)
							}}
						>
							{expanded ? "Show less" : "Show more"}
						</button>
					)}
				</div>
			)}

			<dl className="flex flex-col gap-1.5 text-sm">
				<Fact label="Teacher">
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
				</Fact>
				<Fact label="Location">{course.location}</Fact>
				<Fact label="Periods">{course.period_ids.join(", ")}</Fact>
				<Fact label="Term">{course.term}</Fact>
				{course.cost !== "" && <Fact label="Cost">{course.cost}</Fact>}
				{/*
					The action is an icon here, so this is the only place
					the seat count is visible.
				*/}
				<Fact label="Capacity">
					{course.current_students}/
					{capacityLabel(course.max_students)}
				</Fact>
			</dl>

			{/*
				Always present and always one line tall, even when empty.
				It is the only part of the card whose content arrives
				later — the eligibility map lands after the write — so
				letting it size itself made every card in the row jump the
				moment a reason like "clashes with BB in MON1" appeared.
			*/}
			<p
				id={`${uid}-reasons`}
				title={note === "" ? undefined : note}
				className={`mt-auto line-clamp-1 min-h-5 text-sm ${noteColor}`}
			>
				{note}
			</p>
		</article>
	)
}
