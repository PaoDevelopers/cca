import { useCallback, useEffect, useMemo, useState } from "react"
import {
	BookOpenCheckIcon,
	CalendarDaysIcon,
	LayoutGridIcon,
	ListIcon,
	SearchIcon,
	Trash2Icon,
} from "lucide-react"
import { toast } from "sonner"

import { apiRequest, jsonBody } from "@/api"
import { ErrorAlert, PageSkeleton, PeriodBadges } from "@/components/common"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog"
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
	Empty,
	EmptyDescription,
	EmptyHeader,
	EmptyMedia,
	EmptyTitle,
} from "@/components/ui/empty"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "@/components/ui/input-group"
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import {
	CourseCardGrid,
	CourseList,
	CourseTimetable,
	type CatalogLayout,
} from "@/features/student/CourseCatalogViews"
import { useSearchFilter } from "@/hooks/use-search-filter"
import {
	CCA_DAYS,
	CCA_SLOTS_PER_DAY,
	ccaTimeSlotID,
	formatCCATimeSlotLabel,
	type CCADay,
	type CCASlotNumber,
} from "@/lib/cca-schedule"
import type { Course, StudentBootstrap } from "@/types"

type CCADayFilter = "all" | CCADay
type CCASlotFilter = "all" | `${CCASlotNumber}`

const CCA_DAY_FILTERS: readonly CCADayFilter[] = ["all", ...CCA_DAYS]
const CCA_SLOT_FILTERS: readonly CCASlotFilter[] = [
	"all",
	...CCA_SLOTS_PER_DAY.map((slot) => String(slot) as CCASlotFilter),
]
const EMPTY_COURSES: readonly Course[] = []

function getCourseSearchText(course: Course): string {
	return [
		course.name,
		course.id,
		course.description,
		course.teacher,
		course.location,
		course.category_id,
	].join(" ")
}

function matchesPeriodFilter(
	periodIDs: readonly string[],
	day: CCADayFilter,
	slot: CCASlotFilter,
): boolean {
	if (day === "all" && slot === "all") return periodIDs.length > 0

	if (day !== "all" && slot !== "all") {
		return periodIDs.includes(
			ccaTimeSlotID(day, Number(slot) as CCASlotNumber),
		)
	}

	if (day !== "all") {
		return CCA_SLOTS_PER_DAY.some((slotNumber) =>
			periodIDs.includes(ccaTimeSlotID(day, slotNumber)),
		)
	}

	const slotNumber = Number(slot) as CCASlotNumber
	return CCA_DAYS.some((slotDay) =>
		periodIDs.includes(ccaTimeSlotID(slotDay, slotNumber)),
	)
}

function matchesScheduleFilter(
	course: Course,
	day: CCADayFilter,
	slot: CCASlotFilter,
): boolean {
	return matchesPeriodFilter(course.period_ids, day, slot)
}

interface ScheduleSlotTabsProps {
	day: CCADayFilter
	slot: CCASlotFilter
	onSlotChange: (slot: CCASlotFilter) => void
}

function ScheduleSlotTabs({
	day,
	slot,
	onSlotChange,
}: ScheduleSlotTabsProps): React.JSX.Element {
	const dayLabel = day === "all" ? "All days" : day

	return (
		<Tabs
			value={slot}
			onValueChange={(value) => onSlotChange(value as CCASlotFilter)}
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
						{CCA_SLOT_FILTERS.map((slotValue) => (
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

			{CCA_SLOT_FILTERS.map((slotValue) => (
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

function parseCourseCountUpdate(
	message: string,
): { courseID: string; currentStudents: number } | null {
	const parts = message.split(",")
	const [messageType, courseID, countText] = parts
	if (
		parts.length !== 3 ||
		messageType !== "course_count_update" ||
		courseID === undefined ||
		courseID === "" ||
		countText === undefined
	) {
		return null
	}

	const currentStudents = Number(countText)
	if (!Number.isSafeInteger(currentStudents) || currentStudents < 0) {
		return null
	}

	return { courseID, currentStudents }
}

export default function StudentApp(): React.JSX.Element {
	const [data, setData] = useState<StudentBootstrap | null>(null)
	const [error, setError] = useState<string | null>(null)
	const [query, setQuery] = useState("")
	const [layout, setLayout] = useState<CatalogLayout>("cards")
	const [dayFilter, setDayFilter] = useState<CCADayFilter>("all")
	const [slotFilter, setSlotFilter] = useState<CCASlotFilter>("all")
	const [categoryFilter, setCategoryFilter] = useState("all")
	const [availableOnly, setAvailableOnly] = useState(false)
	const [timetableAvailableOnly, setTimetableAvailableOnly] = useState(true)
	const [busyCourseID, setBusyCourseID] = useState<string | null>(null)
	const [removeCourse, setRemoveCourse] = useState<Course | null>(null)
	const [slotCourse, setSlotCourse] = useState<Course | null>(null)
	const courses = data?.courses ?? EMPTY_COURSES

	const load = useCallback(async (): Promise<void> => {
		try {
			const bootstrap = await apiRequest<StudentBootstrap>(
				"/api/v1/student/bootstrap",
			)
			setData(bootstrap)
			setError(null)
		} catch (caught) {
			setError(
				caught instanceof Error
					? caught.message
					: "Unable to load the CCA catalogue.",
			)
		}
	}, [])

	useEffect(() => {
		// eslint-disable-next-line react-hooks/set-state-in-effect -- Catalogue state is set after the requests resolve.
		void load()
	}, [load])

	useEffect(() => {
		const protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
		const socket = new WebSocket(
			`${protocol}//${window.location.host}/api/v1/student/events`,
		)
		socket.addEventListener("message", (event) => {
			if (typeof event.data !== "string") return
			if (event.data.startsWith("notify,")) {
				toast.info(event.data.slice("notify,".length))
				return
			}

			const countUpdate = parseCourseCountUpdate(event.data)
			if (countUpdate !== null) {
				setData((current) => {
					if (current === null) return current

					const courseIndex = current.courses.findIndex(
						(course) => course.id === countUpdate.courseID,
					)
					const existingCourse = current.courses[courseIndex]
					if (
						existingCourse === undefined ||
						existingCourse.current_students ===
							countUpdate.currentStudents
					) {
						return current
					}

					const courses = [...current.courses]
					courses[courseIndex] = {
						...existingCourse,
						current_students: countUpdate.currentStudents,
					}
					return { ...current, courses }
				})
				return
			}

			if (event.data.startsWith("invalidate_")) {
				void load()
			}
		})
		return () => socket.close()
	}, [load])

	const selectedCourses = useMemo(
		() => courses.filter((course) => course.selected),
		[courses],
	)
	const categories = useMemo(
		() =>
			[...new Set(courses.map((course) => course.category_id))]
				.filter((category) => category !== "")
				.sort((left, right) => left.localeCompare(right)),
		[courses],
	)
	const categoryItems = useMemo(
		() => [
			{ label: "All categories", value: "all" },
			...categories.map((category) => ({
				label: category,
				value: category,
			})),
		],
		[categories],
	)
	const searchResults = useSearchFilter(courses, query, getCourseSearchText)
	const visibleCourses = useMemo(() => {
		return searchResults.filter(
			(course) =>
				(categoryFilter === "all" ||
					course.category_id === categoryFilter) &&
				(!availableOnly ||
					matchesPeriodFilter(
						course.available_period_ids,
						dayFilter,
						slotFilter,
					)) &&
				matchesScheduleFilter(course, dayFilter, slotFilter),
		)
	}, [availableOnly, categoryFilter, dayFilter, searchResults, slotFilter])
	const timetableCandidates = useMemo(
		() =>
			searchResults
				.filter(
					(course) =>
						!course.selected &&
						(categoryFilter === "all" ||
							course.category_id === categoryFilter) &&
						(!timetableAvailableOnly ||
							matchesPeriodFilter(
								course.available_period_ids,
								dayFilter,
								slotFilter,
							)) &&
						matchesScheduleFilter(course, dayFilter, slotFilter),
				)
				.sort(
					(left, right) =>
						Number(right.available) - Number(left.available) ||
						left.name.localeCompare(right.name),
				),
		[
			categoryFilter,
			dayFilter,
			searchResults,
			slotFilter,
			timetableAvailableOnly,
		],
	)

	const mutateSelection = useCallback(
		async (
			course: Course,
			method: "POST" | "DELETE",
			periodID?: string,
		): Promise<void> => {
			setBusyCourseID(course.id)
			try {
				const bootstrap = await apiRequest<StudentBootstrap>(
					"/api/v1/student/selections",
					{
						method,
						body: jsonBody({
							course_id: course.id,
							period_id:
								periodID ?? course.selected_period_id ?? "",
						}),
					},
				)
				setData(bootstrap)
				setError(null)
				toast.success(
					method === "POST"
						? `${course.name} selected for ${formatCCATimeSlotLabel(periodID ?? "")}.`
						: `${course.name} removed.`,
				)
			} catch (caught) {
				toast.error(
					caught instanceof Error
						? caught.message
						: "Unable to update the selection.",
				)
				await load()
			} finally {
				setBusyCourseID(null)
				setRemoveCourse(null)
				setSlotCourse(null)
			}
		},
		[load],
	)
	const handleCourseToggle = useCallback(
		(course: Course, periodID?: string): void => {
			if (course.selected) {
				setRemoveCourse(course)
				return
			}
			if (periodID !== undefined) {
				void mutateSelection(course, "POST", periodID)
				return
			}
			if (course.available_period_ids.length === 1) {
				void mutateSelection(
					course,
					"POST",
					course.available_period_ids[0],
				)
				return
			}
			setSlotCourse(course)
		},
		[mutateSelection],
	)

	if (data === null && error === null) return <PageSkeleton />
	if (data === null)
		return (
			<main id="main-content" className="mx-auto max-w-3xl p-6">
				<div className="flex flex-col gap-4">
					<ErrorAlert message={error ?? "Unable to load."} />
					<Button className="self-start" onClick={() => void load()}>
						Try again
					</Button>
				</div>
			</main>
		)

	const student = data.session.student
	return (
		<div className="min-h-svh bg-muted/30 [--student-page-header-height:3rem] sm:[--student-page-header-height:4.25rem]">
			<header className="sticky top-0 z-30 isolate h-(--student-page-header-height) border-b bg-background shadow-sm">
				<div className="mx-auto flex h-full max-w-7xl items-center justify-between gap-4 px-4 py-3 sm:px-6">
					<div className="min-w-0">
						<h1 className="truncate font-heading text-lg font-semibold">
							YKPaoSchool CCA Sign-Up
						</h1>
					</div>
					<div className="flex min-w-0 items-center gap-2 text-sm">
						<span className="max-w-28 truncate font-medium sm:max-w-64">
							{student.name}
						</span>
						<Badge className="shrink-0" variant="secondary">
							G{student.grade}
						</Badge>
					</div>
				</div>
			</header>

			<main
				id="main-content"
				className="relative z-0 mx-auto flex max-w-7xl flex-col gap-4 p-4 sm:p-6"
			>
				{error !== null ? <ErrorAlert message={error} /> : null}
				<Tabs defaultValue="catalog">
					<TabsList
						variant="line"
						activateOnFocus
						aria-label="Student sections"
					>
						<TabsTrigger value="catalog">
							<LayoutGridIcon data-icon="inline-start" />
							Catalogue
						</TabsTrigger>
						<TabsTrigger value="review">
							<BookOpenCheckIcon data-icon="inline-start" />
							My selections
						</TabsTrigger>
					</TabsList>

					<TabsContent
						value="catalog"
						className="flex flex-col gap-5 pt-5"
					>
						<div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
							<InputGroup className="h-11 max-w-xl">
								<InputGroupAddon>
									<SearchIcon aria-hidden="true" />
								</InputGroupAddon>
								<InputGroupInput
									type="search"
									name="catalogue_search"
									value={query}
									onChange={(event) =>
										setQuery(event.target.value)
									}
									placeholder="Search activities, teachers, or locations…"
									autoComplete="off"
									spellCheck={false}
									aria-label="Search CCA catalogue"
								/>
							</InputGroup>
							<ToggleGroup
								value={[layout]}
								onValueChange={(values) => {
									const next = values[0] as
										| CatalogLayout
										| undefined
									if (next !== undefined) setLayout(next)
								}}
								variant="outline"
								aria-label="Catalogue layout"
							>
								<ToggleGroupItem
									value="cards"
									aria-label="Card layout"
									className="min-h-11"
								>
									<LayoutGridIcon data-icon="inline-start" />
									Cards
								</ToggleGroupItem>
								<ToggleGroupItem
									value="list"
									aria-label="List layout"
									className="min-h-11"
								>
									<ListIcon data-icon="inline-start" />
									List
								</ToggleGroupItem>
								<ToggleGroupItem
									value="timetable"
									aria-label="Timetable layout"
									className="min-h-11"
								>
									<CalendarDaysIcon data-icon="inline-start" />
									Timetable
								</ToggleGroupItem>
							</ToggleGroup>
						</div>

						{layout === "timetable" ? null : (
							<Card size="sm" className="overflow-hidden">
								<CardHeader className="sr-only">
									<CardTitle>Catalogue filters</CardTitle>
								</CardHeader>
								<CardContent>
									<FieldGroup className="flex w-full flex-col items-start gap-4 lg:flex-row lg:flex-wrap lg:items-end">
										<Field className="min-w-0 lg:w-auto">
											<FieldLabel id="schedule-filter-label">
												Schedule
											</FieldLabel>
											<Tabs
												value={dayFilter}
												onValueChange={(value) =>
													setDayFilter(
														value as CCADayFilter,
													)
												}
												className="w-full min-w-0 gap-0 lg:w-auto"
											>
												<div className="no-scrollbar overflow-x-auto overflow-y-hidden">
													<TabsList
														activateOnFocus
														aria-labelledby="schedule-filter-label"
														className="h-10 min-w-max"
													>
														{CCA_DAY_FILTERS.map(
															(dayValue) => (
																<TabsTrigger
																	key={
																		dayValue
																	}
																	value={
																		dayValue
																	}
																	aria-label={
																		dayValue ===
																		"all"
																			? "All days"
																			: dayValue
																	}
																	className="min-w-20 px-3"
																>
																	{dayValue ===
																	"all"
																		? "All days"
																		: dayValue}
																</TabsTrigger>
															),
														)}
													</TabsList>
												</div>

												{CCA_DAY_FILTERS.map(
													(dayValue) => (
														<TabsContent
															key={dayValue}
															value={dayValue}
															className="sr-only"
														>
															{dayValue === "all"
																? "All days"
																: dayValue}
														</TabsContent>
													),
												)}
											</Tabs>
										</Field>
										<Field className="lg:w-56">
											<FieldLabel htmlFor="category-filter">
												CCA category
											</FieldLabel>
											<Select
												items={categoryItems}
												value={categoryFilter}
												onValueChange={(value) =>
													setCategoryFilter(
														value ?? "all",
													)
												}
											>
												<SelectTrigger
													id="category-filter"
													className="h-10 w-full"
												>
													<SelectValue />
												</SelectTrigger>
												<SelectContent
													alignItemWithTrigger={false}
												>
													<SelectGroup>
														{categoryItems.map(
															(item) => (
																<SelectItem
																	key={
																		item.value
																	}
																	value={
																		item.value
																	}
																>
																	{item.label}
																</SelectItem>
															),
														)}
													</SelectGroup>
												</SelectContent>
											</Select>
										</Field>
										<Field
											orientation="horizontal"
											className="min-h-10 w-auto"
										>
											<FieldLabel
												htmlFor="available-filter"
												className="whitespace-nowrap"
											>
												Available only
											</FieldLabel>
											<Switch
												id="available-filter"
												checked={availableOnly}
												onCheckedChange={
													setAvailableOnly
												}
											/>
										</Field>
									</FieldGroup>
								</CardContent>
							</Card>
						)}

						{layout === "timetable" ? null : (
							<ScheduleSlotTabs
								day={dayFilter}
								slot={slotFilter}
								onSlotChange={setSlotFilter}
							/>
						)}

						{layout === "timetable" ? (
							<CourseTimetable
								courses={timetableCandidates}
								selectedCourses={selectedCourses}
								busyCourseID={busyCourseID}
								onToggle={handleCourseToggle}
								dayFilter={dayFilter}
								onDayFilterChange={setDayFilter}
								slotFilter={slotFilter}
								onSlotFilterChange={setSlotFilter}
								categoryItems={categoryItems}
								categoryFilter={categoryFilter}
								onCategoryFilterChange={setCategoryFilter}
								availableOnly={timetableAvailableOnly}
								onAvailableOnlyChange={
									setTimetableAvailableOnly
								}
							/>
						) : visibleCourses.length === 0 ? (
							<Empty>
								<EmptyHeader>
									<EmptyMedia variant="icon">
										<SearchIcon />
									</EmptyMedia>
									<EmptyTitle>No matching CCAs</EmptyTitle>
									<EmptyDescription>
										Try another search or filter.
									</EmptyDescription>
								</EmptyHeader>
							</Empty>
						) : layout === "cards" ? (
							<CourseCardGrid
								courses={visibleCourses}
								busyCourseID={busyCourseID}
								onToggle={handleCourseToggle}
							/>
						) : (
							<CourseList
								courses={visibleCourses}
								busyCourseID={busyCourseID}
								onToggle={handleCourseToggle}
							/>
						)}
					</TabsContent>

					<TabsContent
						value="review"
						className="grid gap-5 pt-5 lg:grid-cols-[minmax(0,2fr)_minmax(16rem,1fr)]"
					>
						<Card>
							<CardHeader>
								<CardTitle>Your selected CCAs</CardTitle>
								<CardDescription>
									Each activity reserves the one slot you
									chose.
								</CardDescription>
							</CardHeader>
							<CardContent className="flex flex-col gap-4">
								{selectedCourses.length === 0 ? (
									<Empty>
										<EmptyHeader>
											<EmptyMedia variant="icon">
												<BookOpenCheckIcon />
											</EmptyMedia>
											<EmptyTitle>
												No selections yet
											</EmptyTitle>
											<EmptyDescription>
												Select an available activity
												from the catalogue.
											</EmptyDescription>
										</EmptyHeader>
									</Empty>
								) : (
									selectedCourses.map((course, index) => (
										<div
											key={course.id}
											className="flex flex-col gap-3"
										>
											{index > 0 ? <Separator /> : null}
											<div className="flex min-w-0 items-start justify-between gap-4">
												<div className="flex min-w-0 flex-1 flex-col gap-2">
													<p className="break-words font-medium">
														{course.name}
													</p>
													<PeriodBadges
														periodIDs={
															course.selected_period_id
																? [
																		course.selected_period_id,
																	]
																: []
														}
													/>
												</div>
												<Button
													variant="ghost"
													size="icon"
													className="size-11"
													disabled={
														!course.removable ||
														busyCourseID !== null
													}
													onClick={() =>
														setRemoveCourse(course)
													}
													aria-label={`Remove ${course.name}`}
												>
													<Trash2Icon />
												</Button>
											</div>
										</div>
									))
								)}
							</CardContent>
						</Card>

						<Card>
							<CardHeader>
								<CardTitle>Requirements</CardTitle>
								<CardDescription>
									Progress for {student.grade}.
								</CardDescription>
							</CardHeader>
							<CardContent className="flex flex-col gap-3">
								{data.requirements.length === 0 ? (
									<p className="text-sm text-muted-foreground">
										No requirements configured.
									</p>
								) : (
									data.requirements.map((row) => (
										<div
											key={row.id}
											className="flex min-w-0 items-center justify-between gap-3"
										>
											<span className="min-w-0 break-words">
												{row.category_ids.join(" / ")}
											</span>
											<Badge
												className="shrink-0 tabular-nums"
												variant={
													row.current_count >=
													row.min_count
														? "secondary"
														: "outline"
												}
											>
												{row.current_count}/
												{row.min_count}
											</Badge>
										</div>
									))
								)}
							</CardContent>
						</Card>
					</TabsContent>
				</Tabs>
			</main>

			<Dialog
				open={slotCourse !== null}
				onOpenChange={(open) => {
					if (!open) setSlotCourse(null)
				}}
			>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>Choose a CCA slot</DialogTitle>
						<DialogDescription>
							{slotCourse === null
								? "Choose one available timetable slot."
								: `Choose when you want to attend ${slotCourse.name}. Only that slot will be reserved.`}
						</DialogDescription>
					</DialogHeader>
					<div className="flex flex-col gap-2">
						{slotCourse?.available_period_ids.map((periodID) => (
							<Button
								key={periodID}
								variant="outline"
								className="justify-start"
								disabled={busyCourseID !== null}
								onClick={() =>
									void mutateSelection(
										slotCourse,
										"POST",
										periodID,
									)
								}
							>
								<CalendarDaysIcon data-icon="inline-start" />
								{formatCCATimeSlotLabel(periodID)}
							</Button>
						))}
					</div>
				</DialogContent>
			</Dialog>

			<AlertDialog
				open={removeCourse !== null}
				onOpenChange={(open) => {
					if (!open) setRemoveCourse(null)
				}}
			>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							Remove this selection?
						</AlertDialogTitle>
						<AlertDialogDescription>
							{removeCourse === null
								? ""
								: `${removeCourse.name} will become available to select again.`}
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>Cancel</AlertDialogCancel>
						<AlertDialogAction
							variant="destructive"
							disabled={
								removeCourse === null || busyCourseID !== null
							}
							onClick={() => {
								if (removeCourse !== null)
									void mutateSelection(removeCourse, "DELETE")
							}}
						>
							<Trash2Icon data-icon="inline-start" />
							Remove
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	)
}
