import { useId, type ReactElement } from "react"
import type { Category, Period } from "@common/types"
import type { CourseFilter } from "@/lib/enrollment"

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
			<input
				type="checkbox"
				id={id}
				className="size-4 shrink-0 accent-primary"
				checked={checked}
				onChange={(event): void => {
					onchange(event.currentTarget.checked)
				}}
			/>
			<label
				htmlFor={id}
				className="cursor-pointer select-none text-sm leading-none"
			>
				{label}
			</label>
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

	return (
		<aside className="w-full shrink-0 md:w-56 md:border-r md:pr-4">
			<nav className="flex flex-col gap-5">
				{/*
					Search sits at the top of the rail with the rest of
					the narrowing controls. Typing a name and ticking a
					period are the same kind of act, and they were in two
					different parts of the page.
				*/}
				<input
					type="search"
					aria-label="Search"
					placeholder="Search CCAs..."
					className="h-9 w-full min-w-0 rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 md:text-sm dark:bg-input/30"
					value={filter.search}
					onChange={(event): void => {
						onchange({ ...filter, search: event.target.value })
					}}
				/>

				<div>
					<p className="mb-3 text-sm font-medium">Periods</p>
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

				<div className="border-t pt-4">
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
