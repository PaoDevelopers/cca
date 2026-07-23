import { useEffect, useMemo, useState } from "react"
import {
	BellRingIcon,
	BookOpenIcon,
	CalendarClockIcon,
	CalendarDaysIcon,
	DownloadIcon,
	InboxIcon,
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
import { Calendar } from "@/components/ui/calendar"
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
	Combobox,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
} from "@/components/ui/combobox"
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
import {
	Item,
	ItemActions,
	ItemContent,
	ItemGroup,
	ItemTitle,
} from "@/components/ui/item"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "@/components/ui/popover"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
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
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip"
import type { AdminPageProps } from "@/features/admin/AdminApp"
import { useSearchFilter } from "@/hooks/use-search-filter"
import { CCA_DAYS, CCA_SLOTS_PER_DAY, ccaTimeSlotID } from "@/lib/cca-schedule"
import type {
	AdminDashboard,
	Course,
	CoursePayload,
	Grade,
	GradeSelectionSchedule,
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
				<h1 className="font-heading text-xl font-semibold tracking-tight">
					{title}
				</h1>
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
						aria-label={`Delete ${name}`}
						disabled={disabled}
					/>
				}
			>
				<Trash2Icon aria-hidden="true" />
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
					<Badge variant="secondary">
						<Icon aria-hidden="true" />
						<span className="sr-only">{label}</span>
					</Badge>
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

export function DashboardPage({
	data,
}: {
	data: AdminDashboard
}): React.JSX.Element {
	return (
		<>
			<PageHeading
				title="System overview"
				description="A live summary of courses, students, and grade access."
			/>
			<div className="grid gap-4 sm:grid-cols-2">
				<StatCard
					icon={BookOpenIcon}
					label="Courses"
					value={data.course_count}
					description={`${data.courses_without_timetable} without a timetable`}
				/>
				<StatCard
					icon={UsersIcon}
					label="Students"
					value={data.student_count}
					description="Registered student profiles"
				/>
			</div>

			<div className="mt-4">
				<Card>
					<CardHeader>
						<CardTitle>Grade settings</CardTitle>
						<CardDescription>
							Selection access and own-choice limits by grade.
						</CardDescription>
					</CardHeader>
					<CardContent>
						{data.grades.length === 0 ? (
							<NoResults
								title="No grades yet"
								description="Add a grade before importing students."
							/>
						) : (
							<ItemGroup>
								{data.grades.map((grade) => (
									<Item key={grade.grade} size="xs">
										<ItemContent>
											<ItemTitle>
												Grade {grade.grade}
											</ItemTitle>
										</ItemContent>
										<ItemActions>
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
												{grade.enabled
													? "Open"
													: "Closed"}
											</Badge>
										</ItemActions>
									</Item>
								))}
							</ItemGroup>
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

const CHINA_TIME_ZONE = "Asia/Shanghai"

function formatChinaTime(value: string): string {
	return new Intl.DateTimeFormat("en-GB", {
		dateStyle: "medium",
		timeStyle: "short",
		timeZone: CHINA_TIME_ZONE,
	}).format(new Date(value))
}

function calendarDateLabel(value: Date | undefined): string {
	return value === undefined
		? "Choose a date"
		: new Intl.DateTimeFormat("en-GB", { dateStyle: "medium" }).format(
				value,
			)
}

function dateKey(value: Date): string {
	const year = value.getFullYear()
	const month = String(value.getMonth() + 1).padStart(2, "0")
	const day = String(value.getDate()).padStart(2, "0")
	return `${year}-${month}-${day}`
}

function combineChinaDateTime(
	date: Date | undefined,
	time: string,
): string | null {
	if (date === undefined || !/^\d{2}:\d{2}$/.test(time)) return null
	const value = new Date(`${dateKey(date)}T${time}:00+08:00`)
	return Number.isNaN(value.getTime()) ? null : value.toISOString()
}

function chinaEditorParts(value: string): { date: Date; time: string } {
	const parts = new Intl.DateTimeFormat("en-CA", {
		year: "numeric",
		month: "2-digit",
		day: "2-digit",
		hour: "2-digit",
		minute: "2-digit",
		hourCycle: "h23",
		timeZone: CHINA_TIME_ZONE,
	}).formatToParts(new Date(value))
	const part = (type: Intl.DateTimeFormatPartTypes): string =>
		parts.find((item) => item.type === type)?.value ?? ""
	return {
		date: new Date(
			Number(part("year")),
			Number(part("month")) - 1,
			Number(part("day")),
		),
		time: `${part("hour")}:${part("minute")}`,
	}
}

function GradeLimitControl({
	grade,
	refresh,
}: {
	grade: Grade
	refresh: () => Promise<void>
}): React.JSX.Element {
	const [limit, setLimit] = useState(String(grade.max_own_choices))
	const [busy, setBusy] = useState(false)

	async function save(): Promise<void> {
		setBusy(true)
		await runMutation(
			() =>
				apiRequest(
					`/api/v1/admin/grades/${encodeURIComponent(grade.grade)}`,
					{
						method: "PUT",
						body: jsonBody({
							max_own_choices: Number(limit),
						}),
					},
				),
			refresh,
			`${grade.grade} choice limit saved.`,
		)
		setBusy(false)
	}

	return (
		<div className="flex min-w-44 items-center gap-2">
			<Input
				aria-label={`Maximum own choices for ${grade.grade}`}
				className="w-20"
				type="number"
				min="0"
				value={limit}
				onChange={(event) => setLimit(event.target.value)}
			/>
			<Button
				variant="outline"
				size="icon"
				aria-label={`Save maximum own choices for ${grade.grade}`}
				disabled={
					busy ||
					!Number.isInteger(Number(limit)) ||
					Number(limit) < 0 ||
					Number(limit) === grade.max_own_choices
				}
				onClick={() => void save()}
			>
				{busy ? <Spinner /> : <SaveIcon />}
			</Button>
		</div>
	)
}

function ScheduleDateField({
	id,
	label,
	date,
	time,
	onDateChange,
	onTimeChange,
	disabled = false,
}: {
	id: string
	label: string
	date: Date | undefined
	time: string
	onDateChange: (value: Date | undefined) => void
	onTimeChange: (value: string) => void
	disabled?: boolean
}): React.JSX.Element {
	return (
		<Field>
			<FieldLabel>{label}</FieldLabel>
			<div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_8rem]">
				<Popover>
					<PopoverTrigger
						render={
							<Button
								variant="outline"
								className="justify-start"
								disabled={disabled}
							/>
						}
					>
						<CalendarDaysIcon data-icon="inline-start" />
						{calendarDateLabel(date)}
					</PopoverTrigger>
					<PopoverContent align="start" className="w-auto p-0">
						<Calendar
							mode="single"
							selected={date}
							onSelect={onDateChange}
							timeZone={CHINA_TIME_ZONE}
							autoFocus
						/>
					</PopoverContent>
				</Popover>
				<Input
					id={id}
					type="time"
					value={time}
					onChange={(event) => onTimeChange(event.target.value)}
					disabled={disabled}
					required
				/>
			</div>
		</Field>
	)
}

function GradeScheduleDialog({
	open,
	onOpenChange,
	grades,
	schedules,
	initialGradeIDs,
	editing,
	refresh,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	grades: readonly Grade[]
	schedules: readonly GradeSelectionSchedule[]
	initialGradeIDs: readonly string[]
	editing: GradeSelectionSchedule | null
	refresh: () => Promise<void>
}): React.JSX.Element {
	const [gradeIDs, setGradeIDs] = useState<string[]>([
		...(editing?.grade_ids ?? initialGradeIDs),
	])
	const [openingDate, setOpeningDate] = useState<Date | undefined>(() => {
		if (editing !== null) return chinaEditorParts(editing.opens_at).date
		const tomorrow = chinaEditorParts(new Date().toISOString()).date
		tomorrow.setDate(tomorrow.getDate() + 1)
		tomorrow.setHours(0, 0, 0, 0)
		return tomorrow
	})
	const [openingTime, setOpeningTime] = useState(() =>
		editing === null ? "08:00" : chinaEditorParts(editing.opens_at).time,
	)
	const [hasClosing, setHasClosing] = useState(
		editing?.closes_at !== undefined,
	)
	const [closingDate, setClosingDate] = useState<Date | undefined>(() =>
		editing?.closes_at === undefined
			? openingDate
			: chinaEditorParts(editing.closes_at).date,
	)
	const [closingTime, setClosingTime] = useState(() =>
		editing?.closes_at === undefined
			? "17:00"
			: chinaEditorParts(editing.closes_at).time,
	)
	const [busy, setBusy] = useState(false)

	const openingISO = combineChinaDateTime(openingDate, openingTime)
	const closingISO = hasClosing
		? combineChinaDateTime(closingDate, closingTime)
		: null
	const conflictingGrades = useMemo(() => {
		const selected = new Set(gradeIDs)
		const conflicts = schedules
			.filter((schedule) => schedule.batch_id !== editing?.batch_id)
			.flatMap((schedule) => schedule.grade_ids)
			.filter((grade) => selected.has(grade))
		return [...new Set(conflicts)].sort()
	}, [editing?.batch_id, gradeIDs, schedules])
	const activeConflictGrades = useMemo(() => {
		const selected = new Set(gradeIDs)
		const conflicts = schedules
			.filter(
				(schedule) =>
					schedule.opened && schedule.batch_id !== editing?.batch_id,
			)
			.flatMap((schedule) => schedule.grade_ids)
			.filter((grade) => selected.has(grade))
		return [...new Set(conflicts)].sort()
	}, [editing?.batch_id, gradeIDs, schedules])
	const replaceableConflictGrades = conflictingGrades.filter(
		(grade) => !activeConflictGrades.includes(grade),
	)
	const currentlyOpenGrades = gradeIDs.filter(
		(gradeID) =>
			grades.find((grade) => grade.grade === gradeID)?.enabled === true,
	)
	const valid =
		gradeIDs.length > 0 &&
		activeConflictGrades.length === 0 &&
		openingISO !== null &&
		(!hasClosing ||
			(closingISO !== null &&
				new Date(closingISO) > new Date(openingISO)))

	function toggleGrade(grade: string, checked: boolean): void {
		setGradeIDs((current) =>
			checked
				? [...current, grade].sort()
				: current.filter((item) => item !== grade),
		)
	}

	async function submit(
		event: React.FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault()
		if (!valid || openingISO === null) return
		setBusy(true)
		const saved = await runMutation(
			() =>
				apiRequest(
					editing === null
						? "/api/v1/admin/grade-schedules"
						: `/api/v1/admin/grade-schedules/${editing.batch_id}`,
					{
						method: editing === null ? "POST" : "PUT",
						body: jsonBody({
							grade_ids: gradeIDs,
							opens_at: openingISO,
							closes_at: closingISO,
							replace_existing:
								replaceableConflictGrades.length > 0,
						}),
					},
				),
			refresh,
			editing === null
				? "Selection schedule created."
				: "Selection schedule updated.",
		)
		setBusy(false)
		if (saved) onOpenChange(false)
	}

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="sm:max-w-2xl">
				<form onSubmit={(event) => void submit(event)}>
					<DialogHeader>
						<DialogTitle>
							{editing === null
								? "Schedule selection access"
								: "Edit selection schedule"}
						</DialogTitle>
						<DialogDescription>
							Choose who can start selecting and when. All times
							use China Standard Time (UTC+8).
						</DialogDescription>
					</DialogHeader>
					<FieldGroup className="py-5">
						<FieldSet disabled={editing?.opened}>
							<FieldLegend variant="label">Grades</FieldLegend>
							<FieldDescription>
								The same opening and closing time will apply to
								every selected grade.
							</FieldDescription>
							<div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
								{grades.map((grade) => (
									<Field
										key={grade.grade}
										orientation="horizontal"
									>
										<Checkbox
											id={domID(
												"schedule-grade",
												grade.grade,
											)}
											checked={gradeIDs.includes(
												grade.grade,
											)}
											onCheckedChange={(checked) =>
												toggleGrade(
													grade.grade,
													checked,
												)
											}
										/>
										<FieldLabel
											htmlFor={domID(
												"schedule-grade",
												grade.grade,
											)}
										>
											{grade.grade}
										</FieldLabel>
									</Field>
								))}
							</div>
						</FieldSet>
						<ScheduleDateField
							id="schedule-opening-time"
							label="Selections open"
							date={openingDate}
							time={openingTime}
							onDateChange={setOpeningDate}
							onTimeChange={setOpeningTime}
							disabled={editing?.opened ?? false}
						/>
						<Field orientation="horizontal">
							<Switch
								id="schedule-has-closing"
								checked={hasClosing}
								onCheckedChange={setHasClosing}
								disabled={editing?.opened}
							/>
							<FieldContent>
								<FieldLabel htmlFor="schedule-has-closing">
									Close selections automatically
								</FieldLabel>
								<FieldDescription>
									When this time arrives, students can no
									longer add or remove selections. Leave this
									off to keep access open.
								</FieldDescription>
							</FieldContent>
						</Field>
						{hasClosing ? (
							<ScheduleDateField
								id="schedule-closing-time"
								label="Selections close"
								date={closingDate}
								time={closingTime}
								onDateChange={setClosingDate}
								onTimeChange={setClosingTime}
							/>
						) : null}
						{currentlyOpenGrades.length > 0 && !editing?.opened ? (
							<Alert>
								<CalendarClockIcon />
								<AlertTitle>
									Currently open grades will close now
								</AlertTitle>
								<AlertDescription>
									Saving will close selections for{" "}
									{currentlyOpenGrades.join(", ")}{" "}
									immediately, then reopen them at the
									scheduled time.
								</AlertDescription>
							</Alert>
						) : null}
						{activeConflictGrades.length > 0 ? (
							<Alert variant="destructive">
								<CalendarClockIcon />
								<AlertTitle>
									An active window cannot be replaced
								</AlertTitle>
								<AlertDescription>
									Edit the current closing time or manually
									close selections for{" "}
									{activeConflictGrades.join(", ")}
									before creating a new schedule.
								</AlertDescription>
							</Alert>
						) : null}
						{replaceableConflictGrades.length > 0 ? (
							<Alert variant="destructive">
								<CalendarClockIcon />
								<AlertTitle>
									Existing schedules will be replaced
								</AlertTitle>
								<AlertDescription>
									{replaceableConflictGrades.join(", ")}{" "}
									already{" "}
									{replaceableConflictGrades.length === 1
										? "has"
										: "have"}{" "}
									a schedule. Saving will replace it for those
									grades.
								</AlertDescription>
							</Alert>
						) : null}
						<Alert>
							<CalendarClockIcon />
							<AlertTitle>Review the schedule</AlertTitle>
							<AlertDescription>
								{gradeIDs.length === 0 ? (
									"Choose at least one grade to review the schedule."
								) : (
									<>
										{gradeIDs.join(", ")} will open{" "}
										{openingISO === null
											? "after you choose an opening date and time"
											: formatChinaTime(openingISO)}
										{hasClosing
											? closingISO === null
												? " and close after you choose a valid closing time."
												: ` and close ${formatChinaTime(closingISO)}.`
											: ". Access will remain open until an administrator closes it."}
									</>
								)}
							</AlertDescription>
						</Alert>
					</FieldGroup>
					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							onClick={() => onOpenChange(false)}
						>
							Cancel
						</Button>
						<Button type="submit" disabled={busy || !valid}>
							{busy ? (
								<Spinner data-icon="inline-start" />
							) : (
								<CalendarClockIcon data-icon="inline-start" />
							)}
							{replaceableConflictGrades.length > 0
								? "Replace and save"
								: "Save schedule"}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}

function GradeRequirementsCard({
	grade,
	categories,
	refresh,
}: {
	grade: Grade
	categories: readonly string[]
	refresh: () => Promise<void>
}): React.JSX.Element {
	const [requirementOpen, setRequirementOpen] = useState(false)
	return (
		<Card>
			<CardHeader>
				<CardTitle>Grade {grade.grade}</CardTitle>
				<CardDescription>
					Minimum selections across a set of categories.
				</CardDescription>
				<CardAction>
					<Button
						size="sm"
						variant="outline"
						disabled={categories.length === 0}
						onClick={() => setRequirementOpen(true)}
					>
						<PlusIcon data-icon="inline-start" />
						Add requirement
					</Button>
				</CardAction>
			</CardHeader>
			<CardContent>
				<Table containerLabel={`${grade.grade} requirement groups`}>
					<TableCaption className="sr-only">
						Requirement groups for {grade.grade}
					</TableCaption>
					<TableHeader>
						<TableRow>
							<TableHead>Categories</TableHead>
							<TableHead className="w-24">Minimum</TableHead>
							<TableHead className="w-16">
								<span className="sr-only">Actions</span>
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{grade.req_groups.length === 0 ? (
							<TableRow>
								<TableCell
									colSpan={3}
									className="h-20 text-center text-muted-foreground"
								>
									No requirements configured.
								</TableCell>
							</TableRow>
						) : (
							grade.req_groups.map((group) => (
								<TableRow key={group.id}>
									<TableCell className="whitespace-normal">
										{group.category_ids.join(" / ")}
									</TableCell>
									<TableCell className="font-medium">
										{group.min_count}
									</TableCell>
									<TableCell>
										<div className="flex justify-end">
											<DeleteButton
												name="requirement group"
												onDelete={() =>
													runMutation(
														() =>
															apiRequest(
																`/api/v1/admin/grades/${encodeURIComponent(grade.grade)}/requirement-groups/${group.id}`,
																{
																	method: "DELETE",
																},
															),
														refresh,
														"Requirement group deleted.",
													)
												}
											/>
										</div>
									</TableCell>
								</TableRow>
							))
						)}
					</TableBody>
				</Table>
			</CardContent>
			<CardFooter className="justify-end">
				<DeleteButton
					name={`Grade ${grade.grade}`}
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
	const [gradeDialogOpen, setGradeDialogOpen] = useState(false)
	const [selectedGrades, setSelectedGrades] = useState<string[]>([])
	const [scheduleDialogOpen, setScheduleDialogOpen] = useState(false)
	const [editingSchedule, setEditingSchedule] =
		useState<GradeSelectionSchedule | null>(null)
	const [pendingAccess, setPendingAccess] = useState<{
		gradeIDs: string[]
		enabled: boolean
	} | null>(null)
	const [accessBusy, setAccessBusy] = useState(false)

	useEffect(() => {
		const timer = window.setInterval(() => void refresh(), 15_000)
		return () => window.clearInterval(timer)
	}, [refresh])

	const scheduleByGrade = useMemo(() => {
		const result = new Map<string, GradeSelectionSchedule>()
		for (const schedule of data.grade_schedules) {
			for (const grade of schedule.grade_ids) result.set(grade, schedule)
		}
		return result
	}, [data.grade_schedules])
	const activeSelectedGrades = selectedGrades.filter((grade) =>
		data.grades.some((item) => item.grade === grade),
	)
	const allSelected =
		data.grades.length > 0 &&
		activeSelectedGrades.length === data.grades.length
	const someSelected = activeSelectedGrades.length > 0 && !allSelected

	async function applyAccess(
		gradeIDs: string[],
		enabled: boolean,
	): Promise<void> {
		setAccessBusy(true)
		await runMutation(
			() =>
				apiRequest("/api/v1/admin/grade-access", {
					method: "POST",
					body: jsonBody({ grade_ids: gradeIDs, enabled }),
				}),
			refresh,
			enabled ? "Selections enabled." : "Selections disabled.",
		)
		setAccessBusy(false)
		setPendingAccess(null)
	}

	function requestAccess(gradeIDs: string[], enabled: boolean): void {
		const normalized = [...new Set(gradeIDs)].sort()
		if (normalized.some((grade) => scheduleByGrade.has(grade))) {
			setPendingAccess({ gradeIDs: normalized, enabled })
			return
		}
		void applyAccess(normalized, enabled)
	}

	function openSchedule(schedule: GradeSelectionSchedule | null): void {
		setEditingSchedule(schedule)
		setScheduleDialogOpen(true)
	}

	return (
		<>
			<PageHeading
				title="Grades"
				description="Control when students can select CCAs and define grade requirements."
				action={
					<Button onClick={() => setGradeDialogOpen(true)}>
						<PlusIcon data-icon="inline-start" />
						Add grade
					</Button>
				}
			/>
			<Tabs defaultValue="access">
				<TabsList variant="line">
					<TabsTrigger value="access">Selection access</TabsTrigger>
					<TabsTrigger value="requirements">Requirements</TabsTrigger>
				</TabsList>
				<TabsContent
					value="access"
					className="flex flex-col gap-4 pt-4"
				>
					<Card>
						<CardHeader>
							<CardTitle>Selection access</CardTitle>
							<CardDescription>
								Enable grades now or schedule a future opening
								and optional closing time.
							</CardDescription>
						</CardHeader>
						<CardContent className="flex flex-col gap-4">
							<div className="flex flex-wrap items-center gap-2">
								<Button
									variant="outline"
									disabled={
										accessBusy ||
										activeSelectedGrades.length === 0
									}
									onClick={() =>
										requestAccess(
											activeSelectedGrades,
											true,
										)
									}
								>
									Enable now
								</Button>
								<Button
									variant="outline"
									disabled={
										accessBusy ||
										activeSelectedGrades.length === 0
									}
									onClick={() =>
										requestAccess(
											activeSelectedGrades,
											false,
										)
									}
								>
									Disable now
								</Button>
								<Button
									disabled={activeSelectedGrades.length === 0}
									onClick={() => openSchedule(null)}
								>
									<CalendarClockIcon data-icon="inline-start" />
									Schedule
								</Button>
								<span className="text-sm text-muted-foreground">
									{activeSelectedGrades.length === 0
										? "Select grades in the table first."
										: `${activeSelectedGrades.length} selected`}
								</span>
							</div>
							<Table containerLabel="Grade selection access">
								<TableHeader>
									<TableRow>
										<TableHead className="w-12">
											<Checkbox
												aria-label="Select all grades"
												checked={allSelected}
												indeterminate={someSelected}
												onCheckedChange={(checked) =>
													setSelectedGrades(
														checked
															? data.grades.map(
																	(grade) =>
																		grade.grade,
																)
															: [],
													)
												}
											/>
										</TableHead>
										<TableHead>Grade</TableHead>
										<TableHead>Current status</TableHead>
										<TableHead>
											Maximum own choices
										</TableHead>
										<TableHead>
											Next scheduled change
										</TableHead>
									</TableRow>
								</TableHeader>
								<TableBody>
									{data.grades.length === 0 ? (
										<TableRow>
											<TableCell
												colSpan={5}
												className="h-24 text-center text-muted-foreground"
											>
												No grades configured.
											</TableCell>
										</TableRow>
									) : (
										data.grades.map((grade) => {
											const schedule =
												scheduleByGrade.get(grade.grade)
											return (
												<TableRow
													key={grade.grade}
													data-state={
														activeSelectedGrades.includes(
															grade.grade,
														)
															? "selected"
															: undefined
													}
												>
													<TableCell>
														<Checkbox
															aria-label={`Select ${grade.grade}`}
															checked={activeSelectedGrades.includes(
																grade.grade,
															)}
															onCheckedChange={(
																checked,
															) =>
																setSelectedGrades(
																	(
																		current,
																	) =>
																		checked
																			? [
																					...current,
																					grade.grade,
																				].sort()
																			: current.filter(
																					(
																						item,
																					) =>
																						item !==
																						grade.grade,
																				),
																)
															}
														/>
													</TableCell>
													<TableCell className="font-medium">
														{grade.grade}
													</TableCell>
													<TableCell>
														<div className="flex items-center gap-3">
															<Switch
																aria-label={`${grade.enabled ? "Disable" : "Enable"} selections for ${grade.grade}`}
																checked={
																	grade.enabled
																}
																disabled={
																	accessBusy
																}
																onCheckedChange={(
																	enabled,
																) =>
																	requestAccess(
																		[
																			grade.grade,
																		],
																		enabled,
																	)
																}
															/>
															<Badge
																variant={
																	grade.enabled
																		? "default"
																		: "secondary"
																}
															>
																{grade.enabled
																	? "Open"
																	: "Closed"}
															</Badge>
														</div>
													</TableCell>
													<TableCell>
														<GradeLimitControl
															key={`${grade.grade}-${grade.max_own_choices}`}
															grade={grade}
															refresh={refresh}
														/>
													</TableCell>
													<TableCell className="whitespace-normal">
														{schedule ===
														undefined ? (
															<span className="text-muted-foreground">
																None
															</span>
														) : schedule.opened ? (
															<span>
																Closes{" "}
																{schedule.closes_at ===
																undefined
																	? "manually"
																	: formatChinaTime(
																			schedule.closes_at,
																		)}
															</span>
														) : (
															<span>
																Opens{" "}
																{formatChinaTime(
																	schedule.opens_at,
																)}
															</span>
														)}
													</TableCell>
												</TableRow>
											)
										})
									)}
								</TableBody>
							</Table>
						</CardContent>
					</Card>

					<Card>
						<CardHeader>
							<CardTitle>Upcoming schedules</CardTitle>
							<CardDescription>
								Times are shown in China Standard Time (UTC+8).
							</CardDescription>
						</CardHeader>
						<CardContent>
							<Table containerLabel="Upcoming grade selection schedules">
								<TableHeader>
									<TableRow>
										<TableHead>Grades</TableHead>
										<TableHead>Opens</TableHead>
										<TableHead>Closes</TableHead>
										<TableHead>Status</TableHead>
										<TableHead className="w-24">
											<span className="sr-only">
												Actions
											</span>
										</TableHead>
									</TableRow>
								</TableHeader>
								<TableBody>
									{data.grade_schedules.length === 0 ? (
										<TableRow>
											<TableCell
												colSpan={5}
												className="h-24 text-center text-muted-foreground"
											>
												No scheduled access changes.
											</TableCell>
										</TableRow>
									) : (
										data.grade_schedules.map((schedule) => (
											<TableRow key={schedule.batch_id}>
												<TableCell className="font-medium">
													{schedule.grade_ids.join(
														", ",
													)}
												</TableCell>
												<TableCell>
													{formatChinaTime(
														schedule.opens_at,
													)}
												</TableCell>
												<TableCell>
													{schedule.closes_at ===
													undefined
														? "Until manually closed"
														: formatChinaTime(
																schedule.closes_at,
															)}
												</TableCell>
												<TableCell>
													<Badge
														variant={
															schedule.opened
																? "default"
																: "secondary"
														}
													>
														{schedule.opened
															? "Open"
															: "Pending"}
													</Badge>
												</TableCell>
												<TableCell>
													<div className="flex justify-end gap-1">
														<Tooltip>
															<TooltipTrigger
																render={
																	<Button
																		variant="ghost"
																		size="icon-sm"
																		aria-label="Edit schedule"
																		onClick={() =>
																			openSchedule(
																				schedule,
																			)
																		}
																	/>
																}
															>
																<PencilIcon />
															</TooltipTrigger>
															<TooltipContent>
																Edit schedule
															</TooltipContent>
														</Tooltip>
														<DeleteButton
															name={`schedule for ${schedule.grade_ids.join(", ")}`}
															description={
																schedule.opened
																	? "Cancelling stops the automatic closing time. Selection access remains open until an administrator closes it."
																	: "Cancelling removes this future opening and closing time."
															}
															onDelete={() =>
																runMutation(
																	() =>
																		apiRequest(
																			`/api/v1/admin/grade-schedules/${schedule.batch_id}`,
																			{
																				method: "DELETE",
																			},
																		),
																	refresh,
																	"Selection schedule cancelled.",
																)
															}
														/>
													</div>
												</TableCell>
											</TableRow>
										))
									)}
								</TableBody>
							</Table>
						</CardContent>
					</Card>
				</TabsContent>
				<TabsContent value="requirements" className="pt-4">
					{data.grades.length === 0 ? (
						<Card>
							<CardContent>
								<NoResults
									title="No grades configured"
									description="Add a grade to configure requirements."
								/>
							</CardContent>
						</Card>
					) : (
						<div className="grid gap-4 lg:grid-cols-2">
							{data.grades.map((grade) => (
								<GradeRequirementsCard
									key={grade.grade}
									grade={grade}
									categories={data.categories}
									refresh={refresh}
								/>
							))}
						</div>
					)}
				</TabsContent>
			</Tabs>
			<GradeDialog
				open={gradeDialogOpen}
				onOpenChange={setGradeDialogOpen}
				refresh={refresh}
			/>
			{scheduleDialogOpen ? (
				<GradeScheduleDialog
					open
					onOpenChange={setScheduleDialogOpen}
					grades={data.grades}
					schedules={data.grade_schedules}
					initialGradeIDs={
						editingSchedule?.grade_ids ?? activeSelectedGrades
					}
					editing={editingSchedule}
					refresh={refresh}
				/>
			) : null}
			<AlertDialog
				open={pendingAccess !== null}
				onOpenChange={(open) => {
					if (!open && !accessBusy) setPendingAccess(null)
				}}
			>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogMedia>
							<CalendarClockIcon />
						</AlertDialogMedia>
						<AlertDialogTitle>
							Override the existing schedule?
						</AlertDialogTitle>
						<AlertDialogDescription>
							Changing access now will cancel the scheduled
							opening or closing for{" "}
							{pendingAccess?.gradeIDs.join(", ")}. You can create
							a new schedule afterwards.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel disabled={accessBusy}>
							Keep schedule
						</AlertDialogCancel>
						<AlertDialogAction
							disabled={accessBusy}
							onClick={() => {
								if (pendingAccess !== null)
									void applyAccess(
										pendingAccess.gradeIDs,
										pendingAccess.enabled,
									)
							}}
						>
							{accessBusy ? (
								<Spinner data-icon="inline-start" />
							) : null}
							Override now
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
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
	const selectedPeriodCounts = useMemo(() => {
		const counts = new Map<string, number>()
		if (course === null) return counts
		for (const selection of data.selections) {
			if (selection.course_id !== course.id) continue
			counts.set(
				selection.period_id,
				(counts.get(selection.period_id) ?? 0) + 1,
			)
		}
		return counts
	}, [course, data.selections])

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
			<DialogContent className="h-[calc(100dvh-2rem)] max-h-[46rem] overflow-hidden p-0 sm:max-w-4xl">
				<form
					className="grid h-full min-h-0 grid-rows-[auto_minmax(0,1fr)_auto]"
					onSubmit={(event) => void submit(event)}
				>
					<DialogHeader className="px-6 pt-6 pb-4 pr-12">
						<DialogTitle>
							{course === null
								? "Create course"
								: `Edit ${course.name}`}
						</DialogTitle>
						<DialogDescription>
							Update course details, timetable, and student
							eligibility.
						</DialogDescription>
					</DialogHeader>
					<ScrollArea className="min-h-0">
						<FieldGroup className="gap-6 px-6 py-5">
							<FieldSet>
								<FieldLegend>Course details</FieldLegend>
								<FieldDescription>
									Core information shown in the student
									catalogue.
								</FieldDescription>
								<FieldGroup className="grid gap-4 md:grid-cols-6">
									<Field
										className="md:col-span-2"
										data-disabled={course !== null}
									>
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
									<Field className="md:col-span-4">
										<FieldLabel htmlFor="course-name">
											Name
										</FieldLabel>
										<Input
											id="course-name"
											value={form.name}
											onChange={(event) =>
												update(
													"name",
													event.target.value,
												)
											}
											required
										/>
									</Field>
									<Field className="md:col-span-3">
										<FieldLabel htmlFor="course-teacher">
											Teacher
										</FieldLabel>
										<Input
											id="course-teacher"
											value={form.teacher}
											onChange={(event) =>
												update(
													"teacher",
													event.target.value,
												)
											}
											required
										/>
									</Field>
									<Field className="md:col-span-3">
										<FieldLabel htmlFor="course-location">
											Location
										</FieldLabel>
										<Input
											id="course-location"
											value={form.location}
											onChange={(event) =>
												update(
													"location",
													event.target.value,
												)
											}
											required
										/>
									</Field>
									<Field className="md:col-span-2">
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
									<Field className="md:col-span-2">
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
									<Field className="md:col-span-2">
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
											Use 0 to prevent normal student
											selections.
										</FieldDescription>
									</Field>
									<Field className="md:col-span-6">
										<FieldLabel htmlFor="course-description">
											Description
										</FieldLabel>
										<Textarea
											id="course-description"
											value={form.description}
											onChange={(event) =>
												update(
													"description",
													event.target.value,
												)
											}
											rows={3}
										/>
									</Field>
								</FieldGroup>
							</FieldSet>

							<Separator />
							<FieldSet>
								<FieldLegend>Schedule</FieldLegend>
								<FieldDescription>
									Select one or more fixed CCA slots. Slots
									with student selections remain locked.
								</FieldDescription>
								<FieldGroup className="grid gap-x-6 gap-y-5 sm:grid-cols-2 md:grid-cols-4">
									{CCA_DAYS.map((day) => (
										<FieldSet key={day}>
											<FieldLegend variant="label">
												{day}
											</FieldLegend>
											<FieldGroup data-slot="checkbox-group">
												{CCA_SLOTS_PER_DAY.map(
													(slot) => {
														const period =
															ccaTimeSlotID(
																day,
																slot,
															)
														if (
															!data.periods.includes(
																period,
															)
														) {
															return null
														}
														const selectionCount =
															selectedPeriodCounts.get(
																period,
															) ?? 0
														const locked =
															selectionCount > 0

														return (
															<Field
																key={period}
																orientation="horizontal"
																data-disabled={
																	locked ||
																	undefined
																}
															>
																<Checkbox
																	id={domID(
																		"course-period",
																		period,
																	)}
																	aria-label={
																		period
																	}
																	checked={form.period_ids.includes(
																		period,
																	)}
																	disabled={
																		locked
																	}
																	onCheckedChange={(
																		checked,
																	) =>
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
																	CCA {slot}
																</FieldLabel>
																{locked ? (
																	<Badge variant="secondary">
																		{
																			selectionCount
																		}{" "}
																		selected
																	</Badge>
																) : null}
															</Field>
														)
													},
												)}
											</FieldGroup>
										</FieldSet>
									))}
								</FieldGroup>
							</FieldSet>

							<Separator />
							<FieldSet>
								<FieldLegend>Eligibility</FieldLegend>
								<FieldDescription>
									Leave a group empty to allow everyone in
									that group.
								</FieldDescription>
								<FieldGroup className="grid gap-6 sm:grid-cols-2">
									<FieldSet>
										<FieldLegend variant="label">
											Legal sex
										</FieldLegend>
										<FieldGroup
											data-slot="checkbox-group"
											className="grid grid-cols-3"
										>
											{(
												["F", "M", "X"] as LegalSex[]
											).map((legalSex) => (
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
											))}
										</FieldGroup>
									</FieldSet>
									<FieldSet>
										<FieldLegend variant="label">
											Grades
										</FieldLegend>
										<FieldGroup
											data-slot="checkbox-group"
											className="grid grid-cols-3"
										>
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
														onCheckedChange={(
															checked,
														) =>
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
														G{grade.grade}
													</FieldLabel>
												</Field>
											))}
										</FieldGroup>
									</FieldSet>
								</FieldGroup>
							</FieldSet>
						</FieldGroup>
					</ScrollArea>
					<DialogFooter className="mx-0 mb-0 rounded-none px-6 py-4">
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
					<CardDescription aria-live="polite" aria-atomic="true">
						{query.trim() === ""
							? `${data.courses.length} course${data.courses.length === 1 ? "" : "s"} configured`
							: `${courses.length} of ${data.courses.length} course${data.courses.length === 1 ? "" : "s"}`}
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
									<TableHead className="sticky right-0 min-w-20 bg-background text-right">
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
											<Badge
												variant={
													course.max_students === 0 ||
													course.current_students >=
														course.max_students
														? "destructive"
														: "outline"
												}
											>
												{course.max_students === 0
													? "Closed"
													: `${course.current_students}/${course.max_students}`}
											</Badge>
										</TableCell>
										<TableCell className="sticky right-0 bg-background">
											<div className="flex justify-end gap-1">
												<Button
													variant="ghost"
													size="icon-sm"
													aria-label={`Edit ${course.name}`}
													onClick={() =>
														openCourse(course)
													}
												>
													<PencilIcon />
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

export function StudentDialog({
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
		<ScrollArea className="h-52 rounded-lg border">
			<FieldGroup data-slot="checkbox-group" className="p-2 pr-4">
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
		</ScrollArea>
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

type DataFileFormat = "csv" | "xlsx"

type DataFormatOption = {
	value: DataFileFormat
	label: string
	accept: string
}

const CSV_DATA_FORMAT: DataFormatOption = {
	value: "csv",
	label: "CSV (.csv)",
	accept: ".csv,text/csv",
}

const EXCEL_DATA_FORMAT: DataFormatOption = {
	value: "xlsx",
	label: "Excel (.xlsx)",
	accept: ".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
}

const DATA_FORMAT_OPTIONS: DataFormatOption[] = [
	CSV_DATA_FORMAT,
	EXCEL_DATA_FORMAT,
]

const DEFAULT_DATA_FORMAT = CSV_DATA_FORMAT

function DataFormatCombobox({
	id,
	value,
	onValueChange,
}: {
	id: string
	value: DataFormatOption
	onValueChange: (value: DataFormatOption) => void
}): React.JSX.Element {
	return (
		<Combobox
			items={DATA_FORMAT_OPTIONS}
			value={value}
			onValueChange={(nextValue) => {
				if (nextValue !== null) onValueChange(nextValue)
			}}
			itemToStringLabel={(item) => item.label}
			itemToStringValue={(item) => item.value}
			isItemEqualToValue={(item, selected) =>
				item.value === selected.value
			}
		>
			<ComboboxInput id={id} placeholder="Choose a format" />
			<ComboboxContent>
				<ComboboxEmpty>No formats found.</ComboboxEmpty>
				<ComboboxList>
					{(item) => (
						<ComboboxItem key={item.value} value={item}>
							{item.label}
						</ComboboxItem>
					)}
				</ComboboxList>
			</ComboboxContent>
		</Combobox>
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
	const [format, setFormat] = useState(DEFAULT_DATA_FORMAT)
	const formID = domID("import-form", title)
	const formatID = domID("format", title)
	const fileID = domID("import", title)

	return (
		<Card>
			<CardHeader>
				<CardTitle>{title}</CardTitle>
				<CardDescription>{description}</CardDescription>
			</CardHeader>
			<CardContent>
				<form
					id={formID}
					action={action}
					method="post"
					encType="multipart/form-data"
				>
					<FieldGroup>
						<Field>
							<FieldLabel htmlFor={formatID}>Format</FieldLabel>
							<DataFormatCombobox
								id={formatID}
								value={format}
								onValueChange={setFormat}
							/>
						</Field>
						<Field>
							<FieldLabel htmlFor={fileID}>
								{format.label} file
							</FieldLabel>
							<Input
								key={format.value}
								id={fileID}
								type="file"
								name="file"
								accept={format.accept}
								required
							/>
						</Field>
						<input
							type="hidden"
							name="format"
							value={format.value}
						/>
					</FieldGroup>
				</form>
			</CardContent>
			<CardFooter className="flex-wrap gap-2">
				<Button type="submit" form={formID} variant="outline">
					<UploadIcon data-icon="inline-start" />
					Import {format.value === "csv" ? "CSV" : "Excel"}
				</Button>
				<Button
					variant="link"
					render={<a href={`${example}?format=${format.value}`} />}
					nativeButton={false}
				>
					<DownloadIcon data-icon="inline-start" />
					Download {format.value === "csv" ? "CSV" : "Excel"} example
				</Button>
			</CardFooter>
		</Card>
	)
}

function SelectionExportCard({ count }: { count: number }): React.JSX.Element {
	const [format, setFormat] = useState(DEFAULT_DATA_FORMAT)

	return (
		<Card>
			<CardHeader>
				<CardTitle>Selection export</CardTitle>
				<CardDescription>
					Download all {count} selection{count === 1 ? "" : "s"},
					including every fixed time slot for each course.
				</CardDescription>
			</CardHeader>
			<CardContent>
				<FieldGroup>
					<Field>
						<FieldLabel htmlFor="selection-export-format">
							Format
						</FieldLabel>
						<DataFormatCombobox
							id="selection-export-format"
							value={format}
							onValueChange={setFormat}
						/>
					</Field>
				</FieldGroup>
			</CardContent>
			<CardFooter>
				<Button
					render={
						<a
							href={`/admin/selections/export?format=${format.value}`}
						/>
					}
					nativeButton={false}
				>
					<DownloadIcon data-icon="inline-start" />
					Download {format.value === "csv" ? "CSV" : "Excel"}
				</Button>
			</CardFooter>
		</Card>
	)
}

type DataResetScope = "selections" | "courses" | "students"

type DataResetDefinition = {
	scope: DataResetScope
	title: string
	description: string
	confirmation: string
	count: number
	blocked: boolean
	blockedReason: string | undefined
}

type AdminResetResult = {
	scope: DataResetScope
	deleted_count: number
	closed_grade_count: number
}

function DataResetCard({
	definition,
	refresh,
}: {
	definition: DataResetDefinition
	refresh: () => Promise<void>
}): React.JSX.Element {
	const [open, setOpen] = useState(false)
	const [confirmation, setConfirmation] = useState("")
	const [busy, setBusy] = useState(false)
	const confirmationMatches = confirmation === definition.confirmation
	const confirmationInvalid = confirmation.length > 0 && !confirmationMatches
	const confirmationID = `reset-${definition.scope}-confirmation`

	function changeOpen(nextOpen: boolean): void {
		if (busy) return
		setOpen(nextOpen)
		if (!nextOpen) setConfirmation("")
	}

	async function resetData(): Promise<void> {
		if (definition.blocked || !confirmationMatches || busy) {
			return
		}

		setBusy(true)
		try {
			const result = await apiRequest<AdminResetResult>(
				"/api/v1/admin/reset",
				{
					method: "POST",
					body: jsonBody({
						scope: definition.scope,
						confirmation,
					}),
				},
			)
			await refresh()
			toast.success(
				`${result.deleted_count} ${result.scope} reset. Selection windows closed for ${result.closed_grade_count} grade${result.closed_grade_count === 1 ? "" : "s"}.`,
			)
			setOpen(false)
			setConfirmation("")
		} catch (caught) {
			toast.error(
				caught instanceof Error ? caught.message : "The reset failed.",
			)
		} finally {
			setBusy(false)
		}
	}

	return (
		<Card>
			<CardHeader>
				<CardTitle>{definition.title}</CardTitle>
				<CardDescription>{definition.description}</CardDescription>
				<CardAction>
					<Badge variant="secondary">
						{definition.count.toLocaleString()}
					</Badge>
				</CardAction>
			</CardHeader>
			<CardContent className="flex-1">
				<p className="text-sm text-muted-foreground">
					This also closes selection for every grade. Grades,
					categories, requirements, fixed time slots, and admin
					accounts are preserved.
				</p>
				{definition.blockedReason ? (
					<p className="mt-3 text-sm font-medium">
						{definition.blockedReason}
					</p>
				) : null}
			</CardContent>
			<CardFooter>
				<AlertDialog open={open} onOpenChange={changeOpen}>
					<AlertDialogTrigger
						render={
							<Button
								variant="destructive"
								disabled={definition.blocked}
							/>
						}
					>
						<Trash2Icon data-icon="inline-start" />
						{definition.title}
					</AlertDialogTrigger>
					<AlertDialogContent>
						<AlertDialogHeader>
							<AlertDialogMedia>
								<Trash2Icon aria-hidden="true" />
							</AlertDialogMedia>
							<AlertDialogTitle>
								{definition.title}?
							</AlertDialogTitle>
							<AlertDialogDescription>
								This permanently deletes{" "}
								{definition.count.toLocaleString()}{" "}
								{definition.scope}. It cannot be undone and no
								backup is created.
							</AlertDialogDescription>
						</AlertDialogHeader>
						<Field data-invalid={confirmationInvalid || undefined}>
							<FieldLabel htmlFor={confirmationID}>
								Type {definition.confirmation} to confirm
							</FieldLabel>
							<Input
								id={confirmationID}
								value={confirmation}
								onChange={(event) =>
									setConfirmation(event.target.value)
								}
								autoComplete="off"
								spellCheck={false}
								aria-invalid={confirmationInvalid || undefined}
								disabled={busy}
							/>
							<FieldDescription>
								The phrase is case-sensitive and must match
								exactly.
							</FieldDescription>
						</Field>
						<AlertDialogFooter>
							<AlertDialogCancel disabled={busy}>
								Cancel
							</AlertDialogCancel>
							<AlertDialogAction
								variant="destructive"
								disabled={!confirmationMatches || busy}
								onClick={() => void resetData()}
							>
								{busy ? (
									<Spinner data-icon="inline-start" />
								) : (
									<Trash2Icon data-icon="inline-start" />
								)}
								Reset
							</AlertDialogAction>
						</AlertDialogFooter>
					</AlertDialogContent>
				</AlertDialog>
			</CardFooter>
		</Card>
	)
}

export function DataManagementPage({
	data,
	refresh,
}: AdminPageProps): React.JSX.Element {
	const hasSelections = data.selections.length > 0
	const resetDefinitions: DataResetDefinition[] = [
		{
			scope: "selections",
			title: "Reset selections",
			description:
				"Delete all normal, invited, and forced student selections.",
			confirmation: "RESET SELECTIONS",
			count: data.selections.length,
			blocked: false,
			blockedReason: undefined,
		},
		{
			scope: "courses",
			title: "Reset courses",
			description:
				"Delete every CCA and its timetable and eligibility settings.",
			confirmation: "RESET COURSES",
			count: data.courses.length,
			blocked: hasSelections,
			blockedReason: hasSelections
				? `Reset all ${data.selections.length.toLocaleString()} selections first.`
				: undefined,
		},
		{
			scope: "students",
			title: "Reset students",
			description:
				"Delete every student profile and invalidate all student sessions.",
			confirmation: "RESET STUDENTS",
			count: data.students.length,
			blocked: hasSelections,
			blockedReason: hasSelections
				? `Reset all ${data.selections.length.toLocaleString()} selections first.`
				: undefined,
		},
	]

	return (
		<>
			<PageHeading
				title="Data management"
				description="Import, export, or reset operational data."
			/>
			<Tabs defaultValue="transfers">
				<TabsList variant="line">
					<TabsTrigger value="transfers">
						Import &amp; Export
					</TabsTrigger>
					<TabsTrigger value="reset">Reset</TabsTrigger>
				</TabsList>
				<Separator />
				<TabsContent value="transfers" className="pt-4">
					<div className="grid gap-4 lg:grid-cols-3">
						<SelectionExportCard count={data.selections.length} />
						<ImportCard
							title="Course import"
							description="Bulk-create courses from a compatible CSV or Excel file. Existing IDs are rejected."
							action="/admin/courses/import"
							example="/admin/data/examples/courses"
						/>
						<ImportCard
							title="Student import"
							description="Bulk-create student profiles from a compatible CSV or Excel file. Existing IDs are rejected."
							action="/admin/students/import"
							example="/admin/data/examples/students"
						/>
						<ImportCard
							title="Selection import"
							description="Bulk-create normal, invited, or forced selections from CSV or Excel. The entire file is transactional."
							action="/admin/selections/import"
							example="/admin/data/examples/selections"
						/>
					</div>
				</TabsContent>
				<TabsContent value="reset" className="pt-4">
					<div className="grid items-stretch gap-4 lg:grid-cols-3">
						{resetDefinitions.map((definition) => (
							<DataResetCard
								key={definition.scope}
								definition={definition}
								refresh={refresh}
							/>
						))}
					</div>
				</TabsContent>
			</Tabs>
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
