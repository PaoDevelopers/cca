import { useId, type ReactElement, type ReactNode } from "react"
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
	const note = state.status !== "" ? state.status : state.reasons.join("; ")
	const noteColor =
		state.status === "" && state.reasons.length > 0
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
				neighbours, and the grid row stretched to match. The full
				text stays on the element, so hovering gives it and it
				stays in the accessibility tree.
			*/}
			{course.description !== "" && (
				<p className="line-clamp-2 text-sm" title={course.description}>
					{course.description}
				</p>
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
