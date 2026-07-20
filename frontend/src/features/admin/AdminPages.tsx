import { useMemo, useState } from "react"
import {
	BellRingIcon,
	BookOpenIcon,
	CalendarDaysIcon,
	CheckCircle2Icon,
	DownloadIcon,
	InboxIcon,
	LockKeyholeIcon,
	PencilIcon,
	PlusIcon,
	SaveIcon,
	SearchIcon,
	SendIcon,
	Trash2Icon,
	UploadIcon,
	UsersIcon,
} from "lucide-react"
import { toast } from "sonner"

import { apiRequest, jsonBody } from "@/api"
import { PeriodBadges } from "@/components/common"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogMedia,
	AlertDialogTitle,
	AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
	Card,
	CardAction,
	CardContent,
	CardDescription,
	CardFooter,
	CardHeader,
	CardTitle,
} from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import {
	Empty,
	EmptyDescription,
	EmptyHeader,
	EmptyMedia,
	EmptyTitle,
} from "@/components/ui/empty"
import {
	Field,
	FieldContent,
	FieldDescription,
	FieldGroup,
	FieldLabel,
	FieldLegend,
	FieldSet,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "@/components/ui/input-group"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import {
	Table,
	TableBody,
	TableCaption,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import type { AdminPageProps } from "@/features/admin/AdminApp"
import { useSearchFilter } from "@/hooks/use-search-filter"
import {
	CCA_DAYS,
	CCA_SLOTS_PER_DAY,
	ccaTimeSlotID,
	FIXED_CCA_TIME_SLOTS,
} from "@/lib/cca-schedule"
import type {
	Course,
	CoursePayload,
	Grade,
	LegalSex,
	MembershipType,
	Selection,
	SelectionType,
	Student,
} from "@/types"

function PageHeading({
	title,
	description,
	action,
}: {
	title: string
	description: string
	action?: React.ReactNode
}): React.JSX.Element {
	return (
		<div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
			<div className="flex flex-col gap-1">
				<h2 className="font-heading text-xl font-semibold tracking-tight">
					{title}
				</h2>
				<p className="max-w-3xl text-sm text-muted-foreground">
					{description}
				</p>
			</div>
			{action}
		</div>
	)
}

function SearchBox({
	value,
	onChange,
	placeholder,
}: {
	value: string
	onChange: (value: string) => void
	placeholder: string
}): React.JSX.Element {
	return (
		<InputGroup className="h-11 max-w-md">
			<InputGroupAddon>
				<SearchIcon aria-hidden="true" />
			</InputGroupAddon>
			<InputGroupInput
				type="search"
				name="admin_search"
				value={value}
				onChange={(event) => onChange(event.target.value)}
				placeholder={`${placeholder}…`}
				autoComplete="off"
				spellCheck={false}
				aria-label={placeholder}
			/>
		</InputGroup>
	)
}

function NoResults({
	title,
	description,
}: {
	title: string
	description: string
}): React.JSX.Element {
	return (
		<Empty>
			<EmptyHeader>
				<EmptyMedia variant="icon">
					<InboxIcon aria-hidden="true" />
				</EmptyMedia>
				<EmptyTitle>{title}</EmptyTitle>
				<EmptyDescription>{description}</EmptyDescription>
			</EmptyHeader>
		</Empty>
	)
}

function domID(prefix: string, value: string | number): string {
	return `${prefix}-${encodeURIComponent(String(value)).replaceAll("%", "")}`
}

function getCourseSearchText(course: Course): string {
	return `${course.id} ${course.name} ${course.teacher} ${course.location} ${course.category_id}`
}

function getStudentSearchText(student: Student): string {
	return `${student.id} ${student.name} ${student.grade}`
}

function getSelectionSearchText(selection: Selection): string {
	return `${selection.student_id ?? ""} ${selection.student_name ?? ""} ${selection.course_id} ${selection.course_name ?? ""}`
}

function incrementCount<Key extends string | number>(
	counts: Map<Key, number>,
	key: Key,
): void {
	counts.set(key, (counts.get(key) ?? 0) + 1)
}

function countCoursesByCategory(
	courses: readonly Course[],
): ReadonlyMap<string, number> {
	const counts = new Map<string, number>()
	for (const course of courses) incrementCount(counts, course.category_id)
	return counts
}

function countCoursesByPeriod(
	courses: readonly Course[],
): ReadonlyMap<string, number> {
	const counts = new Map<string, number>()
	for (const course of courses) {
		for (const periodID of course.period_ids) {
			incrementCount(counts, periodID)
		}
	}
	return counts
}

function countSelectionsByStudent(
	selections: readonly Selection[],
): ReadonlyMap<number, number> {
	const counts = new Map<number, number>()
	for (const selection of selections) {
		if (selection.student_id !== undefined) {
			incrementCount(counts, selection.student_id)
		}
	}
	return counts
}

async function runMutation(
	request: () => Promise<unknown>,
	refresh: () => Promise<void>,
	successMessage: string,
): Promise<boolean> {
	try {
		await request()
		await refresh()
		toast.success(successMessage)
		return true
	} catch (caught) {
		toast.error(
			caught instanceof Error ? caught.message : "The request failed.",
		)
		return false
	}
}

function DeleteButton({
	name,
	description,
	onDelete,
	disabled = false,
}: {
	name: string
	description?: string
	onDelete: () => Promise<boolean>
	disabled?: boolean
}): React.JSX.Element {
	const [open, setOpen] = useState(false)
	const [busy, setBusy] = useState(false)

	async function remove(): Promise<void> {
		setBusy(true)
		const deleted = await onDelete()
		setBusy(false)
		if (deleted) setOpen(false)
	}

	return (
		<AlertDialog open={open} onOpenChange={setOpen}>
			<AlertDialogTrigger
				render={
					<Button
						variant="ghost"
						size="icon-sm"
						disabled={disabled}
					/>
				}
			>
				<Trash2Icon aria-hidden="true" />
				<span className="sr-only">Delete {name}</span>
			</AlertDialogTrigger>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogMedia>
						<Trash2Icon aria-hidden="true" />
					</AlertDialogMedia>
					<AlertDialogTitle>Delete {name}?</AlertDialogTitle>
					<AlertDialogDescription>
						{description ??
							"This action cannot be undone. Related records may prevent deletion."}
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel disabled={busy}>
						Cancel
					</AlertDialogCancel>
					<AlertDialogAction
						variant="destructive"
						disabled={busy}
						onClick={() => void remove()}
					>
						{busy ? (
							<Spinner data-icon="inline-start" />
						) : (
							<Trash2Icon data-icon="inline-start" />
						)}
						Delete
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	)
}

function StatCard({
	icon: Icon,
	label,
	value,
	description,
}: {
	icon: React.ComponentType<React.SVGProps<SVGSVGElement>>
	label: string
	value: number
	description: string
}): React.JSX.Element {
	return (
		<Card>
			<CardHeader>
				<CardTitle>{label}</CardTitle>
				<CardDescription>{description}</CardDescription>
				<CardAction>
					<div className="flex size-9 items-center justify-center rounded-lg bg-muted">
						<Icon aria-hidden="true" />
					</div>
				</CardAction>
			</CardHeader>
			<CardContent>
				<p className="font-heading text-3xl font-semibold tabular-nums">
					{value}
				</p>
			</CardContent>
		</Card>
	)
}

export function DashboardPage({ data }: AdminPageProps): React.JSX.Element {
	const unassigned = data.courses.filter(
		(course) => course.period_ids.length === 0,
	).length
	const normalSelections = data.selections.filter(
		(selection) => selection.selection_type === "normal",
	).length

	return (
		<>
			<PageHeading
				title="System overview"
				description="A live summary of timetable configuration, courses, students, and selections."
			/>
			<div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
				<StatCard
					icon={BookOpenIcon}
					label="Courses"
					value={data.courses.length}
					description={`${unassigned} without a timetable`}
				/>
				<StatCard
					icon={UsersIcon}
					label="Students"
					value={data.students.length}
					description="Registered student profiles"
				/>
				<StatCard
					icon={CheckCircle2Icon}
					label="Selections"
					value={data.selections.length}
					description={`${normalSelections} student choices`}
				/>
				<StatCard
					icon={CalendarDaysIcon}
					label="Time slots"
					value={data.periods.length}
					description="Fixed atomic timetable slots"
				/>
			</div>

			<div className="mt-4 grid gap-4 xl:grid-cols-2">
				<Card>
					<CardHeader>
						<CardTitle>Course capacity</CardTitle>
						<CardDescription>
							Courses nearest to their configured limit.
						</CardDescription>
					</CardHeader>
					<CardContent className="flex flex-col gap-3">
						{data.courses.length === 0 ? (
							<NoResults
								title="No courses yet"
								description="Create a course to begin."
							/>
						) : (
							[...data.courses]
								.sort((a, b) => {
									const aRatio =
										a.max_students === 0
											? 0
											: a.current_students /
												a.max_students
									const bRatio =
										b.max_students === 0
											? 0
											: b.current_students /
												b.max_students
									return bRatio - aRatio
								})
								.slice(0, 6)
								.map((course) => (
									<div
										key={course.id}
										className="flex items-center justify-between gap-4"
									>
										<div className="min-w-0">
											<p className="truncate font-medium">
												{course.name}
											</p>
											<p className="truncate text-xs text-muted-foreground">
												{course.id}
											</p>
										</div>
										<Badge
											variant={
												course.current_students >=
												course.max_students
													? "destructive"
													: "outline"
											}
										>
											{course.current_students}/
											{course.max_students}
										</Badge>
									</div>
								))
						)}
					</CardContent>
				</Card>

				<Card>
					<CardHeader>
						<CardTitle>Grade settings</CardTitle>
						<CardDescription>
							Selection access and own-choice limits by grade.
						</CardDescription>
					</CardHeader>
					<CardContent className="flex flex-col gap-3">
						{data.grades.length === 0 ? (
							<NoResults
								title="No grades yet"
								description="Add a grade before importing students."
							/>
						) : (
							data.grades.map((grade) => (
								<div
									key={grade.grade}
									className="flex items-center justify-between gap-4"
								>
									<span className="font-medium">
										{grade.grade}
									</span>
									<div className="flex items-center gap-2">
										<Badge variant="outline">
											Max {grade.max_own_choices}
										</Badge>
										<Badge
											variant={
												grade.enabled
													? "secondary"
													: "outline"
											}
										>
											{grade.enabled ? "Open" : "Closed"}
										</Badge>
									</div>
								</div>
							))
						)}
					</CardContent>
				</Card>
			</div>
		</>
	)
}

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
				apiRequest(`/api/v1/admin/${resource}`, {
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
						<PlusIcon data-icon="inline-start" />
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
																	apiRequest(
																		`/api/v1/admin/${resource}/${encodeURIComponent(item)}`,
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
								) : (
									<PlusIcon data-icon="inline-start" />
								)}
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
				title="Fixed time slots"
				description="The CCA week has exactly 16 system-defined slots: four slots per day from Monday to Thursday."
			/>

			<Alert className="mb-6">
				<LockKeyholeIcon />
				<AlertTitle>Read-only system schedule</AlertTitle>
				<AlertDescription>
					Time slots cannot be created or deleted here. A course may
					occupy multiple slots, and sharing any slot creates a
					schedule conflict.
				</AlertDescription>
			</Alert>

			<Card>
				<CardHeader>
					<CardTitle>
						{FIXED_CCA_TIME_SLOTS.length} fixed slots
					</CardTitle>
					<CardDescription>
						Each cell shows the permanent slot identifier and how
						many courses currently use it.
					</CardDescription>
				</CardHeader>
				<CardContent className="px-0">
					<Table className="min-w-[58rem] table-fixed">
						<TableCaption className="sr-only">
							The 16 fixed CCA time slots, arranged by weekday and
							slot number.
						</TableCaption>
						<TableHeader>
							<TableRow className="hover:bg-transparent">
								<TableHead className="sticky left-0 z-10 w-24 bg-muted text-center">
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

function GradeDialog({
	open,
	onOpenChange,
	refresh,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	refresh: () => Promise<void>
}): React.JSX.Element {
	const [grade, setGrade] = useState("")
	const [maxChoices, setMaxChoices] = useState("1")
	const [enabled, setEnabled] = useState(false)
	const [busy, setBusy] = useState(false)

	async function submit(
		event: React.FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault()
		setBusy(true)
		const created = await runMutation(
			() =>
				apiRequest("/api/v1/admin/grades", {
					method: "POST",
					body: jsonBody({
						grade,
						enabled,
						max_own_choices: Number(maxChoices),
					}),
				}),
			refresh,
			"Grade created.",
		)
		setBusy(false)
		if (created) {
			onOpenChange(false)
			setGrade("")
			setMaxChoices("1")
			setEnabled(false)
		}
	}

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent>
				<form onSubmit={(event) => void submit(event)}>
					<DialogHeader>
						<DialogTitle>Add grade</DialogTitle>
						<DialogDescription>
							Configure whether students can select and how many
							own choices they may make.
						</DialogDescription>
					</DialogHeader>
					<FieldGroup className="py-5">
						<Field>
							<FieldLabel htmlFor="new-grade">Grade</FieldLabel>
							<Input
								id="new-grade"
								value={grade}
								onChange={(event) =>
									setGrade(event.target.value)
								}
								required
								autoFocus
							/>
						</Field>
						<Field>
							<FieldLabel htmlFor="new-grade-limit">
								Maximum own choices
							</FieldLabel>
							<Input
								id="new-grade-limit"
								type="number"
								min="0"
								value={maxChoices}
								onChange={(event) =>
									setMaxChoices(event.target.value)
								}
								required
							/>
						</Field>
						<Field orientation="horizontal">
							<Switch
								id="new-grade-enabled"
								checked={enabled}
								onCheckedChange={setEnabled}
							/>
							<FieldContent>
								<FieldLabel htmlFor="new-grade-enabled">
									Open student selections
								</FieldLabel>
								<FieldDescription>
									Students in this grade can make normal
									choices.
								</FieldDescription>
							</FieldContent>
						</Field>
					</FieldGroup>
					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							onClick={() => onOpenChange(false)}
						>
							Cancel
						</Button>
						<Button
							type="submit"
							disabled={
								busy ||
								grade.trim() === "" ||
								Number(maxChoices) < 0
							}
						>
							{busy ? (
								<Spinner data-icon="inline-start" />
							) : (
								<PlusIcon data-icon="inline-start" />
							)}
							Create
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}

function RequirementDialog({
	grade,
	categories,
	open,
	onOpenChange,
	refresh,
}: {
	grade: string
	categories: readonly string[]
	open: boolean
	onOpenChange: (open: boolean) => void
	refresh: () => Promise<void>
}): React.JSX.Element {
	const [minimum, setMinimum] = useState("1")
	const [selected, setSelected] = useState<string[]>([])
	const [busy, setBusy] = useState(false)

	function changeOpen(nextOpen: boolean): void {
		if (!nextOpen) {
			setMinimum("1")
			setSelected([])
		}
		onOpenChange(nextOpen)
	}

	function toggle(category: string, checked: boolean): void {
		setSelected((current) =>
			checked
				? [...current, category]
				: current.filter((item) => item !== category),
		)
	}

	async function submit(
		event: React.FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault()
		setBusy(true)
		const created = await runMutation(
			() =>
				apiRequest(
					`/api/v1/admin/grades/${encodeURIComponent(grade)}/requirement-groups`,
					{
						method: "POST",
						body: jsonBody({
							min_count: Number(minimum),
							category_ids: selected,
						}),
					},
				),
			refresh,
			"Requirement group created.",
		)
		setBusy(false)
		if (created) changeOpen(false)
	}

	return (
		<Dialog open={open} onOpenChange={changeOpen}>
			<DialogContent>
				<form onSubmit={(event) => void submit(event)}>
					<DialogHeader>
						<DialogTitle>Add requirement for {grade}</DialogTitle>
						<DialogDescription>
							Students must select the minimum number from any of
							the chosen categories.
						</DialogDescription>
					</DialogHeader>
					<FieldGroup className="py-5">
						<Field>
							<FieldLabel htmlFor={`requirement-min-${grade}`}>
								Minimum selections
							</FieldLabel>
							<Input
								id={`requirement-min-${grade}`}
								type="number"
								min="0"
								value={minimum}
								onChange={(event) =>
									setMinimum(event.target.value)
								}
								required
							/>
						</Field>
						<FieldSet>
							<FieldLegend variant="label">
								Categories
							</FieldLegend>
							<FieldGroup data-slot="checkbox-group">
								{categories.map((category) => (
									<Field
										key={category}
										orientation="horizontal"
									>
										<Checkbox
											id={domID(
												`requirement-${grade}`,
												category,
											)}
											checked={selected.includes(
												category,
											)}
											onCheckedChange={(checked) =>
												toggle(category, checked)
											}
										/>
										<FieldLabel
											htmlFor={domID(
												`requirement-${grade}`,
												category,
											)}
										>
											{category}
										</FieldLabel>
									</Field>
								))}
							</FieldGroup>
						</FieldSet>
					</FieldGroup>
					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							onClick={() => changeOpen(false)}
						>
							Cancel
						</Button>
						<Button
							type="submit"
							disabled={
								busy ||
								selected.length === 0 ||
								Number(minimum) < 0
							}
						>
							{busy ? (
								<Spinner data-icon="inline-start" />
							) : (
								<PlusIcon data-icon="inline-start" />
							)}
							Create
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}

function GradeCard({
	grade,
	categories,
	refresh,
}: {
	grade: Grade
	categories: readonly string[]
	refresh: () => Promise<void>
}): React.JSX.Element {
	const [limit, setLimit] = useState(String(grade.max_own_choices))
	const [requirementOpen, setRequirementOpen] = useState(false)
	const [busy, setBusy] = useState(false)

	async function save(settings: {
		enabled: boolean
		max: number
	}): Promise<boolean> {
		setBusy(true)
		const updated = await runMutation(
			() =>
				apiRequest(
					`/api/v1/admin/grades/${encodeURIComponent(grade.grade)}`,
					{
						method: "PUT",
						body: jsonBody({
							grade: grade.grade,
							enabled: settings.enabled,
							max_own_choices: settings.max,
						}),
					},
				),
			refresh,
			`${grade.grade} settings saved.`,
		)
		setBusy(false)
		return updated
	}

	return (
		<Card>
			<CardHeader>
				<CardTitle>{grade.grade}</CardTitle>
				<CardDescription>
					{grade.enabled
						? "Student selections are open."
						: "Student selections are closed."}
				</CardDescription>
				<CardAction>
					<Switch
						aria-label={`Enable selections for ${grade.grade}`}
						checked={grade.enabled}
						disabled={busy}
						onCheckedChange={(enabled) =>
							void save({ enabled, max: grade.max_own_choices })
						}
					/>
				</CardAction>
			</CardHeader>
			<CardContent className="flex flex-col gap-4">
				<FieldGroup>
					<Field>
						<FieldLabel htmlFor={`limit-${grade.grade}`}>
							Maximum own choices
						</FieldLabel>
						<div className="flex gap-2">
							<Input
								id={`limit-${grade.grade}`}
								className="max-w-32"
								type="number"
								min="0"
								value={limit}
								onChange={(event) =>
									setLimit(event.target.value)
								}
							/>
							<Button
								variant="outline"
								disabled={
									busy ||
									Number(limit) < 0 ||
									Number(limit) === grade.max_own_choices
								}
								onClick={() =>
									void save({
										enabled: grade.enabled,
										max: Number(limit),
									})
								}
							>
								{busy ? (
									<Spinner data-icon="inline-start" />
								) : (
									<SaveIcon data-icon="inline-start" />
								)}
								Save
							</Button>
						</div>
					</Field>
				</FieldGroup>
				<Separator />
				<div className="flex items-center justify-between gap-3">
					<div>
						<p className="font-medium">Requirement groups</p>
						<p className="text-xs text-muted-foreground">
							Minimum selections across a set of categories.
						</p>
					</div>
					<Button
						size="sm"
						variant="outline"
						disabled={categories.length === 0}
						onClick={() => setRequirementOpen(true)}
					>
						<PlusIcon data-icon="inline-start" />
						Add
					</Button>
				</div>
				{grade.req_groups.length === 0 ? (
					<p className="text-sm text-muted-foreground">
						No requirements configured.
					</p>
				) : (
					<div className="flex flex-col gap-2">
						{grade.req_groups.map((group) => (
							<div
								key={group.id}
								className="flex items-center justify-between gap-3 rounded-lg bg-muted/50 p-2.5"
							>
								<div className="min-w-0">
									<p className="text-sm font-medium">
										Choose {group.min_count}
									</p>
									<p className="truncate text-xs text-muted-foreground">
										{group.category_ids.join(" / ")}
									</p>
								</div>
								<DeleteButton
									name="requirement group"
									onDelete={() =>
										runMutation(
											() =>
												apiRequest(
													`/api/v1/admin/grades/${encodeURIComponent(grade.grade)}/requirement-groups/${group.id}`,
													{ method: "DELETE" },
												),
											refresh,
											"Requirement group deleted.",
										)
									}
								/>
							</div>
						))}
					</div>
				)}
			</CardContent>
			<CardFooter className="justify-end">
				<DeleteButton
					name={grade.grade}
					description="Students and course restrictions may prevent this grade from being deleted."
					onDelete={() =>
						runMutation(
							() =>
								apiRequest(
									`/api/v1/admin/grades/${encodeURIComponent(grade.grade)}`,
									{ method: "DELETE" },
								),
							refresh,
							"Grade deleted.",
						)
					}
				/>
			</CardFooter>
			<RequirementDialog
				grade={grade.grade}
				categories={categories}
				open={requirementOpen}
				onOpenChange={setRequirementOpen}
				refresh={refresh}
			/>
		</Card>
	)
}

export function GradesPage({
	data,
	refresh,
}: AdminPageProps): React.JSX.Element {
	const [dialogOpen, setDialogOpen] = useState(false)
	return (
		<>
			<PageHeading
				title="Grade access"
				description="Control sign-up availability, choice limits, and category requirements for each grade."
				action={
					<Button onClick={() => setDialogOpen(true)}>
						<PlusIcon data-icon="inline-start" />
						Add grade
					</Button>
				}
			/>
			{data.grades.length === 0 ? (
				<Card>
					<CardContent>
						<NoResults
							title="No grades configured"
							description="Add a grade to configure student access."
						/>
					</CardContent>
				</Card>
			) : (
				<div className="grid gap-4 lg:grid-cols-2 xl:grid-cols-3">
					{data.grades.map((grade) => (
						<GradeCard
							key={`${grade.grade}-${grade.max_own_choices}-${grade.enabled}`}
							grade={grade}
							categories={data.categories}
							refresh={refresh}
						/>
					))}
				</div>
			)}
			<GradeDialog
				open={dialogOpen}
				onOpenChange={setDialogOpen}
				refresh={refresh}
			/>
		</>
	)
}

function toggleValue<T extends string>(
	values: T[],
	value: T,
	checked: boolean,
): T[] {
	return checked
		? [...values, value]
		: values.filter((item) => item !== value)
}

function initialCoursePayload(
	course: Course | null,
	data: AdminPageProps["data"],
): CoursePayload {
	return course === null
		? {
				id: "",
				name: "",
				description: "",
				period_ids: [],
				max_students: 20,
				membership: "free",
				teacher: "",
				location: "",
				category_id: data.categories[0] ?? "",
				allowed_legal_sexes: [],
				allowed_grades: [],
			}
		: {
				id: course.id,
				name: course.name,
				description: course.description,
				period_ids: [...course.period_ids],
				max_students: course.max_students,
				membership: course.membership,
				teacher: course.teacher,
				location: course.location,
				category_id: course.category_id,
				allowed_legal_sexes: [...course.allowed_legal_sexes],
				allowed_grades: [...course.allowed_grades],
			}
}

function CourseDialog({
	course,
	data,
	open,
	onOpenChange,
	refresh,
}: {
	course: Course | null
	data: AdminPageProps["data"]
	open: boolean
	onOpenChange: (open: boolean) => void
	refresh: () => Promise<void>
}): React.JSX.Element {
	const [form, setForm] = useState<CoursePayload>(() =>
		initialCoursePayload(course, data),
	)
	const [busy, setBusy] = useState(false)
	const scheduleLocked = course !== null && course.current_students > 0

	function update<K extends keyof CoursePayload>(
		key: K,
		value: CoursePayload[K],
	): void {
		setForm((current) => ({ ...current, [key]: value }))
	}

	async function submit(
		event: React.FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault()
		setBusy(true)
		const saved = await runMutation(
			() =>
				apiRequest(
					course === null
						? "/api/v1/admin/courses"
						: `/api/v1/admin/courses/${encodeURIComponent(course.id)}`,
					{
						method: course === null ? "POST" : "PUT",
						body: jsonBody(form),
					},
				),
			refresh,
			course === null ? "Course created." : "Course updated.",
		)
		setBusy(false)
		if (saved) onOpenChange(false)
	}

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto overscroll-contain sm:max-w-3xl">
				<form onSubmit={(event) => void submit(event)}>
					<DialogHeader>
						<DialogTitle>
							{course === null
								? "Create course"
								: `Edit ${course.name}`}
						</DialogTitle>
						<DialogDescription>
							A course may occupy multiple fixed time slots. Any
							shared slot will prevent another selection.
						</DialogDescription>
					</DialogHeader>
					<FieldGroup className="py-5">
						<FieldGroup className="grid gap-4 sm:grid-cols-2">
							<Field data-disabled={course !== null}>
								<FieldLabel htmlFor="course-id">
									Course ID
								</FieldLabel>
								<Input
									id="course-id"
									value={form.id}
									onChange={(event) =>
										update("id", event.target.value)
									}
									disabled={course !== null}
									required
								/>
							</Field>
							<Field>
								<FieldLabel htmlFor="course-name">
									Name
								</FieldLabel>
								<Input
									id="course-name"
									value={form.name}
									onChange={(event) =>
										update("name", event.target.value)
									}
									required
								/>
							</Field>
							<Field>
								<FieldLabel htmlFor="course-teacher">
									Teacher
								</FieldLabel>
								<Input
									id="course-teacher"
									value={form.teacher}
									onChange={(event) =>
										update("teacher", event.target.value)
									}
									required
								/>
							</Field>
							<Field>
								<FieldLabel htmlFor="course-location">
									Location
								</FieldLabel>
								<Input
									id="course-location"
									value={form.location}
									onChange={(event) =>
										update("location", event.target.value)
									}
									required
								/>
							</Field>
							<Field>
								<FieldLabel htmlFor="course-category">
									Category
								</FieldLabel>
								<NativeSelect
									className="w-full"
									id="course-category"
									value={form.category_id}
									onChange={(event) =>
										update(
											"category_id",
											event.target.value,
										)
									}
									required
								>
									{data.categories.map((category) => (
										<NativeSelectOption
											key={category}
											value={category}
										>
											{category}
										</NativeSelectOption>
									))}
								</NativeSelect>
							</Field>
							<Field>
								<FieldLabel htmlFor="course-membership">
									Membership
								</FieldLabel>
								<NativeSelect
									className="w-full"
									id="course-membership"
									value={form.membership}
									onChange={(event) =>
										update(
											"membership",
											event.target
												.value as MembershipType,
										)
									}
								>
									<NativeSelectOption value="free">
										Free choice
									</NativeSelectOption>
									<NativeSelectOption value="invite_only">
										Invite only
									</NativeSelectOption>
								</NativeSelect>
							</Field>
							<Field>
								<FieldLabel htmlFor="course-capacity">
									Maximum students
								</FieldLabel>
								<Input
									id="course-capacity"
									type="number"
									min="0"
									value={form.max_students}
									onChange={(event) =>
										update(
											"max_students",
											Number(event.target.value),
										)
									}
									required
								/>
								<FieldDescription>
									Use 0 to prevent normal student selections.
								</FieldDescription>
							</Field>
						</FieldGroup>
						<Field>
							<FieldLabel htmlFor="course-description">
								Description
							</FieldLabel>
							<Textarea
								id="course-description"
								value={form.description}
								onChange={(event) =>
									update("description", event.target.value)
								}
								rows={4}
							/>
						</Field>
						<FieldSet data-disabled={scheduleLocked}>
							<FieldLegend>Fixed time slots</FieldLegend>
							<FieldDescription>
								{scheduleLocked
									? "The timetable is locked because students already have this course. Remove those selections before changing it."
									: "Choose every slot occupied by this course."}
							</FieldDescription>
							{scheduleLocked ? (
								<Badge variant="secondary">
									<LockKeyholeIcon data-icon="inline-start" />
									Schedule locked
								</Badge>
							) : null}
							<FieldGroup
								data-slot="checkbox-group"
								className="grid sm:grid-cols-2 lg:grid-cols-3"
							>
								{data.periods.map((period) => (
									<Field
										key={period}
										orientation="horizontal"
										data-disabled={scheduleLocked}
									>
										<Checkbox
											id={domID("course-period", period)}
											checked={form.period_ids.includes(
												period,
											)}
											disabled={scheduleLocked}
											onCheckedChange={(checked) =>
												update(
													"period_ids",
													toggleValue(
														form.period_ids,
														period,
														checked,
													),
												)
											}
										/>
										<FieldLabel
											htmlFor={domID(
												"course-period",
												period,
											)}
										>
											{period}
										</FieldLabel>
									</Field>
								))}
							</FieldGroup>
						</FieldSet>
						<FieldGroup className="grid gap-5 sm:grid-cols-2">
							<FieldSet>
								<FieldLegend variant="label">
									Allowed legal sexes
								</FieldLegend>
								<FieldDescription>
									Leave every option unchecked to allow all.
								</FieldDescription>
								<FieldGroup data-slot="checkbox-group">
									{(["F", "M", "X"] as LegalSex[]).map(
										(legalSex) => (
											<Field
												key={legalSex}
												orientation="horizontal"
											>
												<Checkbox
													id={domID(
														"course-sex",
														legalSex,
													)}
													checked={form.allowed_legal_sexes.includes(
														legalSex,
													)}
													onCheckedChange={(
														checked,
													) =>
														update(
															"allowed_legal_sexes",
															toggleValue(
																form.allowed_legal_sexes,
																legalSex,
																checked,
															),
														)
													}
												/>
												<FieldLabel
													htmlFor={domID(
														"course-sex",
														legalSex,
													)}
												>
													{legalSex}
												</FieldLabel>
											</Field>
										),
									)}
								</FieldGroup>
							</FieldSet>
							<FieldSet>
								<FieldLegend variant="label">
									Allowed grades
								</FieldLegend>
								<FieldDescription>
									Leave every option unchecked to allow all.
								</FieldDescription>
								<FieldGroup data-slot="checkbox-group">
									{data.grades.map((grade) => (
										<Field
											key={grade.grade}
											orientation="horizontal"
										>
											<Checkbox
												id={domID(
													"course-grade",
													grade.grade,
												)}
												checked={form.allowed_grades.includes(
													grade.grade,
												)}
												onCheckedChange={(checked) =>
													update(
														"allowed_grades",
														toggleValue(
															form.allowed_grades,
															grade.grade,
															checked,
														),
													)
												}
											/>
											<FieldLabel
												htmlFor={domID(
													"course-grade",
													grade.grade,
												)}
											>
												{grade.grade}
											</FieldLabel>
										</Field>
									))}
								</FieldGroup>
							</FieldSet>
						</FieldGroup>
					</FieldGroup>
					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							onClick={() => onOpenChange(false)}
						>
							Cancel
						</Button>
						<Button
							type="submit"
							disabled={busy || form.period_ids.length === 0}
						>
							{busy ? (
								<Spinner data-icon="inline-start" />
							) : (
								<SaveIcon data-icon="inline-start" />
							)}
							Save course
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}

export function CoursesPage({
	data,
	refresh,
}: AdminPageProps): React.JSX.Element {
	const [query, setQuery] = useState("")
	const [dialogOpen, setDialogOpen] = useState(false)
	const [editing, setEditing] = useState<Course | null>(null)
	const courses = useSearchFilter(data.courses, query, getCourseSearchText)

	function openCourse(course: Course | null): void {
		setEditing(course)
		setDialogOpen(true)
	}

	return (
		<>
			<PageHeading
				title="Courses"
				description="Configure course details, restrictions, capacity, and one or more fixed timetable slots."
				action={
					<Button
						disabled={
							data.periods.length === 0 ||
							data.categories.length === 0
						}
						onClick={() => openCourse(null)}
					>
						<PlusIcon data-icon="inline-start" />
						Add course
					</Button>
				}
			/>
			<Card>
				<CardHeader>
					<CardTitle>Course catalogue</CardTitle>
					<CardDescription>
						{data.courses.length} courses configured
					</CardDescription>
					<CardAction className="col-span-2 col-start-1 row-span-1 row-start-3 mt-2 w-full justify-self-stretch sm:col-span-1 sm:col-start-2 sm:row-span-2 sm:row-start-1 sm:mt-0 sm:w-72 sm:justify-self-end">
						<SearchBox
							value={query}
							onChange={setQuery}
							placeholder="Search courses"
						/>
					</CardAction>
				</CardHeader>
				<CardContent>
					{courses.length === 0 ? (
						<NoResults
							title="No matching courses"
							description="Change the search or create a course."
						/>
					) : (
						<Table containerLabel="Course catalogue table">
							<TableCaption className="sr-only">
								Courses matching the current search.
							</TableCaption>
							<TableHeader>
								<TableRow>
									<TableHead>Course</TableHead>
									<TableHead>Schedule</TableHead>
									<TableHead>Teacher / location</TableHead>
									<TableHead>Students</TableHead>
									<TableHead className="text-right">
										Actions
									</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{courses.map((course) => (
									<TableRow key={course.id}>
										<TableCell>
											<div className="max-w-64">
												<p className="truncate font-medium">
													{course.name}
												</p>
												<p className="truncate text-xs text-muted-foreground">
													{course.id} ·{" "}
													{course.category_id}
												</p>
											</div>
										</TableCell>
										<TableCell>
											<PeriodBadges
												periodIDs={course.period_ids}
											/>
										</TableCell>
										<TableCell>
											<p>{course.teacher}</p>
											<p className="text-xs text-muted-foreground">
												{course.location}
											</p>
										</TableCell>
										<TableCell>
											<Badge variant="outline">
												{course.current_students}/
												{course.max_students}
											</Badge>
										</TableCell>
										<TableCell>
											<div className="flex justify-end gap-1">
												<Button
													variant="ghost"
													size="icon-sm"
													onClick={() =>
														openCourse(course)
													}
												>
													<PencilIcon />
													<span className="sr-only">
														Edit {course.name}
													</span>
												</Button>
												<DeleteButton
													name={course.name}
													disabled={
														course.current_students >
														0
													}
													description={
														course.current_students >
														0
															? "Remove all student selections before deleting this course."
															: "Delete this course and its timetable assignments."
													}
													onDelete={() =>
														runMutation(
															() =>
																apiRequest(
																	`/api/v1/admin/courses/${encodeURIComponent(course.id)}`,
																	{
																		method: "DELETE",
																	},
																),
															refresh,
															"Course deleted.",
														)
													}
												/>
											</div>
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
					)}
				</CardContent>
			</Card>
			{dialogOpen ? (
				<CourseDialog
					key={editing?.id ?? "new-course"}
					course={editing}
					data={data}
					open={dialogOpen}
					onOpenChange={setDialogOpen}
					refresh={refresh}
				/>
			) : null}
		</>
	)
}

function StudentDialog({
	student,
	grades,
	open,
	onOpenChange,
	refresh,
}: {
	student: Student | null
	grades: readonly Grade[]
	open: boolean
	onOpenChange: (open: boolean) => void
	refresh: () => Promise<void>
}): React.JSX.Element {
	const [id, setID] = useState(student === null ? "" : String(student.id))
	const [name, setName] = useState(student?.name ?? "")
	const [grade, setGrade] = useState(student?.grade ?? grades[0]?.grade ?? "")
	const [legalSex, setLegalSex] = useState<LegalSex>(
		student?.legal_sex ?? "X",
	)
	const [busy, setBusy] = useState(false)

	async function submit(
		event: React.FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault()
		setBusy(true)
		const saved = await runMutation(
			() =>
				apiRequest(
					student === null
						? "/api/v1/admin/students"
						: `/api/v1/admin/students/${student.id}`,
					{
						method: student === null ? "POST" : "PUT",
						body: jsonBody({
							id: Number(id),
							name,
							grade,
							legal_sex: legalSex,
						}),
					},
				),
			refresh,
			student === null ? "Student created." : "Student updated.",
		)
		setBusy(false)
		if (saved) onOpenChange(false)
	}

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent>
				<form onSubmit={(event) => void submit(event)}>
					<DialogHeader>
						<DialogTitle>
							{student === null
								? "Add student"
								: `Edit ${student.name}`}
						</DialogTitle>
						<DialogDescription>
							Student IDs are used for authentication and cannot
							be changed after creation.
						</DialogDescription>
					</DialogHeader>
					<FieldGroup className="py-5">
						<Field data-disabled={student !== null}>
							<FieldLabel htmlFor="student-id">
								Student ID
							</FieldLabel>
							<Input
								id="student-id"
								type="number"
								min="1"
								value={id}
								disabled={student !== null}
								onChange={(event) => setID(event.target.value)}
								required
							/>
						</Field>
						<Field>
							<FieldLabel htmlFor="student-name">Name</FieldLabel>
							<Input
								id="student-name"
								value={name}
								onChange={(event) =>
									setName(event.target.value)
								}
								required
							/>
						</Field>
						<FieldGroup className="grid gap-4 sm:grid-cols-2">
							<Field>
								<FieldLabel htmlFor="student-grade">
									Grade
								</FieldLabel>
								<NativeSelect
									className="w-full"
									id="student-grade"
									value={grade}
									onChange={(event) =>
										setGrade(event.target.value)
									}
								>
									{grades.map((item) => (
										<NativeSelectOption
											key={item.grade}
											value={item.grade}
										>
											{item.grade}
										</NativeSelectOption>
									))}
								</NativeSelect>
							</Field>
							<Field>
								<FieldLabel htmlFor="student-sex">
									Legal sex
								</FieldLabel>
								<NativeSelect
									className="w-full"
									id="student-sex"
									value={legalSex}
									onChange={(event) =>
										setLegalSex(
											event.target.value as LegalSex,
										)
									}
								>
									<NativeSelectOption value="F">
										F
									</NativeSelectOption>
									<NativeSelectOption value="M">
										M
									</NativeSelectOption>
									<NativeSelectOption value="X">
										X
									</NativeSelectOption>
								</NativeSelect>
							</Field>
						</FieldGroup>
					</FieldGroup>
					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							onClick={() => onOpenChange(false)}
						>
							Cancel
						</Button>
						<Button
							type="submit"
							disabled={
								busy ||
								Number(id) <= 0 ||
								name.trim() === "" ||
								grade === ""
							}
						>
							{busy ? (
								<Spinner data-icon="inline-start" />
							) : (
								<SaveIcon data-icon="inline-start" />
							)}
							Save student
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}

export function StudentsPage({
	data,
	refresh,
}: AdminPageProps): React.JSX.Element {
	const [query, setQuery] = useState("")
	const [dialogOpen, setDialogOpen] = useState(false)
	const [editing, setEditing] = useState<Student | null>(null)
	const students = useSearchFilter(data.students, query, getStudentSearchText)
	const selectionCounts = useMemo(
		() => countSelectionsByStudent(data.selections),
		[data.selections],
	)

	function openStudent(student: Student | null): void {
		setEditing(student)
		setDialogOpen(true)
	}

	return (
		<>
			<PageHeading
				title="Students"
				description="Manage student profiles without exposing authentication sessions."
				action={
					<Button
						disabled={data.grades.length === 0}
						onClick={() => openStudent(null)}
					>
						<PlusIcon data-icon="inline-start" />
						Add student
					</Button>
				}
			/>
			<Card>
				<CardHeader>
					<CardTitle>Student directory</CardTitle>
					<CardDescription>
						{data.students.length} students registered
					</CardDescription>
					<CardAction className="col-span-2 col-start-1 row-span-1 row-start-3 mt-2 w-full justify-self-stretch sm:col-span-1 sm:col-start-2 sm:row-span-2 sm:row-start-1 sm:mt-0 sm:w-72 sm:justify-self-end">
						<SearchBox
							value={query}
							onChange={setQuery}
							placeholder="Search students"
						/>
					</CardAction>
				</CardHeader>
				<CardContent>
					{students.length === 0 ? (
						<NoResults
							title="No matching students"
							description="Change the search or add a student."
						/>
					) : (
						<Table containerLabel="Student directory table">
							<TableCaption className="sr-only">
								Students matching the current search.
							</TableCaption>
							<TableHeader>
								<TableRow>
									<TableHead>ID</TableHead>
									<TableHead>Name</TableHead>
									<TableHead>Grade</TableHead>
									<TableHead>Legal sex</TableHead>
									<TableHead>Selections</TableHead>
									<TableHead className="text-right">
										Actions
									</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{students.map((student) => {
									const selectionCount =
										selectionCounts.get(student.id) ?? 0
									return (
										<TableRow key={student.id}>
											<TableCell className="font-mono text-xs">
												{student.id}
											</TableCell>
											<TableCell className="font-medium">
												{student.name}
											</TableCell>
											<TableCell>
												<Badge variant="outline">
													{student.grade}
												</Badge>
											</TableCell>
											<TableCell>
												{student.legal_sex}
											</TableCell>
											<TableCell>
												{selectionCount}
											</TableCell>
											<TableCell>
												<div className="flex justify-end gap-1">
													<Button
														variant="ghost"
														size="icon-sm"
														onClick={() =>
															openStudent(student)
														}
													>
														<PencilIcon />
														<span className="sr-only">
															Edit {student.name}
														</span>
													</Button>
													<DeleteButton
														name={student.name}
														disabled={
															selectionCount > 0
														}
														description={
															selectionCount > 0
																? "Remove all course selections before deleting this student."
																: "Delete this student profile."
														}
														onDelete={() =>
															runMutation(
																() =>
																	apiRequest(
																		`/api/v1/admin/students/${student.id}`,
																		{
																			method: "DELETE",
																		},
																	),
																refresh,
																"Student deleted.",
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
			{dialogOpen ? (
				<StudentDialog
					key={editing?.id ?? "new-student"}
					student={editing}
					grades={data.grades}
					open={dialogOpen}
					onOpenChange={setDialogOpen}
					refresh={refresh}
				/>
			) : null}
		</>
	)
}

function MultiChoiceList<T extends string | number>({
	items,
	selected,
	onToggle,
	getID,
	getLabel,
	emptyText,
}: {
	items: Array<{ value: T; label: string; detail: string }>
	selected: T[]
	onToggle: (value: T, checked: boolean) => void
	getID: (value: T) => string
	getLabel: (value: T) => string
	emptyText: string
}): React.JSX.Element {
	if (items.length === 0)
		return <p className="text-sm text-muted-foreground">{emptyText}</p>
	return (
		<FieldGroup
			data-slot="checkbox-group"
			className="max-h-52 overflow-y-auto rounded-lg border p-2"
		>
			{items.map((item) => (
				<Field key={String(item.value)} orientation="horizontal">
					<Checkbox
						id={getID(item.value)}
						checked={selected.includes(item.value)}
						onCheckedChange={(checked) =>
							onToggle(item.value, checked)
						}
					/>
					<FieldContent>
						<FieldLabel htmlFor={getID(item.value)}>
							{getLabel(item.value)}
						</FieldLabel>
						<FieldDescription>{item.detail}</FieldDescription>
					</FieldContent>
				</Field>
			))}
		</FieldGroup>
	)
}

function SelectionTypeBadge({
	type,
}: {
	type: SelectionType
}): React.JSX.Element {
	const label =
		type === "normal"
			? "Student choice"
			: type === "invite"
				? "Invitation"
				: "Forced"
	return (
		<Badge
			variant={
				type === "force"
					? "destructive"
					: type === "invite"
						? "secondary"
						: "outline"
			}
		>
			{label}
		</Badge>
	)
}

const COURSE_SLOT_SEPARATOR = "\u001f"

interface CourseSlotAssignment {
	courseID: string
	periodID: string
}

function courseSlotValue(courseID: string, periodID: string): string {
	return `${courseID}${COURSE_SLOT_SEPARATOR}${periodID}`
}

function parseCourseSlotValue(value: string): CourseSlotAssignment | null {
	const separatorIndex = value.indexOf(COURSE_SLOT_SEPARATOR)
	if (separatorIndex <= 0 || separatorIndex === value.length - 1) return null

	return {
		courseID: value.slice(0, separatorIndex),
		periodID: value.slice(separatorIndex + COURSE_SLOT_SEPARATOR.length),
	}
}

export function SelectionsPage({
	data,
	refresh,
}: AdminPageProps): React.JSX.Element {
	const [studentQuery, setStudentQuery] = useState("")
	const [courseQuery, setCourseQuery] = useState("")
	const [tableQuery, setTableQuery] = useState("")
	const [studentIDs, setStudentIDs] = useState<number[]>([])
	const [courseSlotIDs, setCourseSlotIDs] = useState<string[]>([])
	const [selectionType, setSelectionType] = useState<SelectionType>("invite")
	const [busy, setBusy] = useState(false)

	const students = useSearchFilter(
		data.students,
		studentQuery,
		getStudentSearchText,
	)
	const courses = useSearchFilter(
		data.courses,
		courseQuery,
		getCourseSearchText,
	)
	const selections = useSearchFilter(
		data.selections,
		tableQuery,
		getSelectionSearchText,
	)
	const studentItems = useMemo(
		() =>
			students.map((student) => ({
				value: student.id,
				label: student.name,
				detail: `${student.id} · ${student.grade}`,
			})),
		[students],
	)
	const courseItems = useMemo(
		() =>
			courses.flatMap((course) =>
				course.period_ids.map((periodID) => ({
					value: courseSlotValue(course.id, periodID),
					label: course.name,
					detail: periodID,
				})),
			),
		[courses],
	)
	const studentNamesByID = useMemo(
		() =>
			new Map(data.students.map((student) => [student.id, student.name])),
		[data.students],
	)
	const courseNamesByID = useMemo(
		() => new Map(data.courses.map((course) => [course.id, course.name])),
		[data.courses],
	)

	async function addSelections(
		event: React.FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault()
		const assignments = courseSlotIDs.flatMap((value) => {
			const assignment = parseCourseSlotValue(value)
			return assignment === null ? [] : [assignment]
		})
		if (assignments.length !== courseSlotIDs.length) {
			toast.error("One or more course time slots are invalid.")
			return
		}
		setBusy(true)
		const created = await runMutation(
			() =>
				apiRequest("/api/v1/admin/selections", {
					method: "POST",
					body: jsonBody({
						student_ids: studentIDs,
						course_ids: assignments.map(
							(assignment) => assignment.courseID,
						),
						period_ids: assignments.map(
							(assignment) => assignment.periodID,
						),
						selection_type: selectionType,
					}),
				}),
			refresh,
			`${studentIDs.length * courseSlotIDs.length} selection assignments created.`,
		)
		setBusy(false)
		if (created) {
			setStudentIDs([])
			setCourseSlotIDs([])
		}
	}

	async function updateType(
		selection: Selection,
		type: SelectionType,
	): Promise<void> {
		if (selection.student_id === undefined) return
		await runMutation(
			() =>
				apiRequest(
					`/api/v1/admin/selections/${selection.student_id}/${encodeURIComponent(selection.course_id)}`,
					{
						method: "PUT",
						body: jsonBody({
							course_id: selection.course_id,
							period_id: selection.period_id,
							selection_type: type,
						}),
					},
				),
			refresh,
			"Selection type updated.",
		)
	}

	return (
		<>
			<PageHeading
				title="Selections"
				description="Assign one or more students to one or more compatible courses, then review every active selection."
			/>
			<Card>
				<CardHeader>
					<CardTitle>Create assignments</CardTitle>
					<CardDescription>
						All assignments are checked together; a clash or invalid
						restriction prevents the batch from being saved.
					</CardDescription>
				</CardHeader>
				<CardContent>
					<form onSubmit={(event) => void addSelections(event)}>
						<FieldGroup>
							<FieldGroup className="grid gap-5 lg:grid-cols-2">
								<FieldSet>
									<FieldLegend>Students</FieldLegend>
									<SearchBox
										value={studentQuery}
										onChange={setStudentQuery}
										placeholder="Filter students"
									/>
									<MultiChoiceList
										items={studentItems}
										selected={studentIDs}
										onToggle={(id, checked) =>
											setStudentIDs((current) =>
												checked
													? [...current, id]
													: current.filter(
															(item) =>
																item !== id,
														),
											)
										}
										getID={(id) =>
											`selection-student-${id}`
										}
										getLabel={(id) =>
											studentNamesByID.get(id) ??
											String(id)
										}
										emptyText="No students match the filter."
									/>
								</FieldSet>
								<FieldSet>
									<FieldLegend>Courses</FieldLegend>
									<SearchBox
										value={courseQuery}
										onChange={setCourseQuery}
										placeholder="Filter courses"
									/>
									<MultiChoiceList
										items={courseItems}
										selected={courseSlotIDs}
										onToggle={(id, checked) =>
											setCourseSlotIDs((current) => {
												if (!checked) {
													return current.filter(
														(item) => item !== id,
													)
												}
												const assignment =
													parseCourseSlotValue(id)
												if (assignment === null)
													return current
												return [
													...current.filter(
														(item) =>
															!item.startsWith(
																`${assignment.courseID}${COURSE_SLOT_SEPARATOR}`,
															),
													),
													id,
												]
											})
										}
										getID={(id) => `selection-course-${id}`}
										getLabel={(id) => {
											const assignment =
												parseCourseSlotValue(id)
											if (assignment === null) return id
											const courseName =
												courseNamesByID.get(
													assignment.courseID,
												) ?? assignment.courseID
											return `${courseName} · ${assignment.periodID}`
										}}
										emptyText="No courses match the filter."
									/>
								</FieldSet>
							</FieldGroup>
							<Field>
								<FieldLabel htmlFor="selection-type">
									Assignment type
								</FieldLabel>
								<NativeSelect
									id="selection-type"
									className="w-full sm:max-w-xs"
									value={selectionType}
									onChange={(event) =>
										setSelectionType(
											event.target.value as SelectionType,
										)
									}
								>
									<NativeSelectOption value="normal">
										Student choice
									</NativeSelectOption>
									<NativeSelectOption value="invite">
										Invitation
									</NativeSelectOption>
									<NativeSelectOption value="force">
										Forced
									</NativeSelectOption>
								</NativeSelect>
								<FieldDescription>
									Schedule clashes are rejected for every
									assignment type.
								</FieldDescription>
							</Field>
							<Button
								type="submit"
								disabled={
									busy ||
									studentIDs.length === 0 ||
									courseSlotIDs.length === 0
								}
							>
								{busy ? (
									<Spinner data-icon="inline-start" />
								) : (
									<PlusIcon data-icon="inline-start" />
								)}
								Create{" "}
								{studentIDs.length * courseSlotIDs.length || ""}{" "}
								assignments
							</Button>
						</FieldGroup>
					</form>
				</CardContent>
			</Card>

			<Card className="mt-4">
				<CardHeader>
					<CardTitle>Current selections</CardTitle>
					<CardDescription>
						{data.selections.length} assignments
					</CardDescription>
					<CardAction className="col-span-2 col-start-1 row-span-1 row-start-3 mt-2 w-full justify-self-stretch sm:col-span-1 sm:col-start-2 sm:row-span-2 sm:row-start-1 sm:mt-0 sm:w-72 sm:justify-self-end">
						<SearchBox
							value={tableQuery}
							onChange={setTableQuery}
							placeholder="Search selections"
						/>
					</CardAction>
				</CardHeader>
				<CardContent>
					{selections.length === 0 ? (
						<NoResults
							title="No matching selections"
							description="Create an assignment or change the search."
						/>
					) : (
						<Table containerLabel="Current selections table">
							<TableCaption className="sr-only">
								Selections matching the current search.
							</TableCaption>
							<TableHeader>
								<TableRow>
									<TableHead>Student</TableHead>
									<TableHead>Course</TableHead>
									<TableHead>Schedule</TableHead>
									<TableHead>Type</TableHead>
									<TableHead className="text-right">
										Actions
									</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{selections.map((selection) => (
									<TableRow
										key={`${selection.student_id}-${selection.course_id}`}
									>
										<TableCell>
											<p className="font-medium">
												{selection.student_name ??
													selection.student_id}
											</p>
											<p className="text-xs text-muted-foreground">
												{selection.student_grade}
											</p>
										</TableCell>
										<TableCell>
											<p className="font-medium">
												{selection.course_name ??
													selection.course_id}
											</p>
											<p className="text-xs text-muted-foreground">
												{selection.course_id}
											</p>
										</TableCell>
										<TableCell>
											<PeriodBadges
												periodIDs={[
													selection.period_id,
												]}
											/>
										</TableCell>
										<TableCell>
											<div className="flex items-center gap-2">
												<SelectionTypeBadge
													type={
														selection.selection_type
													}
												/>
												<NativeSelect
													size="sm"
													aria-label={`Selection type for ${selection.student_name ?? selection.student_id} in ${selection.course_name ?? selection.course_id}`}
													value={
														selection.selection_type
													}
													onChange={(event) =>
														void updateType(
															selection,
															event.target
																.value as SelectionType,
														)
													}
												>
													<NativeSelectOption value="normal">
														Student choice
													</NativeSelectOption>
													<NativeSelectOption value="invite">
														Invitation
													</NativeSelectOption>
													<NativeSelectOption value="force">
														Forced
													</NativeSelectOption>
												</NativeSelect>
											</div>
										</TableCell>
										<TableCell>
											<div className="flex justify-end">
												<DeleteButton
													name="selection"
													description={`Remove ${selection.course_name ?? selection.course_id} from ${selection.student_name ?? selection.student_id}.`}
													onDelete={() =>
														selection.student_id ===
														undefined
															? Promise.resolve(
																	false,
																)
															: runMutation(
																	() =>
																		apiRequest(
																			`/api/v1/admin/selections/${selection.student_id}/${encodeURIComponent(selection.course_id)}`,
																			{
																				method: "DELETE",
																			},
																		),
																	refresh,
																	"Selection deleted.",
																)
													}
												/>
											</div>
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
					)}
				</CardContent>
			</Card>
		</>
	)
}

export function NotificationsPage({
	refresh,
}: AdminPageProps): React.JSX.Element {
	const [message, setMessage] = useState("")
	const [busy, setBusy] = useState(false)

	async function send(
		event: React.FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault()
		setBusy(true)
		const sent = await runMutation(
			() =>
				apiRequest("/api/v1/admin/notifications", {
					method: "POST",
					body: jsonBody({ message }),
				}),
			refresh,
			"Notification sent to connected students.",
		)
		setBusy(false)
		if (sent) setMessage("")
	}

	return (
		<>
			<PageHeading
				title="Notifications"
				description="Broadcast a short real-time message to currently connected students."
			/>
			<Card className="max-w-2xl">
				<CardHeader>
					<CardTitle>Broadcast message</CardTitle>
					<CardDescription>
						Messages are delivered live and are not stored as an
						inbox.
					</CardDescription>
					<CardAction>
						<BellRingIcon aria-hidden="true" />
					</CardAction>
				</CardHeader>
				<CardContent>
					<form onSubmit={(event) => void send(event)}>
						<FieldGroup>
							<Field>
								<FieldLabel htmlFor="notification-message">
									Message
								</FieldLabel>
								<Textarea
									id="notification-message"
									value={message}
									maxLength={1000}
									rows={6}
									onChange={(event) =>
										setMessage(event.target.value)
									}
									placeholder="Type an announcement for all connected users…"
									required
								/>
								<FieldDescription>
									{message.length}/1000 characters
								</FieldDescription>
							</Field>
							<Button
								type="submit"
								disabled={busy || message.trim() === ""}
							>
								{busy ? (
									<Spinner data-icon="inline-start" />
								) : (
									<SendIcon data-icon="inline-start" />
								)}
								Send notification
							</Button>
						</FieldGroup>
					</form>
				</CardContent>
			</Card>
		</>
	)
}

function ImportCard({
	title,
	description,
	action,
	example,
}: {
	title: string
	description: string
	action: string
	example: string
}): React.JSX.Element {
	return (
		<Card>
			<CardHeader>
				<CardTitle>{title}</CardTitle>
				<CardDescription>{description}</CardDescription>
			</CardHeader>
			<CardContent>
				<form
					action={action}
					method="post"
					encType="multipart/form-data"
				>
					<FieldGroup>
						<Field>
							<FieldLabel htmlFor={domID("import", title)}>
								CSV file
							</FieldLabel>
							<Input
								id={domID("import", title)}
								type="file"
								name="csv"
								accept=".csv,text/csv"
								required
							/>
						</Field>
						<Button type="submit" variant="outline">
							<UploadIcon data-icon="inline-start" />
							Import CSV
						</Button>
						<Button
							variant="link"
							render={<a href={example} />}
							nativeButton={false}
						>
							<DownloadIcon data-icon="inline-start" />
							Download example
						</Button>
					</FieldGroup>
				</form>
			</CardContent>
		</Card>
	)
}

export function DataManagementPage({
	data,
}: AdminPageProps): React.JSX.Element {
	return (
		<>
			<PageHeading
				title="Data management"
				description="Import courses, students, or selections in bulk and export the current selection list."
			/>
			<div className="grid gap-4 lg:grid-cols-3">
				<Card>
					<CardHeader>
						<CardTitle>Selection export</CardTitle>
						<CardDescription>
							Download all {data.selections.length} selection
							{data.selections.length === 1 ? "" : "s"}, including
							every fixed time slot for each course.
						</CardDescription>
					</CardHeader>
					<CardContent>
						<Button
							render={<a href="/admin/selections/export" />}
							nativeButton={false}
						>
							<DownloadIcon data-icon="inline-start" />
							Download CSV
						</Button>
					</CardContent>
				</Card>
				<ImportCard
					title="Course import"
					description="Bulk-create courses from a compatible CSV. Existing IDs are rejected."
					action="/admin/courses/import"
					example="/admin/static/courses_example.csv"
				/>
				<ImportCard
					title="Student import"
					description="Bulk-create student profiles from a compatible CSV. Existing IDs are rejected."
					action="/admin/students/import"
					example="/admin/static/students_example.csv"
				/>
				<ImportCard
					title="Selection import"
					description="Bulk-create normal, invited, or forced selections from CSV. The entire file is transactional."
					action="/admin/selections/import"
					example="/admin/static/selections_example.csv"
				/>
			</div>
			<Card className="mt-4">
				<CardHeader>
					<CardTitle>Import notes</CardTitle>
					<CardDescription>
						CSV uploads are validated and applied transactionally by
						the Go service.
					</CardDescription>
				</CardHeader>
				<CardContent className="flex flex-col gap-2 text-sm text-muted-foreground">
					<p>
						Course schedules use a comma-separated{" "}
						<code>periods</code> column containing fixed slot
						identifiers.
					</p>
					<p>
						Import grades, categories, and time slots before courses
						or students that reference them.
					</p>
				</CardContent>
			</Card>
		</>
	)
}

// Used while a route-level bootstrap is being refreshed by a parent boundary.
export function AdminPageSkeleton(): React.JSX.Element {
	return (
		<div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
			{Array.from({ length: 6 }, (_, index) => (
				<Skeleton key={index} className="h-48" />
			))}
		</div>
	)
}
