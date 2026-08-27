import type { ReactElement, ReactNode } from "react"
import { CircleAlert, CircleCheck } from "lucide-react"
import type { Category, Grade, StudentInfo } from "@common/types"
import { Badge } from "@/components/ui/badge"
import {
	Card,
	CardAction,
	CardContent,
	CardHeader,
	CardTitle,
} from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"
import { Separator } from "@/components/ui/separator"
import { RequirementsOverview } from "./Requirements"

interface Props {
	user: StudentInfo | null
	grade: Grade | null
	categories: Category[]
}

function Section({
	title,
	action,
	children,
}: {
	title: string
	action?: ReactElement
	children: ReactNode
}): ReactElement {
	return (
		<Card>
			<CardHeader>
				<CardTitle>
					<h2 className="text-lg">{title}</h2>
				</CardTitle>
				{action !== undefined && <CardAction>{action}</CardAction>}
			</CardHeader>
			<CardContent>{children}</CardContent>
		</Card>
	)
}

// A ratio as a labelled bar. Easier to read than a sentence, and the
// figure is still printed beside it for anyone who wants it exactly.
function Meter({
	label,
	value,
	of,
}: {
	label: string
	value: number
	of: number
}): ReactElement {
	return (
		<div className="flex flex-col gap-2">
			<div className="flex items-baseline justify-between gap-3 text-sm">
				<span className="text-muted-foreground">{label}</span>
				<span className="font-medium tabular-nums">
					{value} / {of}
				</span>
			</div>
			<Progress
				value={of === 0 ? 0 : Math.min(100, (value / of) * 100)}
			/>
		</div>
	)
}

function greeting(hour: number): string {
	if (hour < 12) {
		return "Good morning"
	}
	if (hour < 18) {
		return "Good afternoon"
	}
	return "Good evening"
}

// The landing view: where the student stands, and the requirements they
// are judged against. Every number here is read from the server, not
// derived in the browser: budget spent, categories spanned, and each
// requirement with whether it is met all come from
// /student/api/user_info. The rules have one definition, and this is a
// view of it.
export function HomePage({ user, grade, categories }: Props): ReactElement {
	const allMet =
		user !== null &&
		user.requirements.every((r): boolean => r.met) &&
		user.distinct_categories_used >= user.min_distinct_categories

	const showsVerdict =
		user !== null &&
		(user.requirements.length > 0 || user.min_distinct_categories > 0)

	// The first name only. The header already carries the full name, and
	// a second copy of it on the page is one more thing to read past —
	// a greeting is meant to be glanced at.
	const firstName =
		user === null ? "" : (user.name.split(" ")[0] ?? user.name)

	return (
		<div className="flex flex-col gap-6">
			{user !== null && (
				<p className="text-2xl">
					{greeting(new Date().getHours())}, {firstName}
				</p>
			)}

			<div className="grid gap-6 lg:grid-cols-2">
				{grade !== null && (
					<Section
						title="Status"
						/*
						Openness is the server's answer, derived from the
						window's two bounds. Nothing here recomputes it from
						the clock: the page and the write functions must
						agree, or a student is offered a button that cannot
						work.
					*/
						action={
							<Badge
								variant={
									grade.is_open ? "default" : "secondary"
								}
							>
								{grade.is_open ? "Open" : "Closed"}
							</Badge>
						}
					>
						<div className="flex flex-col gap-5">
							<p className="text-sm text-muted-foreground">
								{grade.is_open ? (
									<>
										Enrollment is open for your grade.
										{grade.closes_at !== null &&
											` It closes on ${new Date(
												grade.closes_at,
											).toLocaleString()}.`}
									</>
								) : (
									<>
										Enrollment is closed for your grade. You
										may not make any changes.
										{grade.opens_at !== null &&
											new Date(grade.opens_at) >
												new Date() &&
											` It opens on ${new Date(
												grade.opens_at,
											).toLocaleString()}.`}
									</>
								)}
							</p>

							{showsVerdict && (
								<>
									<Separator />
									<p
										className={
											allMet
												? "flex items-center gap-2 text-sm"
												: "flex items-center gap-2 text-sm text-destructive"
										}
									>
										{allMet ? (
											<CircleCheck
												className="size-4 shrink-0 text-primary"
												aria-hidden="true"
											/>
										) : (
											<CircleAlert
												className="size-4 shrink-0"
												aria-hidden="true"
											/>
										)}
										{allMet
											? "You have satisfied your requirements."
											: "You have not satisfied your requirements."}
									</p>
								</>
							)}
						</div>
					</Section>
				)}

				{user !== null && (
					<Section title="Available Selections">
						<div className="flex flex-col gap-5">
							{/*
							What is left, not what is spent. "Periods used"
							made a student subtract to answer the only
							question they were asking — how many more can I
							take — so the bar and the figure both count down.
						*/}
							{user.max_budgeted_periods === null ? (
								<p className="text-sm text-muted-foreground">
									You may take as many courses as you like.
									You currently hold{" "}
									<strong>
										{user.budgeted_periods_used}
									</strong>
									.
								</p>
							) : (
								<Meter
									label="Selections remaining"
									value={Math.max(
										0,
										user.max_budgeted_periods -
											user.budgeted_periods_used,
									)}
									of={user.max_budgeted_periods}
								/>
							)}

							{user.min_distinct_categories > 0 && (
								<Meter
									label="Categories spanned"
									value={user.distinct_categories_used}
									of={user.min_distinct_categories}
								/>
							)}
						</div>
					</Section>
				)}
			</div>

			{user !== null && (
				<RequirementsOverview
					user={user}
					grade={grade}
					categories={categories}
				/>
			)}
		</div>
	)
}
