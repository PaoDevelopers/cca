import {
	CalendarClockIcon,
	CalendarDaysIcon,
	PencilIcon,
	PlusIcon,
	SaveIcon,
} from "lucide-react"
import { useEffect, useMemo, useState } from "react"

import { adminRequest, jsonBody } from "@/api"
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
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
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
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "@/components/ui/popover"
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip"
import type { AdminPageProps } from "@/features/admin/app/AdminApp"
import {
	DeleteButton,
	NoResults,
	PageHeading,
} from "@/features/admin/components/AdminPagePrimitives"
import { domID, runMutation } from "@/features/admin/lib/page-utils"
import type { Grade, GradeSelectionSchedule } from "@/types"

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
				adminRequest("/grades", {
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
				adminRequest(
					`/grades/${encodeURIComponent(grade)}/requirement-groups`,
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
				adminRequest(`/grades/${encodeURIComponent(grade.grade)}`, {
					method: "PUT",
					body: jsonBody({
						max_own_choices: Number(limit),
					}),
				}),
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
				adminRequest(
					editing === null
						? "/grade-schedules"
						: `/grade-schedules/${editing.batch_id}`,
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
															adminRequest(
																`/grades/${encodeURIComponent(grade.grade)}/requirement-groups/${group.id}`,
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
								adminRequest(
									`/grades/${encodeURIComponent(grade.grade)}`,
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
				adminRequest("/grade-access", {
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
																		adminRequest(
																			`/grade-schedules/${schedule.batch_id}`,
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
