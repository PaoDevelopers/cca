import {
	memo,
	type DragEvent,
	type KeyboardEvent,
	useCallback,
	useMemo,
	useState,
} from "react"
import {
	CheckIcon,
	ClockAlertIcon,
	MousePointerClickIcon,
	RotateCcwIcon,
	XIcon,
} from "lucide-react"

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
	EmptyContent,
	EmptyDescription,
	EmptyHeader,
	EmptyMedia,
	EmptyTitle,
} from "@/components/ui/empty"
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip"
import {
	CCA_DAYS,
	CCA_SLOTS_PER_DAY,
	ccaTimeSlotID,
	FIXED_CCA_TIME_SLOTS,
	formatCCATimeSlotLabel,
	type CCADay,
} from "@/lib/cca-schedule"
import { cn } from "@/lib/utils"
import type { Course } from "@/types"

export type CatalogLayout = "cards" | "list" | "timetable"

interface CourseViewProps {
	courses: readonly Course[]
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
		(reason) =>
			reason.code !== "course_full" &&
			reason.code !== "selections_closed",
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
	} else if (
		!isFull &&
		specificReasons.length === 0 &&
		!course.block_reasons.some(
			(reason) => reason.code === "selections_closed",
		)
	) {
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
		const badges = getSelectionContextBadges(course).filter(
			(badge) => badge.label !== "Locked",
		)
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

interface CourseCardProps {
	course: Course
	busyCourseID: string | null
	onToggle: (course: Course, periodID?: string) => void
	mode?: "catalog" | "timetable"
	isPreviewed?: boolean
	onPreview?: (courseID: string) => void
	onHover?: (courseID: string | null) => void
	onDragStart?: (event: DragEvent<HTMLDivElement>, course: Course) => void
	onDragEnd?: () => void
}

const CourseCard = memo(function CourseCard({
	course,
	busyCourseID,
	onToggle,
	mode = "catalog",
	isPreviewed = false,
	onPreview,
	onHover,
	onDragStart,
	onDragEnd,
}: CourseCardProps): React.JSX.Element {
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
			onMouseEnter={() => {
				if (canDrag) onHover?.(course.id)
			}}
			onMouseLeave={() => {
				if (canDrag) onHover?.(null)
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
					{course.category_id}
				</CardDescription>
				{isTimetableCandidate ? (
					course.available ? (
						<CardAction>
							<Button
								variant="outline"
								size="sm"
								disabled={busyCourseID !== null}
								onClick={previewCourse}
								onFocus={previewCourse}
								aria-pressed={isPreviewed}
							>
								Choose slots
							</Button>
						</CardAction>
					) : null
				) : (
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
})

interface TimetableSelectedCourseProps {
	course: Course
	busyCourseID: string | null
	onToggle: (course: Course, periodID?: string) => void
}

const TimetableSelectedCourse = memo(function TimetableSelectedCourse({
	course,
	busyCourseID,
	onToggle,
}: TimetableSelectedCourseProps): React.JSX.Element {
	const busy = busyCourseID === course.id
	const removable = course.removable && course.selection_type !== "force"

	return (
		<Card size="sm" className="gap-0 py-0">
			<CardHeader className="flex min-h-11 items-center gap-1 px-2 md:min-h-8">
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
										size="icon-sm"
										className="size-11 md:size-7"
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
})

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
		<>
			<div
				className="flex flex-col gap-3 md:hidden"
				aria-label="CCA catalogue list"
			>
				{courses.map((course) => (
					<Card key={course.id} size="sm">
						<CardHeader>
							<CardTitle className="break-words">
								{course.name}
							</CardTitle>
							<CardDescription className="break-all">
								{course.category_id}
							</CardDescription>
							<CardAction>
								<CourseAction
									course={course}
									busyCourseID={busyCourseID}
									onToggle={onToggle}
									size="sm"
								/>
							</CardAction>
						</CardHeader>
						<CardContent className="flex flex-col gap-3">
							<PeriodBadges
								periodIDs={
									course.selected && course.selected_period_id
										? [course.selected_period_id]
										: course.period_ids
								}
							/>
							<dl className="grid grid-cols-2 gap-3 text-sm">
								<div className="flex min-w-0 flex-col gap-0.5">
									<dt className="text-xs text-muted-foreground">
										Teacher
									</dt>
									<dd className="break-words">
										{course.teacher}
									</dd>
								</div>
								<div className="flex min-w-0 flex-col gap-0.5">
									<dt className="text-xs text-muted-foreground">
										Location
									</dt>
									<dd className="break-words">
										{course.location}
									</dd>
								</div>
							</dl>
							<CourseListStatus course={course} />
						</CardContent>
						<CardFooter className="justify-between gap-3">
							<span className="text-muted-foreground">
								Enrollment
							</span>
							<EnrollmentBadge course={course} />
						</CardFooter>
					</Card>
				))}
			</div>

			<Card className="hidden md:flex">
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
								<TableHead className="text-right">
									Action
								</TableHead>
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
												{course.category_id}
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
													? [
															course.selected_period_id,
														]
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
		</>
	)
}

interface CourseTimetableProps extends CourseViewProps {
	selectedCourses: readonly Course[]
	onResetFilters: () => void
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
	onResetFilters,
}: CourseTimetableProps): React.JSX.Element {
	const [mobileDay, setMobileDay] = useState<CCADay>("Monday")
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
	const coursesByID = useMemo(() => {
		const indexedCourses = new Map<string, Course>()
		for (const course of courses) indexedCourses.set(course.id, course)
		return indexedCourses
	}, [courses])

	const activeCourseID =
		draggingCourseID ?? hoveredCourseID ?? previewedCourseID ?? busyCourseID
	const activeCourse =
		activeCourseID === null ? undefined : coursesByID.get(activeCourseID)
	const activePeriodIDs = useMemo(
		() => new Set(activeCourse?.available_period_ids ?? []),
		[activeCourse],
	)

	const clearDragState = useCallback((): void => {
		setDraggingCourseID(null)
		setDragOverTimeSlot(null)
	}, [])

	const handleDragStart = useCallback(
		(event: DragEvent<HTMLDivElement>, course: Course): void => {
			event.dataTransfer.effectAllowed = "copy"
			event.dataTransfer.setData("text/plain", course.id)
			setDraggingCourseID(course.id)
			setPreviewedCourseID(course.id)
		},
		[],
	)

	const handleDrop = (
		event: DragEvent<HTMLTableCellElement>,
		timeSlot: string,
	): void => {
		event.preventDefault()
		const courseID =
			draggingCourseID || event.dataTransfer.getData("text/plain")
		const course = coursesByID.get(courseID)
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
			<div className="sticky top-[calc(var(--student-page-header-height)+0.5rem)] z-20 pb-3">
				<Card size="sm" className="shadow-md shadow-foreground/10">
					<CardHeader>
						<CardTitle>Your timetable</CardTitle>
						<CardDescription>
							Choose a CCA below, then choose one highlighted
							slot. On desktop, you can also drag a CCA into the
							timetable.
						</CardDescription>
					</CardHeader>
					<CardContent className="px-0">
						<Tabs
							value={mobileDay}
							onValueChange={(value) =>
								setMobileDay(value as CCADay)
							}
							className="gap-3 px-3 md:hidden"
						>
							<TabsList
								variant="line"
								activateOnFocus
								aria-label="Timetable day"
								className="h-auto w-full"
							>
								{CCA_DAYS.map((day) => (
									<TabsTrigger
										key={day}
										value={day}
										aria-label={day}
										className="min-h-11"
									>
										{day.slice(0, 3)}
									</TabsTrigger>
								))}
							</TabsList>
							{CCA_DAYS.map((day) => (
								<TabsContent key={day} value={day}>
									<Table
										className="table-fixed"
										containerLabel={`${day} CCA timetable`}
									>
										<TableCaption className="sr-only">
											Selected CCAs and available slots
											for {day}.
										</TableCaption>
										<TableBody>
											{CCA_SLOTS_PER_DAY.map((slot) => {
												const timeSlot = ccaTimeSlotID(
													day,
													slot,
												)
												const slotCourses =
													coursesByTimeSlot.get(
														timeSlot,
													) ?? []
												const isPreviewSlot =
													activePeriodIDs.has(
														timeSlot,
													)

												return (
													<TableRow
														key={timeSlot}
														className={cn(
															isPreviewSlot &&
																"bg-accent/60",
														)}
													>
														<TableHead
															scope="row"
															className="w-20 px-2 text-center"
														>
															CCA {slot}
														</TableHead>
														<TableCell className="h-12 p-1 whitespace-normal">
															<div className="flex min-h-11 flex-col justify-center gap-1">
																{slotCourses.length ===
																0 ? (
																	<span className="px-2 text-xs text-muted-foreground">
																		No CCA
																	</span>
																) : (
																	slotCourses.map(
																		(
																			course,
																		) => (
																			<TimetableSelectedCourse
																				key={
																					course.id
																				}
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
																		),
																	)
																)}
																{activeCourse !==
																	undefined &&
																isPreviewSlot &&
																!activeCourse.selected ? (
																	<Badge variant="outline">
																		Available
																	</Badge>
																) : null}
															</div>
														</TableCell>
													</TableRow>
												)
											})}
										</TableBody>
									</Table>
								</TabsContent>
							))}
						</Tabs>

						<div className="hidden md:block">
							<Table
								className="min-w-[44rem] table-fixed"
								containerLabel="Your weekly CCA timetable"
							>
								<TableCaption className="sr-only">
									Selected CCAs with weekdays as columns and
									CCA slots as rows. Drag an available CCA to
									one highlighted slot, or choose a card and
									then a highlighted slot.
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
													activePeriodIDs.has(
														timeSlot,
													)
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
															if (!canSelect)
																return
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
																		: draggingCourseID !==
																			  null
																			? "Drop here"
																			: `Choose ${formatCCATimeSlotLabel(timeSlot)}`}
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
						</div>
					</CardContent>
				</Card>
			</div>

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
					<EmptyContent>
						<Button variant="outline" onClick={onResetFilters}>
							<RotateCcwIcon data-icon="inline-start" />
							Reset filters
						</Button>
					</EmptyContent>
				</Empty>
			) : (
				<div
					className={cn(
						"grid gap-4 md:grid-cols-2 xl:grid-cols-3",
						activeCourse !== undefined && "pb-48 md:pb-0",
					)}
				>
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

			{activeCourse !== undefined &&
			activeCourse.available &&
			!activeCourse.selected ? (
				<Card
					size="sm"
					className="fixed inset-x-4 bottom-4 z-40 shadow-lg md:hidden"
					aria-live="polite"
				>
					<CardHeader>
						<CardTitle>{activeCourse.name}</CardTitle>
						<CardDescription>
							Choose one available CCA slot.
						</CardDescription>
						<CardAction>
							<Button
								variant="ghost"
								size="icon"
								className="size-11"
								onClick={() => setPreviewedCourseID(null)}
								aria-label={`Close slot choices for ${activeCourse.name}`}
							>
								<XIcon />
							</Button>
						</CardAction>
					</CardHeader>
					<CardContent className="grid grid-cols-2 gap-2">
						{activeCourse.available_period_ids.map((periodID) => (
							<Button
								key={periodID}
								variant="outline"
								className="min-h-11 justify-start"
								disabled={busyCourseID !== null}
								onClick={() => onToggle(activeCourse, periodID)}
							>
								<MousePointerClickIcon data-icon="inline-start" />
								{formatCCATimeSlotLabel(periodID)}
							</Button>
						))}
					</CardContent>
				</Card>
			) : null}
		</div>
	)
}
