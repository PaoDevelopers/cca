// The landing page: what needs attention, at a glance, with each
// figure a link to the rows behind it. Everything here is derived from
// data the other pages already load; there is no endpoint of its own.
import { capacityLabel, isFull } from "@common/capacity"
import { Progress } from "@common/components/ui/progress"
import type { Course } from "@common/types"
import { useEffect, type ReactElement, type ReactNode } from "react"
import { useAdminData } from "./data"
import { windowSummary } from "./GradeCard"

const q = (route: string, query: string): string =>
	`${route}?q=${encodeURIComponent(query)}`

// The courses closest to full, capped ones only: an uncapped course
// has no "full" to be close to, and an empty one is not close.
function fullest(courses: Course[], count: number): Course[] {
	const fill = (c: Course): number =>
		c.max_students === 0 ? 1 : c.current_students / (c.max_students ?? 1)
	return courses
		.filter(
			(c): boolean => c.max_students !== null && c.current_students > 0,
		)
		.sort((a, b): number => fill(b) - fill(a))
		.slice(0, count)
}

function standing(total: number, short: number): string {
	if (total === 0) {
		return "no students"
	}
	if (short === 0) {
		return `all ${String(total)} met`
	}
	return `${String(short)} of ${String(total)} still short`
}

export function HomePage(): ReactElement {
	const data = useAdminData()

	useEffect((): void => {
		data.want("students", "grades", "courses", "enrollments")
	}, [data])

	if (!data.ready("students", "grades", "courses", "enrollments")) {
		return <p className="text-muted-foreground">Loading…</p>
	}

	const openGrades = data.grades.filter((g): boolean => g.is_open)
	const fullCourses = data.courses.filter((c): boolean => isFull(c))
	const enrolled = new Set(data.enrollments.map((e): string => e.student_id))
	const unmet = new Set(
		data.studentStatus
			.filter((s): boolean => !s.requirements_met)
			.map((s): string => s.student_id),
	)

	return (
		<section aria-labelledby="home-heading" className="flex flex-col gap-6">
			<div>
				<h2
					id="home-heading"
					tabIndex={-1}
					className="text-2xl font-semibold tracking-tight outline-none"
				>
					Overview
				</h2>
				<p className="text-sm text-muted-foreground">
					Where enrollment stands right now.
				</p>
			</div>

			<div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
				<Stat
					href="#/grades"
					label="Enrollment open"
					value={`${String(openGrades.length)} of ${String(data.grades.length)}`}
					note={
						openGrades.length === 0
							? "No grade can enroll"
							: `grades: ${openGrades.map((g) => g.id).join(", ")}`
					}
					live={openGrades.length > 0}
				/>
				<Stat
					href="#/students"
					label="Students"
					value={String(data.students.length)}
					note={`${String(data.students.length - enrolled.size)} with no enrollments`}
				/>
				<Stat
					href="#/enrollments"
					label="Enrollments"
					value={String(data.enrollments.length)}
					note={`${String(enrolled.size)} students placed`}
				/>
				<Stat
					href="#/courses"
					label="Courses"
					value={String(data.courses.length)}
					note={`${String(fullCourses.length)} full`}
				/>
			</div>

			<div className="grid gap-4 lg:grid-cols-2">
				<Panel title="Enrollment windows" href="#/grades">
					{data.grades.length === 0 ? (
						<Empty>No grades yet.</Empty>
					) : (
						<ul className="divide-y">
							{data.grades.map((g) => (
								<li key={g.id}>
									<a
										href="#/grades"
										className="flex items-start gap-3 py-2.5 hover:text-primary"
									>
										<span
											aria-hidden="true"
											className={
												g.is_open
													? "mt-1.5 size-2 shrink-0 rounded-full bg-primary"
													: "mt-1.5 size-2 shrink-0 rounded-full bg-muted-foreground/40"
											}
										/>
										<span className="w-12 shrink-0 font-medium">
											{g.id}
										</span>
										<span className="text-sm text-muted-foreground">
											{windowSummary(g)}
										</span>
									</a>
								</li>
							))}
						</ul>
					)}
				</Panel>

				<Panel title="Requirements met, by grade" href="#/students">
					{data.grades.length === 0 ? (
						<Empty>No grades yet.</Empty>
					) : (
						<ul className="flex flex-col gap-3 py-1">
							{data.grades.map((g) => {
								const ids = data.students
									.filter((s): boolean => s.grade_id === g.id)
									.map((s): string => s.id)
								const short = ids.filter((id): boolean =>
									unmet.has(id),
								).length
								const met = ids.length - short
								const percent =
									ids.length === 0
										? 0
										: Math.round((met / ids.length) * 100)
								return (
									<li key={g.id}>
										<a
											href={q(
												"#/students",
												`grade:${g.id} requirements:Unsatisfied`,
											)}
											className="group flex flex-col gap-1.5"
										>
											<span className="flex items-baseline justify-between gap-2 text-sm">
												<span className="font-medium group-hover:text-primary">
													{g.id}
												</span>
												<span className="text-muted-foreground tabular-nums">
													{standing(
														ids.length,
														short,
													)}
												</span>
											</span>
											<Progress
												value={percent}
												aria-label={`${g.id}: ${String(percent)}% meet their requirements`}
											/>
										</a>
									</li>
								)
							})}
						</ul>
					)}
				</Panel>

				<Panel
					title="Fullest courses"
					href="#/courses"
					className="lg:col-span-2"
				>
					{fullest(data.courses, 8).length === 0 ? (
						<Empty>No capped course has anyone in it yet.</Empty>
					) : (
						<ul className="grid gap-x-8 gap-y-3 py-1 md:grid-cols-2">
							{fullest(data.courses, 8).map((c) => {
								const max = c.max_students ?? 0
								const percent =
									max === 0
										? 100
										: Math.min(
												100,
												Math.round(
													(c.current_students / max) *
														100,
												),
											)
								return (
									<li key={c.id}>
										<a
											href={q("#/courses", `id:${c.id}`)}
											className="group flex flex-col gap-1.5"
										>
											<span className="flex items-baseline justify-between gap-2 text-sm">
												<span className="truncate group-hover:text-primary">
													<span className="font-mono font-medium">
														{c.id}
													</span>{" "}
													<span className="text-muted-foreground">
														{c.name}
													</span>
												</span>
												<span className="shrink-0 tabular-nums">
													{c.current_students}/
													{capacityLabel(
														c.max_students,
													)}
												</span>
											</span>
											<Progress
												value={percent}
												aria-label={`${c.id}: ${String(c.current_students)} of ${String(max)} places taken`}
											/>
										</a>
									</li>
								)
							})}
						</ul>
					)}
				</Panel>
			</div>
		</section>
	)
}

function Stat({
	href,
	label,
	value,
	note,
	live,
}: {
	href: string
	label: string
	value: string
	note: string
	live?: boolean
}): ReactElement {
	return (
		<a
			href={href}
			className="flex flex-col gap-1 rounded-xl border bg-card p-4 shadow-xs transition-colors hover:border-ring/60 hover:bg-accent/40"
		>
			<span className="flex items-center gap-2 text-sm text-muted-foreground">
				{live !== undefined && (
					<span
						aria-hidden="true"
						className={
							live
								? "size-2 rounded-full bg-primary"
								: "size-2 rounded-full bg-muted-foreground/40"
						}
					/>
				)}
				{label}
			</span>
			<span className="text-3xl font-semibold tracking-tight tabular-nums">
				{value}
			</span>
			<span className="truncate text-xs text-muted-foreground">
				{note}
			</span>
		</a>
	)
}

function Panel({
	title,
	href,
	className,
	children,
}: {
	title: string
	href: string
	className?: string
	children: ReactNode
}): ReactElement {
	return (
		<div
			className={`flex flex-col gap-2 rounded-xl border bg-card p-5 shadow-xs ${className ?? ""}`}
		>
			<div className="flex items-baseline justify-between gap-2">
				<h3 className="font-semibold">{title}</h3>
				<a
					href={href}
					className="text-sm text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
				>
					View all
				</a>
			</div>
			{children}
		</div>
	)
}

function Empty({ children }: { children: ReactNode }): ReactElement {
	return <p className="py-2 text-sm text-muted-foreground">{children}</p>
}
