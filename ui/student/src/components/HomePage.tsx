import type { ReactElement, ReactNode } from "react"
import type { Category, Grade, StudentInfo } from "@common/types"
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

export function SelectionBudget({ user }: { user: StudentInfo }): ReactElement {
	const max = user.max_budgeted_periods
	const remaining = Math.max(0, (max ?? 0) - user.budgeted_periods_used)

	return (
		<div className="flex flex-col gap-4">
			<h2 className="text-lg font-semibold">Selection budget</h2>
			{max === null ? (
				<p className="text-sm text-muted-foreground">
					You may take as many courses as you like. You currently hold{" "}
					<strong>{user.budgeted_periods_used}</strong>.
				</p>
			) : (
				<div className="flex flex-col gap-2">
					<p className="text-sm text-muted-foreground">
						<span className="font-medium tabular-nums">
							{remaining} out of {max}
						</span>{" "}
						selections remaining
					</p>
					<Progress
						value={
							max === 0
								? 0
								: Math.min(100, (remaining / max) * 100)
						}
					/>
				</div>
			)}
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
// are judged against. Every result here comes from /student/api/user_info;
// the rules have one definition, and this is a view of it.
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
		<div className="flex flex-col gap-4">
			{user !== null && (
				<p className="text-2xl">
					{greeting(new Date().getHours())}, {firstName}
				</p>
			)}

			<div className="grid gap-4">
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
							<span
								data-slot="badge"
								className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${grade.is_open ? "bg-primary text-primary-foreground" : "bg-secondary text-secondary-foreground"}`}
							>
								{grade.is_open ? "Open" : "Closed"}
							</span>
						}
					>
						<div className="flex flex-col gap-4">
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
												? "text-sm"
												: "text-sm text-destructive"
										}
									>
										{allMet
											? "You have satisfied your requirements."
											: "You have not satisfied your requirements."}
									</p>
								</>
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
