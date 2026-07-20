import { type DragEvent, type KeyboardEvent, useMemo, useState } from "react"
import { CheckIcon, ClockAlertIcon, XIcon } from "lucide-react"

import { BlockReasonBadges, PeriodBadges } from "@/components/common"
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
import {
	Empty,
	EmptyDescription,
	EmptyHeader,
	EmptyMedia,
	EmptyTitle,
} from "@/components/ui/empty"
import { Spinner } from "@/components/ui/spinner"
import { Separator } from "@/components/ui/separator"
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
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select"
import {
	CCA_DAYS,
	CCA_SLOTS_PER_DAY,
	ccaTimeSlotID,
	FIXED_CCA_TIME_SLOTS,
	formatCCATimeSlotLabel,
	type CCADay,
	type CCASlotNumber,
} from "@/lib/cca-schedule"
import { cn } from "@/lib/utils"
import type { Course } from "@/types"

export type CatalogLayout = "cards" | "list" | "timetable"

interface CourseViewProps {
	courses: Course[]
	busyCourseID: string | null
	onToggle: (course: Course, periodID?: string) => void
}

interface CourseActionProps {
	course: Course
	busyCourseID: string | null
	onToggle: (course: Course, periodID?: string) => void
	size?: "default" | "sm"
}

function CourseAction({
	course,
	busyCourseID,
	onToggle,
	size = "default",
}: CourseActionProps): React.JSX.Element {
	const busy = busyCourseID === course.id
	const unavailable = !course.selected && !course.available
	const disabled =
		busyCourseID !== null ||
		unavailable ||
		(course.selected && !course.removable)
	const label = busy
		? course.selected
			? "Removing…"
			: "Selecting…"
		: course.selected
			? course.removable
				? "Remove"
				: "Locked"
			: "Select"
	const accessibleLabel = busy
		? `${course.selected ? "Removing" : "Selecting"} ${course.name}`
		: course.selected
			? course.removable
				? `Remove ${course.name}`
				: `${course.name} is locked`
			: unavailable
				? `${course.name} is unavailable`
				: `Select ${course.name}`

	return (
		<Button
			variant={
				course.selected
					? "secondary"
					: unavailable
						? "outline"
						: "default"
			}
			size={size}
			disabled={disabled || course.selection_type === "force"}
			onClick={() => onToggle(course)}
			aria-label={accessibleLabel}
			aria-busy={busy}
		>
			{busy ? (
				<Spinner data-icon="inline-start" aria-hidden="true" />
			) : course.selected ? (
				<CheckIcon data-icon="inline-start" />
			) : null}
			{label}
		</Button>
	)
}

interface CourseStatusBadge {
	label: string
	variant: "destructive" | "outline" | "secondary"
}

function courseIsFull(course: Course): boolean {
	return (
		course.current_students >= course.max_students ||
		course.block_reasons.some((reason) => reason.code === "course_full")
	)
}

function getSpecificBlockReasons(course: Course): Course["block_reasons"] {
	return course.block_reasons.filter(
		(reason) => reason.code !== "course_full",
	)
}

function getSelectionContextBadges(course: Course): CourseStatusBadge[] {
	if (!course.selected) return []

	const badges: CourseStatusBadge[] = []
	if (!course.removable) {
		badges.push({ label: "Locked", variant: "secondary" })
	}
	if (course.selection_type === "invite") {
		badges.push({ label: "Invited", variant: "outline" })
	}
	if (course.selection_type === "force") {
		badges.push({ label: "Required", variant: "outline" })
	}
	return badges
}

function getListStatusBadges(
	course: Course,
	specificReasons: Course["block_reasons"],
): CourseStatusBadge[] {
	const isFull = courseIsFull(course)
	const badges: CourseStatusBadge[] = []

	if (course.selected) {
		badges.push({ label: "Selected", variant: "secondary" })
	} else if (course.available) {
		badges.push({ label: "Available", variant: "secondary" })
	} else if (!isFull && specificReasons.length === 0) {
		badges.push({ label: "Unavailable", variant: "outline" })
	}

	if (isFull) {
		badges.push({ label: "Full", variant: "destructive" })
	}
	badges.push(...getSelectionContextBadges(course))
	return badges
}

function CourseStatusBadges({
	badges,
}: {
	badges: CourseStatusBadge[]
}): React.JSX.Element {
	return (
		<>
			{badges.map((badge) => (
				<Badge key={badge.label} variant={badge.variant}>
					{badge.label}
				</Badge>
			))}
		</>
	)
}

function CompactCourseState({
	course,
}: {
	course: Course
}): React.JSX.Element | null {
	if (course.selected) {
		const badges = getSelectionContextBadges(course)
		return badges.length > 0 ? (
			<div className="flex flex-wrap gap-1.5">
				<CourseStatusBadges badges={badges} />
			</div>
		) : null
	}

	const specificReasons = getSpecificBlockReasons(course)
	return specificReasons.length > 0 ? (
		<BlockReasonBadges reasons={specificReasons} />
	) : null
}

function CourseListStatus({ course }: { course: Course }): React.JSX.Element {
	const specificReasons = getSpecificBlockReasons(course)
	const badges = getListStatusBadges(course, specificReasons)

	return (
		<div className="flex flex-wrap items-center gap-1.5">
			<CourseStatusBadges badges={badges} />
			{specificReasons.length > 0 ? (
				<BlockReasonBadges reasons={specificReasons} />
			) : null}
		</div>
	)
}

function EnrollmentBadge({ course }: { course: Course }): React.JSX.Element {
	return (
		<Badge
			className="tabular-nums"
			variant={
				course.current_students >= course.max_students
					? "destructive"
					: "outline"
			}
		>
			{course.current_students}/{course.max_students}
		</Badge>
	)
}

function CourseCard({
	course,
	busyCourseID,
	onToggle,
	mode = "catalog",
	isPreviewed = false,
	onPreview,
	onHover,
	onDragStart,
	onDragEnd,
}: {
	course: Course
	busyCourseID: string | null
	onToggle: (course: Course, periodID?: string) => void
	mode?: "catalog" | "timetable"
	isPreviewed?: boolean
	onPreview?: (courseID: string) => void
	onHover?: (courseID: string | null) => void
	onDragStart?: (event: DragEvent<HTMLDivElement>, course: Course) => void
	onDragEnd?: () => void
}): React.JSX.Element {
	const isTimetableCandidate = mode === "timetable"
	const displayedPeriodIDs =
		course.selected && course.selected_period_id
			? [course.selected_period_id]
			: course.period_ids
	const canDrag =
		isTimetableCandidate && course.available && busyCourseID === null

	const previewCourse = (): void => {
		if (isTimetableCandidate) onPreview?.(course.id)
	}

	return (
		<Card
			className={cn(
				"h-full",
				isPreviewed && "ring-2 ring-ring",
				canDrag && "cursor-grab active:cursor-grabbing",
			)}
			draggable={canDrag}
			role={isTimetableCandidate ? "button" : undefined}
			tabIndex={isTimetableCandidate ? 0 : undefined}
			aria-label={
				isTimetableCandidate
					? `Preview ${course.name} in your timetable`
					: undefined
			}
			onClick={previewCourse}
			onKeyDown={(event) => {
				if (
					!isTimetableCandidate ||
					(event.key !== "Enter" && event.key !== " ")
				) {
					return
				}
				event.preventDefault()
				previewCourse()
			}}
			onFocus={previewCourse}
			onMouseEnter={() => {
				if (isTimetableCandidate) onHover?.(course.id)
			}}
			onMouseLeave={() => {
				if (isTimetableCandidate) onHover?.(null)
			}}
			onDragStart={(event) => {
				if (canDrag) onDragStart?.(event, course)
			}}
			onDragEnd={onDragEnd}
		>
			<CardHeader>
				<CardTitle className="min-w-0 break-words">
					{course.name}
				</CardTitle>
				<CardDescription className="min-w-0 break-words">
					{course.category_id} · {course.id}
				</CardDescription>
				{isTimetableCandidate ? null : (
					<CardAction>
						<CourseAction
							course={course}
							busyCourseID={busyCourseID}
							onToggle={onToggle}
						/>
					</CardAction>
				)}
			</CardHeader>
			<CardContent className="flex flex-1 flex-col gap-4">
				<div className="flex flex-wrap items-center gap-2">
					<PeriodBadges periodIDs={displayedPeriodIDs} />
					<CompactCourseState course={course} />
				</div>
				{course.selected && !course.removable ? (
					<Badge variant="secondary">
						{course.removal_block_reason ??
							"This selection is locked."}
					</Badge>
				) : null}
				<p className="break-words text-sm leading-relaxed text-muted-foreground">
					{course.description || "No description has been provided."}
				</p>
				<dl className="grid grid-cols-2 gap-4 text-sm">
					<div className="flex min-w-0 flex-col gap-1">
						<dt className="text-xs text-muted-foreground">
							Teacher
						</dt>
						<dd className="break-words font-medium">
							{course.teacher}
						</dd>
					</div>
					<div className="flex min-w-0 flex-col gap-1">
						<dt className="text-xs text-muted-foreground">
							Location
						</dt>
						<dd className="break-words font-medium">
							{course.location}
						</dd>
					</div>
				</dl>
			</CardContent>
			<CardFooter className="mt-auto justify-between gap-3">
				<span className="text-muted-foreground">Enrollment</span>
				<EnrollmentBadge course={course} />
			</CardFooter>
		</Card>
	)
}

function TimetableSelectedCourse({
	course,
	busyCourseID,
	onToggle,
}: {
	course: Course
	busyCourseID: string | null
	onToggle: (course: Course, periodID?: string) => void
}): React.JSX.Element {
	const busy = busyCourseID === course.id
	const removable = course.removable && course.selection_type !== "force"

	return (
		<Card size="sm" className="gap-0 py-0">
			<CardHeader className="flex h-8 items-center gap-1 px-2">
				<CardTitle className="min-w-0 flex-1 text-left text-xs">
					<Tooltip>
						<TooltipTrigger
							render={
								<span className="block truncate text-left" />
							}
						>
							{course.name}
						</TooltipTrigger>
						<TooltipContent>{course.name}</TooltipContent>
					</Tooltip>
				</CardTitle>
				{removable ? (
					<CardAction className="self-center">
						<Tooltip>
							<TooltipTrigger
								render={
									<Button
										variant="ghost"
										size="icon-xs"
										disabled={busyCourseID !== null}
										onClick={() => onToggle(course)}
										aria-label={`Remove ${course.name}`}
										aria-busy={busy}
									/>
								}
							>
								{busy ? (
									<Spinner
										data-icon="inline-start"
										aria-hidden="true"
									/>
								) : (
									<XIcon
										data-icon="inline-start"
										aria-hidden="true"
									/>
								)}
							</TooltipTrigger>
							<TooltipContent>
								Remove {course.name}
							</TooltipContent>
						</Tooltip>
					</CardAction>
				) : null}
			</CardHeader>
		</Card>
	)
}

export function CourseCardGrid({
	courses,
	busyCourseID,
	onToggle,
}: CourseViewProps): React.JSX.Element {
	return (
		<div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
			{courses.map((course) => (
				<CourseCard
					key={course.id}
					course={course}
					busyCourseID={busyCourseID}
					onToggle={onToggle}
				/>
			))}
		</div>
	)
}

export function CourseList({
	courses,
	busyCourseID,
	onToggle,
}: CourseViewProps): React.JSX.Element {
	return (
		<Card>
			<CardHeader>
				<CardTitle>Catalogue list</CardTitle>
				<CardDescription>
					Compare schedules, availability, and enrollment in one
					compact view.
				</CardDescription>
			</CardHeader>
			<CardContent className="px-0">
				<Table containerLabel="CCA catalogue table">
					<TableCaption>
						Showing {courses.length} matching CCA
						{courses.length === 1 ? "" : "s"}.
					</TableCaption>
					<TableHeader>
						<TableRow>
							<TableHead>CCA</TableHead>
							<TableHead>Schedule</TableHead>
							<TableHead>Teacher & location</TableHead>
							<TableHead>Enrollment</TableHead>
							<TableHead>Status</TableHead>
							<TableHead className="text-right">Action</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{courses.map((course) => (
							<TableRow
								key={course.id}
								data-state={
									course.selected ? "selected" : undefined
								}
							>
								<TableCell className="min-w-64 whitespace-normal">
									<div className="flex min-w-0 flex-col gap-1">
										<span className="break-words font-medium">
											{course.name}
										</span>
										<span className="break-all text-xs text-muted-foreground">
											{course.id} · {course.category_id}
										</span>
										<span className="line-clamp-2 text-xs text-muted-foreground">
											{course.description ||
												"No description has been provided."}
										</span>
									</div>
								</TableCell>
								<TableCell className="min-w-48 whitespace-normal">
									<PeriodBadges
										periodIDs={
											course.selected &&
											course.selected_period_id
												? [course.selected_period_id]
												: course.period_ids
										}
									/>
								</TableCell>
								<TableCell className="min-w-48 whitespace-normal">
									<div className="flex min-w-0 flex-col gap-1">
										<span className="break-words">
											{course.teacher}
										</span>
										<span className="break-words text-xs text-muted-foreground">
											{course.location}
										</span>
									</div>
								</TableCell>
								<TableCell>
									<EnrollmentBadge course={course} />
								</TableCell>
								<TableCell className="min-w-48 whitespace-normal">
									<CourseListStatus course={course} />
								</TableCell>
								<TableCell className="text-right">
									<CourseAction
										course={course}
										busyCourseID={busyCourseID}
										onToggle={onToggle}
										size="sm"
									/>
								</TableCell>
							</TableRow>
						))}
					</TableBody>
				</Table>
			</CardContent>
		</Card>
	)
}

interface CategoryItem {
	label: string
	value: string
}

type TimetableDayFilter = "all" | CCADay
type TimetableSlotFilter = "all" | `${CCASlotNumber}`

const TIMETABLE_DAY_FILTERS: readonly TimetableDayFilter[] = [
	"all",
	...CCA_DAYS,
]
const TIMETABLE_SLOT_FILTERS: readonly TimetableSlotFilter[] = [
	"all",
	...CCA_SLOTS_PER_DAY.map((slot) => String(slot) as TimetableSlotFilter),
]

interface CourseTimetableProps extends CourseViewProps {
	selectedCourses: Course[]
	dayFilter: TimetableDayFilter
	onDayFilterChange: (day: TimetableDayFilter) => void
	slotFilter: TimetableSlotFilter
	onSlotFilterChange: (slot: TimetableSlotFilter) => void
	categoryItems: CategoryItem[]
	categoryFilter: string
	onCategoryFilterChange: (category: string) => void
	availableOnly: boolean
	onAvailableOnlyChange: (availableOnly: boolean) => void
}

function TimetableSlotTabs({
	day,
	slot,
	onSlotChange,
}: {
	day: TimetableDayFilter
	slot: TimetableSlotFilter
	onSlotChange: (slot: TimetableSlotFilter) => void
}): React.JSX.Element {
	const dayLabel = day === "all" ? "All days" : day

	return (
		<Tabs
			value={slot}
			onValueChange={(value) =>
				onSlotChange(value as TimetableSlotFilter)
			}
			className="min-w-0 gap-0"
		>
			<div className="relative">
				<Separator
					aria-hidden="true"
					className="absolute bottom-1 left-0"
				/>
				<div className="no-scrollbar overflow-x-auto overflow-y-hidden pb-2">
					<TabsList
						activateOnFocus
						variant="line"
						aria-label={`${dayLabel} CCA slot`}
						className="h-10 min-w-max justify-start"
					>
						{TIMETABLE_SLOT_FILTERS.map((slotValue) => (
							<TabsTrigger
								key={slotValue}
								value={slotValue}
								aria-label={
									slotValue === "all"
										? "All CCA slots"
										: `CCA ${slotValue}`
								}
								className="min-w-20 px-3"
							>
								{slotValue === "all"
									? "All slots"
									: `CCA ${slotValue}`}
							</TabsTrigger>
						))}
					</TabsList>
				</div>
			</div>

			{TIMETABLE_SLOT_FILTERS.map((slotValue) => (
				<TabsContent
					key={slotValue}
					value={slotValue}
					className="sr-only"
				>
					{dayLabel},{" "}
					{slotValue === "all" ? "all CCA slots" : `CCA ${slotValue}`}
				</TabsContent>
			))}
		</Tabs>
	)
}

function handleTimeSlotKeyDown(
	event: KeyboardEvent<HTMLTableCellElement>,
	onActivate: () => void,
): void {
	if (event.key !== "Enter" && event.key !== " ") return
	event.preventDefault()
	onActivate()
}

export function CourseTimetable({
	courses,
	selectedCourses,
	busyCourseID,
	onToggle,
	dayFilter,
	onDayFilterChange,
	slotFilter,
	onSlotFilterChange,
	categoryItems,
	categoryFilter,
	onCategoryFilterChange,
	availableOnly,
	onAvailableOnlyChange,
}: CourseTimetableProps): React.JSX.Element {
	const [previewedCourseID, setPreviewedCourseID] = useState<string | null>(
		null,
	)
	const [hoveredCourseID, setHoveredCourseID] = useState<string | null>(null)
	const [draggingCourseID, setDraggingCourseID] = useState<string | null>(
		null,
	)
	const [dragOverTimeSlot, setDragOverTimeSlot] = useState<string | null>(
		null,
	)
	const coursesByTimeSlot = useMemo(() => {
		const grouped = new Map(
			FIXED_CCA_TIME_SLOTS.map((timeSlot) => [timeSlot, [] as Course[]]),
		)
		for (const course of selectedCourses) {
			if (course.selected_period_id) {
				grouped.get(course.selected_period_id)?.push(course)
			}
		}
		return grouped
	}, [selectedCourses])

	const activeCourseID =
		draggingCourseID ?? hoveredCourseID ?? previewedCourseID ?? busyCourseID
	const activeCourse = courses.find((course) => course.id === activeCourseID)
	const activePeriodIDs = new Set(activeCourse?.available_period_ids ?? [])

	const clearDragState = (): void => {
		setDraggingCourseID(null)
		setDragOverTimeSlot(null)
	}

	const handleDragStart = (
		event: DragEvent<HTMLDivElement>,
		course: Course,
	): void => {
		event.dataTransfer.effectAllowed = "copy"
		event.dataTransfer.setData("text/plain", course.id)
		setDraggingCourseID(course.id)
		setPreviewedCourseID(course.id)
	}

	const handleDrop = (
		event: DragEvent<HTMLTableCellElement>,
		timeSlot: string,
	): void => {
		event.preventDefault()
		const courseID =
			draggingCourseID || event.dataTransfer.getData("text/plain")
		const course = courses.find((candidate) => candidate.id === courseID)
		clearDragState()

		if (
			course === undefined ||
			!course.available ||
			busyCourseID !== null ||
			!course.available_period_ids.includes(timeSlot)
		) {
			return
		}

		onToggle(course, timeSlot)
	}

	const selectActiveCourse = (timeSlot: string): void => {
		if (
			activeCourse === undefined ||
			!activeCourse.available ||
			busyCourseID !== null ||
			!activeCourse.available_period_ids.includes(timeSlot)
		) {
			return
		}
		onToggle(activeCourse, timeSlot)
	}

	return (
		<div className="flex flex-col gap-5">
			<div className="pb-3 md:sticky md:top-[calc(var(--student-page-header-height)+0.75rem)] md:z-20">
				<Card
					size="sm"
					className="shadow-sm md:shadow-md md:shadow-foreground/10"
				>
					<CardHeader>
						<CardTitle>Your timetable</CardTitle>
					</CardHeader>
					<CardContent className="px-0">
						<Table
							className="min-w-[44rem] table-fixed"
							containerLabel="Your weekly CCA timetable"
						>
							<TableCaption className="sr-only">
								Selected CCAs with weekdays as columns and CCA
								slots as rows. Drag an available CCA to one
								highlighted slot, or choose a card and then a
								highlighted slot.
							</TableCaption>
							<TableHeader>
								<TableRow className="hover:bg-transparent">
									<TableHead className="sticky left-0 z-10 w-16 bg-muted px-1 text-center">
										Slot
									</TableHead>
									{CCA_DAYS.map((day) => (
										<TableHead
											key={day}
											className="border-l bg-muted/50 text-center"
										>
											<span className="sm:hidden">
												{day.slice(0, 3)}
											</span>
											<span className="hidden sm:inline">
												{day}
											</span>
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
											className="sticky left-0 z-10 h-10 bg-muted px-1 text-center"
										>
											CCA {slot}
										</TableHead>
										{CCA_DAYS.map((day) => {
											const timeSlot = ccaTimeSlotID(
												day,
												slot,
											)
											const slotCourses =
												coursesByTimeSlot.get(
													timeSlot,
												) ?? []
											const isPreviewSlot =
												activePeriodIDs.has(timeSlot)
											const canSelect =
												activeCourse?.available ===
													true &&
												busyCourseID === null &&
												isPreviewSlot
											const canDrop =
												draggingCourseID !== null &&
												canSelect

											return (
												<TableCell
													key={timeSlot}
													className={cn(
														"h-10 border-l p-1 align-middle whitespace-normal transition-[background-color,box-shadow,opacity]",
														activeCourse !==
															undefined &&
															!isPreviewSlot &&
															"opacity-50",
														isPreviewSlot &&
															"bg-accent/70 ring-2 ring-inset ring-ring/60",
														canDrop &&
															dragOverTimeSlot ===
																timeSlot &&
															"bg-primary/10",
													)}
													role={
														canSelect
															? "button"
															: undefined
													}
													tabIndex={
														canSelect
															? 0
															: undefined
													}
													aria-label={
														canSelect &&
														activeCourse !==
															undefined
															? `${formatCCATimeSlotLabel(timeSlot)}, select ${activeCourse.name} in this slot`
															: undefined
													}
													onClick={() =>
														selectActiveCourse(
															timeSlot,
														)
													}
													onKeyDown={(event) => {
														if (!canSelect) return
														handleTimeSlotKeyDown(
															event,
															() =>
																selectActiveCourse(
																	timeSlot,
																),
														)
													}}
													onDragOver={(event) => {
														if (!canDrop) return
														event.preventDefault()
														event.dataTransfer.dropEffect =
															"copy"
														setDragOverTimeSlot(
															timeSlot,
														)
													}}
													onDragLeave={() =>
														setDragOverTimeSlot(
															null,
														)
													}
													onDrop={(event) =>
														handleDrop(
															event,
															timeSlot,
														)
													}
												>
													<div className="flex min-h-8 flex-col justify-center gap-1">
														{slotCourses.map(
															(course) => (
																<div
																	key={
																		course.id
																	}
																	onClick={(
																		event,
																	) =>
																		event.stopPropagation()
																	}
																>
																	<TimetableSelectedCourse
																		course={
																			course
																		}
																		busyCourseID={
																			busyCourseID
																		}
																		onToggle={
																			onToggle
																		}
																	/>
																</div>
															),
														)}
														{activeCourse !==
															undefined &&
														isPreviewSlot &&
														activeCourse.available &&
														!activeCourse.selected ? (
															<Badge
																variant="outline"
																className="h-auto justify-start rounded-md py-1"
															>
																{busyCourseID ===
																activeCourse.id ? (
																	<Spinner data-icon="inline-start" />
																) : null}
																{busyCourseID ===
																activeCourse.id
																	? "Selecting…"
																	: "Drop here"}
															</Badge>
														) : null}
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
			</div>

			<Card size="sm" className="overflow-hidden">
				<CardHeader className="sr-only">
					<CardTitle>CCA candidate filters</CardTitle>
				</CardHeader>
				<CardContent>
					<FieldGroup className="flex w-full flex-col items-start gap-4 lg:flex-row lg:flex-wrap lg:items-end">
						<Field className="min-w-0 lg:w-auto">
							<FieldLabel id="timetable-schedule-filter-label">
								Schedule
							</FieldLabel>
							<Tabs
								value={dayFilter}
								onValueChange={(value) =>
									onDayFilterChange(
										value as TimetableDayFilter,
									)
								}
								className="w-full min-w-0 gap-0 lg:w-auto"
							>
								<div className="no-scrollbar overflow-x-auto overflow-y-hidden">
									<TabsList
										activateOnFocus
										aria-labelledby="timetable-schedule-filter-label"
										className="h-10 min-w-max"
									>
										{TIMETABLE_DAY_FILTERS.map(
											(dayValue) => (
												<TabsTrigger
													key={dayValue}
													value={dayValue}
													aria-label={
														dayValue === "all"
															? "All days"
															: dayValue
													}
													className="min-w-20 px-3"
												>
													{dayValue === "all"
														? "All days"
														: dayValue}
												</TabsTrigger>
											),
										)}
									</TabsList>
								</div>

								{TIMETABLE_DAY_FILTERS.map((dayValue) => (
									<TabsContent
										key={dayValue}
										value={dayValue}
										className="sr-only"
									>
										{dayValue === "all"
											? "All days"
											: dayValue}
									</TabsContent>
								))}
							</Tabs>
						</Field>
						<Field className="lg:w-56">
							<FieldLabel htmlFor="timetable-category-filter">
								CCA category
							</FieldLabel>
							<Select
								items={categoryItems}
								value={categoryFilter}
								onValueChange={(value) =>
									onCategoryFilterChange(value ?? "all")
								}
							>
								<SelectTrigger
									id="timetable-category-filter"
									className="h-10 w-full"
								>
									<SelectValue />
								</SelectTrigger>
								<SelectContent alignItemWithTrigger={false}>
									<SelectGroup>
										{categoryItems.map((item) => (
											<SelectItem
												key={item.value}
												value={item.value}
											>
												{item.label}
											</SelectItem>
										))}
									</SelectGroup>
								</SelectContent>
							</Select>
						</Field>
						<Field
							orientation="horizontal"
							className="min-h-10 w-auto"
						>
							<FieldLabel
								htmlFor="timetable-available-filter"
								className="whitespace-nowrap"
							>
								Available only
							</FieldLabel>
							<Switch
								id="timetable-available-filter"
								checked={availableOnly}
								onCheckedChange={onAvailableOnlyChange}
							/>
						</Field>
					</FieldGroup>
				</CardContent>
			</Card>

			<TimetableSlotTabs
				day={dayFilter}
				slot={slotFilter}
				onSlotChange={onSlotFilterChange}
			/>

			{courses.length === 0 ? (
				<Empty>
					<EmptyHeader>
						<EmptyMedia variant="icon">
							<ClockAlertIcon />
						</EmptyMedia>
						<EmptyTitle>No matching CCA candidates</EmptyTitle>
						<EmptyDescription>
							Try another search, timetable slot, or filter.
						</EmptyDescription>
					</EmptyHeader>
				</Empty>
			) : (
				<div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
					{courses.map((course) => (
						<CourseCard
							key={course.id}
							course={course}
							busyCourseID={busyCourseID}
							onToggle={onToggle}
							mode="timetable"
							isPreviewed={activeCourseID === course.id}
							onPreview={setPreviewedCourseID}
							onHover={setHoveredCourseID}
							onDragStart={handleDragStart}
							onDragEnd={clearDragState}
						/>
					))}
				</div>
			)}
		</div>
	)
}
