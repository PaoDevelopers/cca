import { useId, type ReactElement } from "react"
import type { Category, Period } from "@common/types"
import type { CourseFilter } from "@/lib/enrollment"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

interface Props {
	filter: CourseFilter
	categories: Category[]
	periods: Period[]
	onchange: (filter: CourseFilter) => void
}

// The four availability switches, in the order they were in when this
// was a row of bare checkboxes.
const toggles = [
	{ key: "hideFull", label: "Hide full" },
	{ key: "hideInviteOnly", label: "Hide invite-only" },
	{ key: "hideIncompatible", label: "Hide incompatible" },
	{ key: "hideConflicting", label: "Hide conflicting" },
] as const

function Choice({
	id,
	checked,
	label,
	onchange,
}: {
	id: string
	checked: boolean
	label: string
	onchange: (checked: boolean) => void
}): ReactElement {
	return (
		<li className="flex items-center gap-2">
			<Checkbox
				id={id}
				checked={checked}
				onCheckedChange={(next): void => {
					onchange(next === true)
				}}
			/>
			<Label htmlFor={id} className="font-normal">
				{label}
			</Label>
		</li>
	)
}

// The filter rail. Periods first, because picking a slot in the week is
// how a student narrows a catalogue of forty courses, and it is what
// the left column held before.
export function Sidebar({
	filter,
	categories,
	periods,
	onchange,
}: Props): ReactElement {
	const uid = useId()
	const allPeriods = filter.periods.length === 0

	return (
		<aside className="w-full shrink-0 md:w-56 md:border-r md:pr-6">
			<nav className="flex flex-col gap-6">
				{/*
					Search sits at the top of the rail with the rest of
					the narrowing controls. Typing a name and ticking a
					period are the same kind of act, and they were in two
					different parts of the page.
				*/}
				<Input
					type="search"
					aria-label="Search"
					placeholder="Search CCAs..."
					className="h-9"
					value={filter.search}
					onChange={(event): void => {
						onchange({ ...filter, search: event.target.value })
					}}
				/>

				<div>
					<button
						className={cn(
							"mb-3 cursor-pointer text-sm font-medium",
							allPeriods
								? "text-primary"
								: "text-muted-foreground hover:text-foreground",
						)}
						aria-pressed={allPeriods}
						onClick={(): void => {
							onchange({ ...filter, periods: [] })
						}}
					>
						All periods
					</button>
					<ul className="flex list-none flex-col gap-2 p-0">
						{periods.map((period): ReactElement => (
							<Choice
								key={period.id}
								id={`${uid}-p-${period.id}`}
								checked={filter.periods.includes(period.id)}
								label={period.name}
								onchange={(checked): void => {
									onchange({
										...filter,
										periods: checked
											? [...filter.periods, period.id]
											: filter.periods.filter(
													(p): boolean =>
														p !== period.id,
												),
									})
								}}
							/>
						))}
					</ul>
				</div>

				{categories.length > 0 && (
					<div>
						<p className="mb-3 text-sm font-medium">Categories</p>
						<ul className="flex list-none flex-col gap-2 p-0">
							{categories.map((category): ReactElement => (
								<Choice
									key={category.id}
									id={`${uid}-c-${category.id}`}
									checked={filter.categories.includes(
										category.id,
									)}
									label={category.name}
									onchange={(checked): void => {
										onchange({
											...filter,
											categories: checked
												? [
														...filter.categories,
														category.id,
													]
												: filter.categories.filter(
														(c): boolean =>
															c !== category.id,
													),
										})
									}}
								/>
							))}
						</ul>
					</div>
				)}

				<div className="border-t pt-5">
					<ul className="flex list-none flex-col gap-2 p-0">
						{toggles.map(({ key, label }): ReactElement => (
							<Choice
								key={key}
								id={`${uid}-${key}`}
								checked={filter[key]}
								label={label}
								onchange={(checked): void => {
									onchange({ ...filter, [key]: checked })
								}}
							/>
						))}
					</ul>
				</div>
			</nav>
		</aside>
	)
}
