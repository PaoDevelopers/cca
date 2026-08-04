import { useMemo, useState } from "react"

import { adminRequest, jsonBody } from "@/api"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "@/components/ui/card"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import {
	Table,
	TableBody,
	TableCaption,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/ui/table"
import type { AdminPageProps } from "@/features/admin/app/AdminApp"
import {
	DeleteButton,
	NoResults,
	PageHeading,
} from "@/features/admin/components/AdminPagePrimitives"
import {
	countCoursesByCategory,
	countCoursesByPeriod,
	runMutation,
} from "@/features/admin/lib/page-utils"
import { CCA_DAYS, CCA_SLOTS_PER_DAY, ccaTimeSlotID } from "@/lib/cca-schedule"

interface NamedResourcePageProps extends AdminPageProps {
	resource: "categories"
	title: string
	description: string
	itemLabel: string
	items: readonly string[]
}

function NamedResourcePage({
	resource,
	title,
	description,
	itemLabel,
	items,
	data,
	refresh,
}: NamedResourcePageProps): React.JSX.Element {
	const [dialogOpen, setDialogOpen] = useState(false)
	const [value, setValue] = useState("")
	const [busy, setBusy] = useState(false)
	const courseUsage = useMemo(
		() => countCoursesByCategory(data.courses),
		[data.courses],
	)

	async function add(event: React.FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault()
		setBusy(true)
		const created = await runMutation(
			() =>
				adminRequest(`/${resource}`, {
					method: "POST",
					body: jsonBody({ id: value }),
				}),
			refresh,
			`${itemLabel} created.`,
		)
		setBusy(false)
		if (created) {
			setDialogOpen(false)
			setValue("")
		}
	}

	return (
		<>
			<PageHeading
				title={title}
				description={description}
				action={
					<Button onClick={() => setDialogOpen(true)}>
						Add {itemLabel.toLowerCase()}
					</Button>
				}
			/>

			<Card>
				<CardHeader>
					<CardTitle>{items.length} configured</CardTitle>
					<CardDescription>
						Delete is blocked when the value is still used by
						another record.
					</CardDescription>
				</CardHeader>
				<CardContent>
					{items.length === 0 ? (
						<NoResults
							title={`No ${resource} configured`}
							description={`Add the first ${itemLabel.toLowerCase()} to continue.`}
						/>
					) : (
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>{itemLabel}</TableHead>
									<TableHead>Usage</TableHead>
									<TableHead className="text-right">
										Actions
									</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{items.map((item) => {
									const usage = courseUsage.get(item) ?? 0
									return (
										<TableRow key={item}>
											<TableCell className="font-medium">
												{item}
											</TableCell>
											<TableCell>
												<Badge variant="outline">
													{usage} courses
												</Badge>
											</TableCell>
											<TableCell>
												<div className="flex justify-end">
													<DeleteButton
														name={item}
														disabled={usage > 0}
														description={
															usage > 0
																? `${item} is used by ${usage} courses and cannot be deleted.`
																: `Delete ${item} permanently.`
														}
														onDelete={() =>
															runMutation(
																() =>
																	adminRequest(
																		`/${resource}/${encodeURIComponent(item)}`,
																		{
																			method: "DELETE",
																		},
																	),
																refresh,
																`${itemLabel} deleted.`,
															)
														}
													/>
												</div>
											</TableCell>
										</TableRow>
									)
								})}
							</TableBody>
						</Table>
					)}
				</CardContent>
			</Card>

			<Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
				<DialogContent>
					<form onSubmit={(event) => void add(event)}>
						<DialogHeader>
							<DialogTitle>
								Add {itemLabel.toLowerCase()}
							</DialogTitle>
							<DialogDescription>
								Use the exact identifier shown to administrators
								and students.
							</DialogDescription>
						</DialogHeader>
						<FieldGroup className="py-5">
							<Field>
								<FieldLabel htmlFor={`${resource}-id`}>
									{itemLabel}
								</FieldLabel>
								<Input
									id={`${resource}-id`}
									value={value}
									onChange={(event) =>
										setValue(event.target.value)
									}
									required
									autoFocus
								/>
							</Field>
						</FieldGroup>
						<DialogFooter>
							<Button
								type="button"
								variant="outline"
								onClick={() => setDialogOpen(false)}
							>
								Cancel
							</Button>
							<Button
								type="submit"
								disabled={busy || value.trim() === ""}
							>
								{busy ? (
									<Spinner data-icon="inline-start" />
								) : null}
								Create
							</Button>
						</DialogFooter>
					</form>
				</DialogContent>
			</Dialog>
		</>
	)
}

export function PeriodsPage(props: AdminPageProps): React.JSX.Element {
	const configuredTimeSlots = useMemo(
		() => new Set(props.data.periods),
		[props.data.periods],
	)
	const courseUsage = useMemo(
		() => countCoursesByPeriod(props.data.courses),
		[props.data.courses],
	)

	return (
		<>
			<PageHeading
				title="Periods"
				description="The CCA week has exactly 16 system-defined slots: four slots per day from Monday to Thursday."
			/>

			<Card>
				<CardHeader>
					<CardTitle>Periods</CardTitle>
					<CardDescription>
						Each cell shows the permanent slot identifier and how
						many courses currently use it.
					</CardDescription>
				</CardHeader>
				<CardContent className="px-0">
					<Table className="min-w-[40rem] table-fixed">
						<TableCaption className="sr-only">
							The 16 fixed CCA time slots, arranged by weekday and
							slot number.
						</TableCaption>
						<TableHeader>
							<TableRow className="hover:bg-transparent">
								<TableHead className="sticky left-0 z-10 w-20 bg-muted text-center">
									Slot
								</TableHead>
								{CCA_DAYS.map((day) => (
									<TableHead
										key={day}
										className="border-l bg-muted/50 text-center"
									>
										{day}
									</TableHead>
								))}
							</TableRow>
						</TableHeader>
						<TableBody>
							{CCA_SLOTS_PER_DAY.map((slot) => (
								<TableRow
									key={slot}
									className="hover:bg-transparent"
								>
									<TableHead
										scope="row"
										className="sticky left-0 z-10 bg-muted text-center"
									>
										CCA {slot}
									</TableHead>
									{CCA_DAYS.map((day) => {
										const timeSlot = ccaTimeSlotID(
											day,
											slot,
										)
										const usage =
											courseUsage.get(timeSlot) ?? 0

										return (
											<TableCell
												key={timeSlot}
												className="border-l whitespace-normal"
											>
												<div className="flex flex-col gap-2">
													<span className="font-medium">
														{timeSlot}
													</span>
													<Badge
														variant={
															configuredTimeSlots.has(
																timeSlot,
															)
																? "outline"
																: "destructive"
														}
													>
														{configuredTimeSlots.has(
															timeSlot,
														)
															? `${usage} course${usage === 1 ? "" : "s"}`
															: "Missing from database"}
													</Badge>
												</div>
											</TableCell>
										)
									})}
								</TableRow>
							))}
						</TableBody>
					</Table>
				</CardContent>
			</Card>
		</>
	)
}

export function CategoriesPage(props: AdminPageProps): React.JSX.Element {
	return (
		<NamedResourcePage
			{...props}
			resource="categories"
			title="Course categories"
			description="Categories organise the catalogue and define grade requirement groups."
			itemLabel="Category"
			items={props.data.categories}
		/>
	)
}
