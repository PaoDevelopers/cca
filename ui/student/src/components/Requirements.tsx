import type { ReactElement } from "react"
import type { Category, Grade, StudentInfo } from "@common/types"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"
import { Separator } from "@/components/ui/separator"
import { cn } from "@/lib/utils"

interface Props {
	user: StudentInfo | null
	grade: Grade | null
	categories: Category[]
}

interface RequirementItem {
	key: string
	label: string
	current: number
	required: number
	met: boolean
	kind: "period" | "category"
}

function plural(value: number, noun: string): string {
	return `${String(value)} ${noun}${value === 1 ? "" : "s"}`
}

function requirementItems({
	user,
	grade,
	categories,
}: Props): RequirementItem[] {
	if (user === null) {
		return []
	}

	const categoryName = new Map(
		categories.map((category): [string, string] => [
			category.id,
			category.name,
		]),
	)
	const items = user.requirements.map((progress): RequirementItem => {
		const rule = grade?.requirements.find(
			(requirement): boolean => requirement.id === progress.id,
		)
		const labels =
			rule?.category_ids.map(
				(id): string => categoryName.get(id) ?? id,
			) ?? []
		const label =
			labels.length > 0 ? labels.join(" / ") : "Category requirement"

		return {
			key: `requirement-${String(progress.id)}`,
			label,
			current: progress.satisfied_periods,
			required: progress.min_period_count,
			met: progress.met,
			kind: "period",
		}
	})

	if (user.min_distinct_categories > 0) {
		items.push({
			key: "distinct-categories",
			label: "Different categories",
			current: user.distinct_categories_used,
			required: user.min_distinct_categories,
			met: user.distinct_categories_used >= user.min_distinct_categories,
			kind: "category",
		})
	}

	return items
}

export function RequirementsOverview(props: Props): ReactElement {
	const items = requirementItems(props)

	return (
		<Card>
			<CardHeader>
				<CardTitle>
					<h2 className="text-lg">Requirements</h2>
				</CardTitle>
			</CardHeader>
			<CardContent>
				{items.length === 0 ? (
					<p className="text-sm text-muted-foreground">
						No additional requirements.
					</p>
				) : (
					<div className="flex flex-col">
						{items.map((item, index): ReactElement => (
							<div key={item.key}>
								<div
									className={cn(
										"flex items-baseline justify-between gap-4 py-3",
										index === 0 && "pt-0",
										index === items.length - 1 && "pb-0",
									)}
								>
									<p className="font-medium">{item.label}</p>
									<p className="shrink-0 text-sm text-muted-foreground tabular-nums">
										At least{" "}
										{plural(item.required, item.kind)}
									</p>
								</div>
								{index < items.length - 1 && <Separator />}
							</div>
						))}
					</div>
				)}
			</CardContent>
		</Card>
	)
}

export function RequirementsProgress(props: Props): ReactElement {
	const items = requirementItems(props)
	const complete = items.filter((item): boolean => item.met).length

	return (
		<div className="flex flex-col gap-6">
			<div className="flex flex-col gap-2">
				<h2 className="text-lg font-semibold">Requirements progress</h2>
				<p className="text-sm text-muted-foreground">
					{items.length === 0
						? "No additional requirements."
						: `${String(complete)} of ${String(items.length)} complete`}
				</p>
			</div>
			{items.length > 0 && (
				<div className="flex flex-col gap-5">
					{items.map((item, index): ReactElement => {
						const value =
							item.required === 0
								? 100
								: Math.min(
										100,
										(item.current / item.required) * 100,
									)
						return (
							<div key={item.key} className="flex flex-col gap-3">
								<div className="min-w-0">
									<p className="font-medium">{item.label}</p>
									<p className="mt-1 text-sm text-muted-foreground tabular-nums">
										{item.current} of {item.required}{" "}
										{item.kind}
										{item.required === 1 ? "" : "s"}
									</p>
								</div>
								<Progress
									value={value}
									aria-label={`${item.label}: ${String(item.current)} of ${String(item.required)} ${item.kind}${item.required === 1 ? "" : "s"}`}
								/>
								{index < items.length - 1 && <Separator />}
							</div>
						)
					})}
				</div>
			)}
		</div>
	)
}
