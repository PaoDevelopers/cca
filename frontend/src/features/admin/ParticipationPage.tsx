import { useEffect, useMemo, useState } from "react"
import {
	BookOpenIcon,
	FilterIcon,
	MoreHorizontalIcon,
	PencilIcon,
	PlusIcon,
	SearchIcon,
	Trash2Icon,
	UserRoundIcon,
	XIcon,
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
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
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
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuGroup,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
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
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "@/components/ui/input-group"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import {
	Pagination,
	PaginationContent,
	PaginationEllipsis,
	PaginationItem,
	PaginationLink,
	PaginationNext,
	PaginationPrevious,
} from "@/components/ui/pagination"
import {
	Popover,
	PopoverContent,
	PopoverDescription,
	PopoverHeader,
	PopoverTitle,
	PopoverTrigger,
} from "@/components/ui/popover"
import {
	ResizableHandle,
	ResizablePanel,
	ResizablePanelGroup,
} from "@/components/ui/resizable"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import {
	Sheet,
	SheetContent,
	SheetDescription,
	SheetFooter,
	SheetHeader,
	SheetTitle,
} from "@/components/ui/sheet"
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
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import type { AdminPageProps } from "@/features/admin/AdminApp"
import { StudentDialog } from "@/features/admin/AdminPages"
import { useMediaQuery } from "@/hooks/use-media-query"
import { CCA_DAYS, CCA_SLOTS_PER_DAY, ccaTimeSlotID } from "@/lib/cca-schedule"
import { cn } from "@/lib/utils"
import type {
	AdminBootstrap,
	Course,
	Selection,
	SelectionType,
	Student,
} from "@/types"

type ParticipationTab = "students" | "courses" | "assignments"
type CompletionStatus = "all" | "unassigned" | "incomplete" | "complete"

interface ParticipationFilters {
	query: string
	grade: string
	period: string
	type: "all" | SelectionType
	status: CompletionStatus
	course: string
}

type DetailTarget =
	| { kind: "student"; id: number }
	| { kind: "course"; id: string }
	| null

const PAGE_SIZE = 50
const ASSIGNMENT_STUDENT_WINDOW_SIZE = 50
const PARTICIPATION_MAIN_PANEL_ID = "participation-main"
const PARTICIPATION_DETAILS_PANEL_ID = "participation-details"
const PARTICIPATION_LAYOUT_STORAGE_KEY = "cca-admin-participation-layout"
const DEFAULT_PARTICIPATION_LAYOUT: Record<string, number> = {
	[PARTICIPATION_MAIN_PANEL_ID]: 70,
	[PARTICIPATION_DETAILS_PANEL_ID]: 30,
}

const EMPTY_FILTERS: ParticipationFilters = {
	query: "",
	grade: "",
	period: "",
	type: "all",
	status: "all",
	course: "",
}

function loadParticipationLayout(): Record<string, number> {
	try {
		const raw = window.localStorage.getItem(
			PARTICIPATION_LAYOUT_STORAGE_KEY,
		)
		if (raw === null) return DEFAULT_PARTICIPATION_LAYOUT
		const parsed: unknown = JSON.parse(raw)
		if (typeof parsed !== "object" || parsed === null)
			return DEFAULT_PARTICIPATION_LAYOUT
		const layout = parsed as Record<string, unknown>
		const main = layout[PARTICIPATION_MAIN_PANEL_ID]
		const details = layout[PARTICIPATION_DETAILS_PANEL_ID]
		if (
			typeof main !== "number" ||
			typeof details !== "number" ||
			!Number.isFinite(main) ||
			!Number.isFinite(details) ||
			main < 55 ||
			main > 100 ||
			details < 0 ||
			details > 45 ||
			Math.abs(main + details - 100) > 0.5
		) {
			return DEFAULT_PARTICIPATION_LAYOUT
		}
		return {
			[PARTICIPATION_MAIN_PANEL_ID]: main,
			[PARTICIPATION_DETAILS_PANEL_ID]: details,
		}
	} catch {
		return DEFAULT_PARTICIPATION_LAYOUT
	}
}

function saveParticipationLayout(layout: Record<string, number>): void {
	try {
		window.localStorage.setItem(
			PARTICIPATION_LAYOUT_STORAGE_KEY,
			JSON.stringify(layout),
		)
	} catch {
		// The layout still works when browser storage is unavailable.
	}
}

function textMatches(value: string, query: string): boolean {
	return value.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())
}

function selectionsForStudent(
	selections: readonly Selection[],
	studentID: number,
): Selection[] {
	return selections.filter((selection) => selection.student_id === studentID)
}

function selectionsForCourse(
	selections: readonly Selection[],
	courseID: string,
): Selection[] {
	return selections.filter((selection) => selection.course_id === courseID)
}

function studentCompletionStatus(
	student: Student,
	selections: readonly Selection[],
	data: AdminBootstrap,
): Exclude<CompletionStatus, "all"> {
	if (selections.length === 0) return "unassigned"

	const grade = data.grades.find((item) => item.grade === student.grade)
	if (grade === undefined) return "incomplete"

	const courseByID = new Map(
		data.courses.map((course) => [course.id, course]),
	)
	const meetsSelectionTarget = selections.length >= grade.max_own_choices
	const meetsRequirements = grade.req_groups.every((group) => {
		const count = selections.filter((selection) => {
			const course = courseByID.get(selection.course_id)
			return (
				course !== undefined &&
				group.category_ids.includes(course.category_id)
			)
		}).length
		return count >= group.min_count
	})

	return meetsSelectionTarget && meetsRequirements ? "complete" : "incomplete"
}

function CompletionBadge({
	status,
}: {
	status: Exclude<CompletionStatus, "all">
}): React.JSX.Element {
	return (
		<Badge
			variant={
				status === "complete"
					? "secondary"
					: status === "unassigned"
						? "destructive"
						: "outline"
			}
		>
			{status === "complete"
				? "Complete"
				: status === "unassigned"
					? "Unassigned"
					: "Incomplete"}
		</Badge>
	)
}

function SelectionTypeBadge({
	type,
}: {
	type: SelectionType
}): React.JSX.Element {
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
			{type === "normal"
				? "Student choice"
				: type === "invite"
					? "Invitation"
					: "Forced"}
		</Badge>
	)
}

function SearchBox({
	value,
	onChange,
}: {
	value: string
	onChange: (value: string) => void
}): React.JSX.Element {
	return (
		<InputGroup className="h-10 w-full sm:max-w-sm">
			<InputGroupAddon>
				<SearchIcon aria-hidden="true" />
			</InputGroupAddon>
			<InputGroupInput
				type="search"
				value={value}
				onChange={(event) => onChange(event.target.value)}
				placeholder="Search students, courses, or IDs…"
				autoComplete="off"
				spellCheck={false}
				aria-label="Search participation"
			/>
		</InputGroup>
	)
}

function FilterPopover({
	data,
	filters,
	onChange,
	tab,
}: {
	data: AdminBootstrap
	filters: ParticipationFilters
	onChange: (filters: ParticipationFilters) => void
	tab: ParticipationTab
}): React.JSX.Element {
	const activeCount = [
		filters.grade,
		filters.period,
		filters.type === "all" ? "" : filters.type,
		filters.status === "all" ? "" : filters.status,
		filters.course,
	].filter(Boolean).length

	return (
		<Popover>
			<PopoverTrigger
				render={
					<Button variant="outline">
						<FilterIcon data-icon="inline-start" />
						Filters
						{activeCount > 0 ? (
							<Badge variant="secondary">{activeCount}</Badge>
						) : null}
					</Button>
				}
			/>
			<PopoverContent align="end" className="w-80">
				<PopoverHeader>
					<PopoverTitle>Filters</PopoverTitle>
					<PopoverDescription>
						Narrow the current participation table.
					</PopoverDescription>
				</PopoverHeader>
				<FieldGroup>
					{tab !== "courses" ? (
						<Field>
							<FieldLabel htmlFor="participation-grade-filter">
								Grade
							</FieldLabel>
							<NativeSelect
								id="participation-grade-filter"
								value={filters.grade}
								onChange={(event) =>
									onChange({
										...filters,
										grade: event.target.value,
									})
								}
							>
								<NativeSelectOption value="">
									All grades
								</NativeSelectOption>
								{data.grades.map((grade) => (
									<NativeSelectOption
										key={grade.grade}
										value={grade.grade}
									>
										G{grade.grade}
									</NativeSelectOption>
								))}
							</NativeSelect>
						</Field>
					) : null}
					<Field>
						<FieldLabel htmlFor="participation-period-filter">
							CCA slot
						</FieldLabel>
						<NativeSelect
							id="participation-period-filter"
							value={filters.period}
							onChange={(event) =>
								onChange({
									...filters,
									period: event.target.value,
								})
							}
						>
							<NativeSelectOption value="">
								All slots
							</NativeSelectOption>
							{data.periods.map((period) => (
								<NativeSelectOption key={period} value={period}>
									{period}
								</NativeSelectOption>
							))}
						</NativeSelect>
					</Field>
					{tab === "assignments" ? (
						<Field>
							<FieldLabel htmlFor="participation-type-filter">
								Assignment type
							</FieldLabel>
							<NativeSelect
								id="participation-type-filter"
								value={filters.type}
								onChange={(event) =>
									onChange({
										...filters,
										type: event.target
											.value as ParticipationFilters["type"],
									})
								}
							>
								<NativeSelectOption value="all">
									All types
								</NativeSelectOption>
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
						</Field>
					) : null}
					{tab === "students" ? (
						<Field>
							<FieldLabel htmlFor="participation-status-filter">
								Completion status
							</FieldLabel>
							<NativeSelect
								id="participation-status-filter"
								value={filters.status}
								onChange={(event) =>
									onChange({
										...filters,
										status: event.target
											.value as CompletionStatus,
									})
								}
							>
								<NativeSelectOption value="all">
									All statuses
								</NativeSelectOption>
								<NativeSelectOption value="unassigned">
									Unassigned
								</NativeSelectOption>
								<NativeSelectOption value="incomplete">
									Incomplete
								</NativeSelectOption>
								<NativeSelectOption value="complete">
									Complete
								</NativeSelectOption>
							</NativeSelect>
						</Field>
					) : null}
					{tab !== "courses" ? (
						<Field>
							<FieldLabel htmlFor="participation-course-filter">
								Course
							</FieldLabel>
							<NativeSelect
								id="participation-course-filter"
								value={filters.course}
								onChange={(event) =>
									onChange({
										...filters,
										course: event.target.value,
									})
								}
							>
								<NativeSelectOption value="">
									All courses
								</NativeSelectOption>
								{data.courses.map((course) => (
									<NativeSelectOption
										key={course.id}
										value={course.id}
									>
										{course.name}
									</NativeSelectOption>
								))}
							</NativeSelect>
						</Field>
					) : null}
				</FieldGroup>
				<Button
					variant="outline"
					onClick={() =>
						onChange({ ...EMPTY_FILTERS, query: filters.query })
					}
				>
					Reset filters
				</Button>
			</PopoverContent>
		</Popover>
	)
}

function TableToolbar({
	data,
	filters,
	onChange,
	tab,
	children,
}: {
	data: AdminBootstrap
	filters: ParticipationFilters
	onChange: (filters: ParticipationFilters) => void
	tab: ParticipationTab
	children?: React.ReactNode
}): React.JSX.Element {
	return (
		<div className="flex flex-col gap-3 py-4 sm:flex-row sm:items-center sm:justify-between">
			<SearchBox
				value={filters.query}
				onChange={(query) => onChange({ ...filters, query })}
			/>
			<div className="flex flex-wrap items-center gap-2">
				{children}
				<FilterPopover
					data={data}
					filters={filters}
					onChange={onChange}
					tab={tab}
				/>
			</div>
		</div>
	)
}

function PaginationControls({
	page,
	total,
	onPageChange,
}: {
	page: number
	total: number
	onPageChange: (page: number) => void
}): React.JSX.Element {
	const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE))
	const start = total === 0 ? 0 : (page - 1) * PAGE_SIZE + 1
	const end = Math.min(page * PAGE_SIZE, total)
	const pageItems: Array<number | "start-ellipsis" | "end-ellipsis"> =
		pageCount <= 7
			? Array.from({ length: pageCount }, (_, index) => index + 1)
			: [
					1,
					...(page > 4 ? (["start-ellipsis"] as const) : []),
					...Array.from(
						{ length: Math.min(5, pageCount - 2) },
						(_, index) =>
							Math.max(2, Math.min(page - 2, pageCount - 5)) +
							index,
					),
					...(page < pageCount - 3
						? (["end-ellipsis"] as const)
						: []),
					pageCount,
				]

	function navigate(
		event: React.MouseEvent<HTMLAnchorElement>,
		nextPage: number,
	): void {
		event.preventDefault()
		if (nextPage < 1 || nextPage > pageCount || nextPage === page) return
		onPageChange(nextPage)
	}

	return (
		<div className="flex items-center justify-between gap-3 py-4">
			<p className="text-sm text-muted-foreground">
				{start}–{end} of {total}
			</p>
			<Pagination className="mx-0 w-auto justify-end">
				<PaginationContent>
					<PaginationItem>
						<PaginationPrevious
							href="#"
							aria-disabled={page <= 1}
							tabIndex={page <= 1 ? -1 : undefined}
							className={cn(
								page <= 1 && "pointer-events-none opacity-50",
							)}
							onClick={(event) => navigate(event, page - 1)}
						/>
					</PaginationItem>
					{pageItems.map((item) =>
						typeof item === "number" ? (
							<PaginationItem key={item}>
								<PaginationLink
									href="#"
									isActive={item === page}
									aria-label={`Go to page ${item}`}
									onClick={(event) => navigate(event, item)}
								>
									{item}
								</PaginationLink>
							</PaginationItem>
						) : (
							<PaginationItem key={item}>
								<PaginationEllipsis />
							</PaginationItem>
						),
					)}
					<PaginationItem>
						<PaginationNext
							href="#"
							aria-disabled={page >= pageCount}
							tabIndex={page >= pageCount ? -1 : undefined}
							className={cn(
								page >= pageCount &&
									"pointer-events-none opacity-50",
							)}
							onClick={(event) => navigate(event, page + 1)}
						/>
					</PaginationItem>
				</PaginationContent>
			</Pagination>
		</div>
	)
}

function EmptyTable({
	title,
	description,
}: {
	title: string
	description: string
}): React.JSX.Element {
	return (
		<Empty>
			<EmptyHeader>
				<EmptyTitle>{title}</EmptyTitle>
				<EmptyDescription>{description}</EmptyDescription>
			</EmptyHeader>
		</Empty>
	)
}

function openRow(
	event: React.KeyboardEvent<HTMLTableRowElement>,
	open: () => void,
): void {
	if (event.key === "Enter" || event.key === " ") {
		event.preventDefault()
		open()
	}
}

function StudentActions({
	student,
	onOpen,
	onEdit,
	onAssign,
}: {
	student: Student
	onOpen: () => void
	onEdit: () => void
	onAssign: () => void
}): React.JSX.Element {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger
				render={
					<Button
						variant="ghost"
						size="icon-sm"
						aria-label={`Actions for ${student.name}`}
						onClick={(event) => event.stopPropagation()}
					/>
				}
			>
				<MoreHorizontalIcon />
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuGroup>
					<DropdownMenuLabel>{student.name}</DropdownMenuLabel>
					<DropdownMenuItem onClick={onOpen}>
						<UserRoundIcon />
						View participation
					</DropdownMenuItem>
					<DropdownMenuItem onClick={onAssign}>
						<PlusIcon />
						Assign course
					</DropdownMenuItem>
					<DropdownMenuItem onClick={onEdit}>
						<PencilIcon />
						Edit profile
					</DropdownMenuItem>
				</DropdownMenuGroup>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}

function CourseActions({
	course,
	onOpen,
	onAssign,
}: {
	course: Course
	onOpen: () => void
	onAssign: () => void
}): React.JSX.Element {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger
				render={
					<Button
						variant="ghost"
						size="icon-sm"
						aria-label={`Participation actions for ${course.name}`}
						onClick={(event) => event.stopPropagation()}
					/>
				}
			>
				<MoreHorizontalIcon />
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuGroup>
					<DropdownMenuLabel>{course.name}</DropdownMenuLabel>
					<DropdownMenuItem onClick={onOpen}>
						<BookOpenIcon />
						View roster
					</DropdownMenuItem>
					<DropdownMenuItem onClick={onAssign}>
						<PlusIcon />
						Assign students
					</DropdownMenuItem>
				</DropdownMenuGroup>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}

function StudentTimetable({
	selections,
}: {
	selections: readonly Selection[]
}): React.JSX.Element {
	const byPeriod = new Map(
		selections.map((selection) => [selection.period_id, selection]),
	)
	return (
		<Table containerLabel="Student CCA timetable" className="table-fixed">
			<TableCaption className="sr-only">
				Student selections arranged by weekday and CCA slot.
			</TableCaption>
			<TableHeader>
				<TableRow>
					<TableHead className="w-16">Slot</TableHead>
					{CCA_DAYS.map((day) => (
						<TableHead key={day}>{day.slice(0, 3)}</TableHead>
					))}
				</TableRow>
			</TableHeader>
			<TableBody>
				{CCA_SLOTS_PER_DAY.map((slot) => (
					<TableRow key={slot}>
						<TableHead scope="row">CCA {slot}</TableHead>
						{CCA_DAYS.map((day) => {
							const selection = byPeriod.get(
								ccaTimeSlotID(day, slot),
							)
							return (
								<TableCell
									key={day}
									className="whitespace-normal"
								>
									{selection?.course_name ??
										selection?.course_id ??
										"—"}
								</TableCell>
							)
						})}
					</TableRow>
				))}
			</TableBody>
		</Table>
	)
}

interface ParticipationDetailProps {
	target: DetailTarget
	data: AdminBootstrap
	onAssignStudent: (studentID: number) => void
	onAssignCourse: (courseID: string) => void
	onEditStudent: (student: Student) => void
	onUpdateType: (selection: Selection, type: SelectionType) => Promise<void>
	onDeleteSelection: (selection: Selection) => Promise<void>
}

function resolveParticipationDetail(
	target: DetailTarget,
	data: AdminBootstrap,
): {
	student: Student | undefined
	course: Course | undefined
	selections: Selection[]
} {
	const student =
		target?.kind === "student"
			? data.students.find((item) => item.id === target.id)
			: undefined
	const course =
		target?.kind === "course"
			? data.courses.find((item) => item.id === target.id)
			: undefined
	const selections =
		student !== undefined
			? selectionsForStudent(data.selections, student.id)
			: course !== undefined
				? selectionsForCourse(data.selections, course.id)
				: []
	return { student, course, selections }
}

function ParticipationDetailBody({
	student,
	course,
	selections,
	onUpdateType,
	onDeleteSelection,
}: {
	student: Student | undefined
	course: Course | undefined
	selections: readonly Selection[]
	onUpdateType: (selection: Selection, type: SelectionType) => Promise<void>
	onDeleteSelection: (selection: Selection) => Promise<void>
}): React.JSX.Element {
	if (student !== undefined) {
		return (
			<>
				<section className="flex flex-col gap-3">
					<h3 className="font-medium">Timetable</h3>
					<StudentTimetable selections={selections} />
				</section>
				<Separator />
				<SelectionList
					selections={selections}
					onUpdateType={onUpdateType}
					onDelete={onDeleteSelection}
				/>
			</>
		)
	}

	return (
		<>
			<section className="flex flex-col gap-3">
				<h3 className="font-medium">Schedule</h3>
				<PeriodBadges periodIDs={course?.period_ids ?? []} />
			</section>
			<Separator />
			<SelectionList
				title="Roster"
				selections={selections}
				onUpdateType={onUpdateType}
				onDelete={onDeleteSelection}
			/>
		</>
	)
}

function ParticipationDetailPanel({
	target,
	data,
	onClose,
	onAssignStudent,
	onAssignCourse,
	onEditStudent,
	onUpdateType,
	onDeleteSelection,
}: ParticipationDetailProps & { onClose: () => void }): React.JSX.Element {
	const { student, course, selections } = resolveParticipationDetail(
		target,
		data,
	)
	if (student === undefined && course === undefined) {
		return (
			<div className="flex h-full min-h-0 flex-col">
				<header className="flex flex-col gap-1 border-b pb-4">
					<h2 className="font-heading text-base font-medium">
						Participation details
					</h2>
					<p className="text-sm text-muted-foreground">
						Select a student or course to view participation.
					</p>
				</header>
				<div className="flex min-h-0 flex-1">
					<Empty>
						<EmptyHeader>
							<EmptyMedia variant="icon">
								<BookOpenIcon />
							</EmptyMedia>
							<EmptyTitle>Nothing selected</EmptyTitle>
							<EmptyDescription>
								Choose a row from the table to show its details
								here.
							</EmptyDescription>
						</EmptyHeader>
					</Empty>
				</div>
			</div>
		)
	}

	return (
		<div className="flex h-full min-h-0 flex-col">
			<header className="flex items-start justify-between gap-3 border-b pb-4">
				<div className="flex min-w-0 flex-col gap-1">
					<h2 className="truncate font-heading text-base font-medium">
						{student?.name ?? course?.name}
					</h2>
					<p className="text-sm text-muted-foreground">
						{student !== undefined
							? `${student.id} · G${student.grade} · ${student.legal_sex}`
							: course !== undefined
								? `${course.id} · ${course.category_id} · ${course.current_students}/${course.max_students} students`
								: null}
					</p>
				</div>
				<div className="shrink-0">
					<Button
						variant="ghost"
						size="icon-sm"
						onClick={onClose}
						aria-label="Close participation details"
					>
						<XIcon />
					</Button>
				</div>
			</header>
			<ScrollArea className="min-h-0 flex-1">
				<div className="flex flex-col gap-6 py-4 pr-3">
					<ParticipationDetailBody
						student={student}
						course={course}
						selections={selections}
						onUpdateType={onUpdateType}
						onDeleteSelection={onDeleteSelection}
					/>
				</div>
			</ScrollArea>
			<footer className="flex justify-end gap-2 border-t pt-4">
				{student !== undefined ? (
					<>
						<Button
							variant="outline"
							onClick={() => onEditStudent(student)}
						>
							<PencilIcon data-icon="inline-start" />
							Edit profile
						</Button>
						<Button onClick={() => onAssignStudent(student.id)}>
							<PlusIcon data-icon="inline-start" />
							Assign course
						</Button>
					</>
				) : course !== undefined ? (
					<Button onClick={() => onAssignCourse(course.id)}>
						<PlusIcon data-icon="inline-start" />
						Assign students
					</Button>
				) : null}
			</footer>
		</div>
	)
}

function ParticipationSheet({
	open,
	target,
	data,
	onOpenChange,
	onAssignStudent,
	onAssignCourse,
	onEditStudent,
	onUpdateType,
	onDeleteSelection,
}: ParticipationDetailProps & {
	open: boolean
	onOpenChange: (open: boolean) => void
}): React.JSX.Element {
	const { student, course, selections } = resolveParticipationDetail(
		target,
		data,
	)

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent className="w-full sm:max-w-2xl">
				{student !== undefined ? (
					<>
						<SheetHeader>
							<SheetTitle>{student.name}</SheetTitle>
							<SheetDescription>
								{student.id} · G{student.grade} ·{" "}
								{student.legal_sex}
							</SheetDescription>
						</SheetHeader>
						<ScrollArea className="min-h-0 flex-1 px-4">
							<div className="flex flex-col gap-6 pb-4">
								<ParticipationDetailBody
									student={student}
									course={course}
									selections={selections}
									onUpdateType={onUpdateType}
									onDeleteSelection={onDeleteSelection}
								/>
							</div>
						</ScrollArea>
						<SheetFooter className="flex-row justify-end border-t">
							<Button
								variant="outline"
								onClick={() => onEditStudent(student)}
							>
								<PencilIcon data-icon="inline-start" />
								Edit profile
							</Button>
							<Button onClick={() => onAssignStudent(student.id)}>
								<PlusIcon data-icon="inline-start" />
								Assign course
							</Button>
						</SheetFooter>
					</>
				) : course !== undefined ? (
					<>
						<SheetHeader>
							<SheetTitle>{course.name}</SheetTitle>
							<SheetDescription>
								{course.id} · {course.category_id} ·{" "}
								{course.current_students}/{course.max_students}{" "}
								students
							</SheetDescription>
						</SheetHeader>
						<ScrollArea className="min-h-0 flex-1 px-4">
							<div className="flex flex-col gap-6 pb-4">
								<ParticipationDetailBody
									student={student}
									course={course}
									selections={selections}
									onUpdateType={onUpdateType}
									onDeleteSelection={onDeleteSelection}
								/>
							</div>
						</ScrollArea>
						<SheetFooter className="flex-row justify-end border-t">
							<Button onClick={() => onAssignCourse(course.id)}>
								<PlusIcon data-icon="inline-start" />
								Assign students
							</Button>
						</SheetFooter>
					</>
				) : (
					<SheetHeader>
						<SheetTitle>Participation details</SheetTitle>
						<SheetDescription>
							The selected record is no longer available.
						</SheetDescription>
					</SheetHeader>
				)}
			</SheetContent>
		</Sheet>
	)
}

function SelectionList({
	selections,
	onUpdateType,
	onDelete,
	title = "Selections",
}: {
	selections: readonly Selection[]
	onUpdateType: (selection: Selection, type: SelectionType) => Promise<void>
	onDelete: (selection: Selection) => Promise<void>
	title?: string
}): React.JSX.Element {
	return (
		<section className="flex flex-col gap-3">
			<div className="flex items-center justify-between gap-3">
				<h3 className="font-medium">{title}</h3>
				<Badge variant="outline">{selections.length}</Badge>
			</div>
			{selections.length === 0 ? (
				<EmptyTable
					title="No selections"
					description="Use Assign to add a course selection."
				/>
			) : (
				<Table containerLabel={`${title} table`}>
					<TableHeader>
						<TableRow>
							<TableHead>Student</TableHead>
							<TableHead>Course</TableHead>
							<TableHead>Schedule</TableHead>
							<TableHead>Type</TableHead>
							<TableHead>
								<span className="sr-only">Actions</span>
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{selections.map((selection) => (
							<TableRow
								key={`${selection.student_id}-${selection.course_id}`}
							>
								<TableCell>
									{selection.student_name ??
										selection.student_id}
								</TableCell>
								<TableCell>
									{selection.course_name ??
										selection.course_id}
								</TableCell>
								<TableCell>
									<PeriodBadges
										periodIDs={[selection.period_id]}
									/>
								</TableCell>
								<TableCell>
									<SelectionTypeBadge
										type={selection.selection_type}
									/>
								</TableCell>
								<TableCell>
									<SelectionActions
										selection={selection}
										onUpdateType={onUpdateType}
										onDelete={onDelete}
									/>
								</TableCell>
							</TableRow>
						))}
					</TableBody>
				</Table>
			)}
		</section>
	)
}

function SelectionActions({
	selection,
	onUpdateType,
	onDelete,
}: {
	selection: Selection
	onUpdateType: (selection: Selection, type: SelectionType) => Promise<void>
	onDelete: (selection: Selection) => Promise<void>
}): React.JSX.Element {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger
				render={
					<Button
						variant="ghost"
						size="icon-sm"
						aria-label={`Actions for ${selection.course_name ?? selection.course_id}`}
						onClick={(event) => event.stopPropagation()}
					/>
				}
			>
				<MoreHorizontalIcon />
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuGroup>
					<DropdownMenuLabel>Change type</DropdownMenuLabel>
					<DropdownMenuItem
						onClick={() => void onUpdateType(selection, "normal")}
					>
						Student choice
					</DropdownMenuItem>
					<DropdownMenuItem
						onClick={() => void onUpdateType(selection, "invite")}
					>
						Invitation
					</DropdownMenuItem>
					<DropdownMenuItem
						onClick={() => void onUpdateType(selection, "force")}
					>
						Forced
					</DropdownMenuItem>
				</DropdownMenuGroup>
				<DropdownMenuSeparator />
				<DropdownMenuGroup>
					<DropdownMenuItem
						variant="destructive"
						onClick={() => void onDelete(selection)}
					>
						<Trash2Icon />
						Remove selection
					</DropdownMenuItem>
				</DropdownMenuGroup>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}

function assignmentIssue(
	student: Student,
	course: Course,
	periodID: string,
	type: SelectionType,
	data: AdminBootstrap,
): string | null {
	const studentSelections = selectionsForStudent(data.selections, student.id)
	if (
		studentSelections.some((selection) => selection.course_id === course.id)
	) {
		return "Already assigned to this course"
	}
	if (
		studentSelections.some((selection) => selection.period_id === periodID)
	) {
		return "Timetable clash"
	}
	if (type !== "normal") return null
	if (course.membership === "invite_only") return "Invitation required"
	if (
		course.allowed_legal_sexes.length > 0 &&
		!course.allowed_legal_sexes.includes(student.legal_sex)
	) {
		return "Not available for this student"
	}
	if (
		course.allowed_grades.length > 0 &&
		!course.allowed_grades.includes(student.grade)
	) {
		return "Not available for this grade"
	}
	const grade = data.grades.find((item) => item.grade === student.grade)
	if (grade === undefined || !grade.enabled) return "Selections are closed"
	const normalCount = studentSelections.filter(
		(selection) => selection.selection_type === "normal",
	).length
	if (normalCount >= grade.max_own_choices) return "Selection limit reached"
	if (course.current_students >= course.max_students) return "Course is full"
	return null
}

function AssignmentDialog({
	open,
	onOpenChange,
	data,
	refresh,
	initialStudentID,
	initialCourseID,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	data: AdminBootstrap
	refresh: () => Promise<void>
	initialStudentID?: number
	initialCourseID?: string
}): React.JSX.Element {
	const initialCourse = data.courses.find(
		(course) => course.id === initialCourseID,
	)
	const [courseID, setCourseID] = useState(initialCourseID ?? "")
	const [periodID, setPeriodID] = useState(initialCourse?.period_ids[0] ?? "")
	const [studentQuery, setStudentQuery] = useState("")
	const [studentIDs, setStudentIDs] = useState<number[]>(
		initialStudentID === undefined ? [] : [initialStudentID],
	)
	const [selectionType, setSelectionType] = useState<SelectionType>("invite")
	const [busy, setBusy] = useState(false)
	const course = data.courses.find((item) => item.id === courseID)
	const matchingStudents = useMemo(() => {
		const matches = data.students.filter((student) =>
			textMatches(
				`${student.id} ${student.name} ${student.grade}`,
				studentQuery,
			),
		)
		if (matches.length <= ASSIGNMENT_STUDENT_WINDOW_SIZE) return matches

		const selectedIndex = matches.findIndex(
			(student) => student.id === initialStudentID,
		)
		if (selectedIndex < 0)
			return matches.slice(0, ASSIGNMENT_STUDENT_WINDOW_SIZE)

		const centeredStart =
			selectedIndex - Math.floor(ASSIGNMENT_STUDENT_WINDOW_SIZE / 2)
		const start = Math.min(
			Math.max(centeredStart, 0),
			matches.length - ASSIGNMENT_STUDENT_WINDOW_SIZE,
		)
		return matches.slice(start, start + ASSIGNMENT_STUDENT_WINDOW_SIZE)
	}, [data.students, initialStudentID, studentQuery])
	const remainingNormalCapacity =
		course === undefined
			? 0
			: Math.max(0, course.max_students - course.current_students)
	const baseReview = studentIDs.map((studentID) => {
		const student = data.students.find((item) => item.id === studentID)
		const issue =
			student === undefined || course === undefined || periodID === ""
				? "Choose a course and slot"
				: assignmentIssue(
						student,
						course,
						periodID,
						selectionType,
						data,
					)
		return {
			student,
			issue,
		}
	})
	const review = baseReview.map((item, index) => ({
		...item,
		issue:
			item.issue === null &&
			selectionType === "normal" &&
			baseReview
				.slice(0, index + 1)
				.filter((candidate) => candidate.issue === null).length >
				remainingNormalCapacity
				? "Course capacity would be exceeded"
				: item.issue,
	}))
	const invalidCount = review.filter((item) => item.issue !== null).length

	useEffect(() => {
		if (!open || initialStudentID === undefined) return
		const frame = window.requestAnimationFrame(() => {
			const checkbox = document.querySelector<HTMLElement>(
				`[data-slot="checkbox"][data-assignment-student-id="${initialStudentID}"]`,
			)
			const studentRow = checkbox?.closest<HTMLElement>(
				'[data-slot="field"]',
			)
			const studentViewport = checkbox?.closest<HTMLElement>(
				'[data-slot="scroll-area-viewport"]',
			)
			const dialogViewport =
				studentViewport?.parentElement?.closest<HTMLElement>(
					'[data-slot="scroll-area-viewport"]',
				)

			dialogViewport?.scrollTo({ top: 0, behavior: "auto" })
			if (
				studentRow !== undefined &&
				studentRow !== null &&
				studentViewport !== undefined &&
				studentViewport !== null
			) {
				const rowRect = studentRow.getBoundingClientRect()
				const viewportRect = studentViewport.getBoundingClientRect()
				const centeredTop =
					studentViewport.scrollTop +
					rowRect.top -
					viewportRect.top -
					(studentViewport.clientHeight - rowRect.height) / 2
				studentViewport.scrollTo({
					top: Math.max(0, centeredTop),
					behavior: "auto",
				})
			}
			checkbox?.focus({ preventScroll: true })
		})
		return () => window.cancelAnimationFrame(frame)
	}, [initialStudentID, open])

	function chooseCourse(nextCourseID: string): void {
		setCourseID(nextCourseID)
		const nextCourse = data.courses.find((item) => item.id === nextCourseID)
		setPeriodID(nextCourse?.period_ids[0] ?? "")
	}

	async function submit(
		event: React.FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault()
		if (course === undefined || periodID === "" || invalidCount > 0) return
		setBusy(true)
		try {
			await apiRequest("/api/v1/admin/selections", {
				method: "POST",
				body: jsonBody({
					student_ids: studentIDs,
					course_ids: [course.id],
					period_ids: [periodID],
					selection_type: selectionType,
				}),
			})
			await refresh()
			toast.success(`${studentIDs.length} assignments created.`)
			onOpenChange(false)
		} catch (caught) {
			toast.error(
				caught instanceof Error
					? caught.message
					: "Unable to create assignments.",
			)
		} finally {
			setBusy(false)
		}
	}

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="h-[calc(100dvh-2rem)] max-h-[48rem] overflow-hidden p-0 sm:max-w-3xl">
				<form
					className="grid h-full min-h-0 grid-rows-[auto_minmax(0,1fr)_auto]"
					onSubmit={(event) => void submit(event)}
				>
					<DialogHeader className="px-6 pt-6 pb-4 pr-12">
						<DialogTitle>Assign students</DialogTitle>
						<DialogDescription>
							Assign one exact course slot to one or more
							students.
						</DialogDescription>
					</DialogHeader>
					<ScrollArea className="min-h-0">
						<FieldGroup className="gap-6 px-6 py-5">
							<Field>
								<FieldLabel htmlFor="assignment-course">
									Course
								</FieldLabel>
								<Combobox
									items={data.courses}
									value={course ?? null}
									onValueChange={(nextCourse) =>
										chooseCourse(nextCourse?.id ?? "")
									}
									itemToStringLabel={(item) => item.name}
									itemToStringValue={(item) => item.id}
									isItemEqualToValue={(item, value) =>
										item.id === value.id
									}
									filter={(item, query) =>
										textMatches(
											[
												item.id,
												item.name,
												item.category_id,
												item.teacher,
												item.location,
												...item.period_ids,
											].join(" "),
											query,
										)
									}
									autoHighlight
								>
									<ComboboxInput
										id="assignment-course"
										placeholder="Search CCAs..."
										showClear
									/>
									<ComboboxContent>
										<ComboboxEmpty>
											No CCAs found.
										</ComboboxEmpty>
										<ComboboxList>
											{(item) => (
												<ComboboxItem
													key={item.id}
													value={item}
												>
													<div className="flex min-w-0 flex-col gap-0.5">
														<span className="truncate font-medium">
															{item.name}
														</span>
														<span className="truncate text-xs text-muted-foreground">
															{item.id} ·{" "}
															{item.category_id} ·{" "}
															{item.teacher}
														</span>
														<span className="truncate text-xs text-muted-foreground">
															{item.period_ids.join(
																" · ",
															)}
														</span>
													</div>
												</ComboboxItem>
											)}
										</ComboboxList>
									</ComboboxContent>
								</Combobox>
								<FieldDescription>
									Search by name, ID, category, teacher,
									location, or slot.
								</FieldDescription>
							</Field>
							<FieldSet>
								<FieldLegend variant="label">
									Exact CCA slot
								</FieldLegend>
								{course === undefined ? (
									<FieldDescription>
										Choose a course first.
									</FieldDescription>
								) : (
									<ToggleGroup
										value={
											periodID === "" ? [] : [periodID]
										}
										onValueChange={(values) => {
											const next = values[0]
											if (next !== undefined)
												setPeriodID(next)
										}}
										variant="outline"
										className="flex-wrap"
									>
										{course.period_ids.map((period) => (
											<ToggleGroupItem
												key={period}
												value={period}
											>
												{period}
											</ToggleGroupItem>
										))}
									</ToggleGroup>
								)}
							</FieldSet>
							<FieldSet>
								<FieldLegend variant="label">
									Students
								</FieldLegend>
								<SearchBox
									value={studentQuery}
									onChange={setStudentQuery}
								/>
								<ScrollArea className="h-56 rounded-lg border">
									<FieldGroup
										data-slot="checkbox-group"
										className="p-3 pr-5"
									>
										{matchingStudents.map((student) => (
											<Field
												key={student.id}
												orientation="horizontal"
											>
												<Checkbox
													id={`assignment-student-${student.id}`}
													data-assignment-student-id={
														student.id
													}
													checked={studentIDs.includes(
														student.id,
													)}
													onCheckedChange={(
														checked,
													) =>
														setStudentIDs(
															(current) =>
																checked
																	? [
																			...current,
																			student.id,
																		]
																	: current.filter(
																			(
																				id,
																			) =>
																				id !==
																				student.id,
																		),
														)
													}
												/>
												<FieldContent>
													<FieldLabel
														htmlFor={`assignment-student-${student.id}`}
													>
														{student.name}
													</FieldLabel>
													<FieldDescription>
														{student.id} · G
														{student.grade}
													</FieldDescription>
												</FieldContent>
											</Field>
										))}
									</FieldGroup>
								</ScrollArea>
							</FieldSet>
							<FieldSet>
								<FieldLegend variant="label">
									Assignment type
								</FieldLegend>
								<ToggleGroup
									value={[selectionType]}
									onValueChange={(values) => {
										const next = values[0] as
											| SelectionType
											| undefined
										if (next !== undefined)
											setSelectionType(next)
									}}
									variant="outline"
								>
									<ToggleGroupItem value="normal">
										Student choice
									</ToggleGroupItem>
									<ToggleGroupItem value="invite">
										Invitation
									</ToggleGroupItem>
									<ToggleGroupItem value="force">
										Forced
									</ToggleGroupItem>
								</ToggleGroup>
							</FieldSet>
							{selectionType === "force" ? (
								<Alert variant="destructive">
									<AlertTitle>Forced assignment</AlertTitle>
									<AlertDescription>
										Students cannot remove forced selections
										themselves.
									</AlertDescription>
								</Alert>
							) : null}
							<FieldSet>
								<FieldLegend variant="label">
									Review
								</FieldLegend>
								<FieldDescription>
									{studentIDs.length} selected ·{" "}
									{invalidCount} need attention
								</FieldDescription>
								{review.length > 0 ? (
									<Table containerLabel="Assignment review table">
										<TableHeader>
											<TableRow>
												<TableHead>Student</TableHead>
												<TableHead>Status</TableHead>
											</TableRow>
										</TableHeader>
										<TableBody>
											{review.map((item) => (
												<TableRow
													key={
														item.student?.id ??
														"missing"
													}
												>
													<TableCell>
														{item.student?.name ??
															"Unknown student"}
													</TableCell>
													<TableCell>
														{item.issue === null ? (
															<Badge variant="secondary">
																Ready
															</Badge>
														) : (
															<Badge variant="destructive">
																{item.issue}
															</Badge>
														)}
													</TableCell>
												</TableRow>
											))}
										</TableBody>
									</Table>
								) : (
									<FieldDescription>
										Select at least one student.
									</FieldDescription>
								)}
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
							disabled={
								busy ||
								studentIDs.length === 0 ||
								course === undefined ||
								periodID === "" ||
								invalidCount > 0
							}
						>
							{busy ? (
								<Spinner data-icon="inline-start" />
							) : (
								<PlusIcon data-icon="inline-start" />
							)}
							Create {studentIDs.length || ""} assignment
							{studentIDs.length === 1 ? "" : "s"}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}

export function ParticipationPage({
	data,
	refresh,
}: AdminPageProps): React.JSX.Element {
	const [tab, setTab] = useState<ParticipationTab>("students")
	const [filters, setFilters] = useState<ParticipationFilters>(EMPTY_FILTERS)
	const [page, setPage] = useState(1)
	const [detail, setDetail] = useState<DetailTarget>(null)
	const [studentDialogOpen, setStudentDialogOpen] = useState(false)
	const [editingStudent, setEditingStudent] = useState<Student | null>(null)
	const [assignmentOpen, setAssignmentOpen] = useState(false)
	const [assignmentKey, setAssignmentKey] = useState(0)
	const [initialStudentID, setInitialStudentID] = useState<
		number | undefined
	>()
	const [initialCourseID, setInitialCourseID] = useState<string | undefined>()
	const [selectedAssignments, setSelectedAssignments] = useState<string[]>([])
	const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false)
	const [bulkBusy, setBulkBusy] = useState(false)
	const [desktopLayout] = useState(loadParticipationLayout)
	const isWideDetailLayout = useMediaQuery("(min-width: 1024px)")

	const studentRows = useMemo(() => {
		const courseByID = new Map(
			data.courses.map((course) => [course.id, course]),
		)
		return data.students
			.map((student) => {
				const selections = selectionsForStudent(
					data.selections,
					student.id,
				)
				const selectedCourseSearchText = selections
					.map((selection) => {
						const course = courseByID.get(selection.course_id)
						return [
							selection.course_id,
							selection.course_name,
							selection.period_id,
							course?.category_id,
							course?.teacher,
							course?.location,
						]
							.filter(Boolean)
							.join(" ")
					})
					.join(" ")
				return {
					student,
					selections,
					searchText: `${student.id} ${student.name} ${student.grade} ${selectedCourseSearchText}`,
					status: studentCompletionStatus(student, selections, data),
				}
			})
			.filter((row) => {
				if (!textMatches(row.searchText, filters.query)) {
					return false
				}
				if (filters.grade !== "" && row.student.grade !== filters.grade)
					return false
				if (filters.status !== "all" && row.status !== filters.status)
					return false
				if (
					filters.period !== "" &&
					!row.selections.some(
						(selection) => selection.period_id === filters.period,
					)
				) {
					return false
				}
				if (
					filters.course !== "" &&
					!row.selections.some(
						(selection) => selection.course_id === filters.course,
					)
				) {
					return false
				}
				return true
			})
	}, [data, filters])

	const courseRows = useMemo(
		() =>
			data.courses.filter((course) => {
				if (
					!textMatches(
						`${course.id} ${course.name} ${course.teacher} ${course.category_id}`,
						filters.query,
					)
				) {
					return false
				}
				return (
					filters.period === "" ||
					course.period_ids.includes(filters.period)
				)
			}),
		[data.courses, filters.period, filters.query],
	)

	const assignmentRows = useMemo(
		() =>
			data.selections.filter((selection) => {
				if (
					!textMatches(
						`${selection.student_id ?? ""} ${selection.student_name ?? ""} ${selection.course_id} ${selection.course_name ?? ""}`,
						filters.query,
					)
				) {
					return false
				}
				if (
					filters.grade !== "" &&
					selection.student_grade !== filters.grade
				)
					return false
				if (
					filters.period !== "" &&
					selection.period_id !== filters.period
				)
					return false
				if (
					filters.type !== "all" &&
					selection.selection_type !== filters.type
				)
					return false
				if (
					filters.course !== "" &&
					selection.course_id !== filters.course
				)
					return false
				return true
			}),
		[data.selections, filters],
	)

	function changeTab(value: string): void {
		setTab(value as ParticipationTab)
		setFilters(EMPTY_FILTERS)
		setPage(1)
		setSelectedAssignments([])
		setDetail(null)
	}

	function changeFilters(next: ParticipationFilters): void {
		setFilters(next)
		setPage(1)
		setSelectedAssignments([])
	}

	function openAssignment(studentID?: number, courseID?: string): void {
		if (!isWideDetailLayout) setDetail(null)
		setInitialStudentID(studentID)
		setInitialCourseID(courseID)
		setAssignmentKey((current) => current + 1)
		setAssignmentOpen(true)
	}

	function editStudent(student: Student): void {
		if (!isWideDetailLayout) setDetail(null)
		setEditingStudent(student)
		setStudentDialogOpen(true)
	}

	async function updateSelectionType(
		selection: Selection,
		type: SelectionType,
	): Promise<void> {
		if (
			selection.student_id === undefined ||
			type === selection.selection_type
		)
			return
		try {
			await apiRequest(
				`/api/v1/admin/selections/${selection.student_id}/${encodeURIComponent(selection.course_id)}`,
				{
					method: "PUT",
					body: jsonBody({
						course_id: selection.course_id,
						period_id: selection.period_id,
						selection_type: type,
					}),
				},
			)
			await refresh()
			toast.success("Assignment type updated.")
		} catch (caught) {
			toast.error(
				caught instanceof Error
					? caught.message
					: "Unable to update assignment.",
			)
		}
	}

	async function deleteSelection(selection: Selection): Promise<void> {
		if (selection.student_id === undefined) return
		try {
			await apiRequest(
				`/api/v1/admin/selections/${selection.student_id}/${encodeURIComponent(selection.course_id)}`,
				{ method: "DELETE" },
			)
			await refresh()
			toast.success("Selection removed.")
		} catch (caught) {
			toast.error(
				caught instanceof Error
					? caught.message
					: "Unable to remove selection.",
			)
		}
	}

	async function bulkUpdate(type: SelectionType): Promise<void> {
		const selected = data.selections.filter((selection) =>
			selectedAssignments.includes(
				`${selection.student_id}-${selection.course_id}`,
			),
		)
		setBulkBusy(true)
		try {
			await Promise.all(
				selected.map((selection) => {
					if (selection.student_id === undefined)
						return Promise.resolve()
					return apiRequest(
						`/api/v1/admin/selections/${selection.student_id}/${encodeURIComponent(selection.course_id)}`,
						{
							method: "PUT",
							body: jsonBody({
								course_id: selection.course_id,
								period_id: selection.period_id,
								selection_type: type,
							}),
						},
					)
				}),
			)
			await refresh()
			setSelectedAssignments([])
			toast.success(`${selected.length} assignments updated.`)
		} catch (caught) {
			toast.error(
				caught instanceof Error
					? caught.message
					: "Unable to update assignments.",
			)
		} finally {
			setBulkBusy(false)
		}
	}

	async function bulkDelete(): Promise<void> {
		const selected = data.selections.filter((selection) =>
			selectedAssignments.includes(
				`${selection.student_id}-${selection.course_id}`,
			),
		)
		setBulkBusy(true)
		try {
			await Promise.all(
				selected.map((selection) => {
					if (selection.student_id === undefined)
						return Promise.resolve()
					return apiRequest(
						`/api/v1/admin/selections/${selection.student_id}/${encodeURIComponent(selection.course_id)}`,
						{ method: "DELETE" },
					)
				}),
			)
			await refresh()
			setSelectedAssignments([])
			setBulkDeleteOpen(false)
			toast.success(`${selected.length} assignments removed.`)
		} catch (caught) {
			toast.error(
				caught instanceof Error
					? caught.message
					: "Unable to remove assignments.",
			)
		} finally {
			setBulkBusy(false)
		}
	}

	const currentStudents = studentRows.slice(
		(page - 1) * PAGE_SIZE,
		page * PAGE_SIZE,
	)
	const currentCourses = courseRows.slice(
		(page - 1) * PAGE_SIZE,
		page * PAGE_SIZE,
	)
	const currentAssignments = assignmentRows.slice(
		(page - 1) * PAGE_SIZE,
		page * PAGE_SIZE,
	)
	const currentAssignmentKeys = currentAssignments.map(
		(selection) => `${selection.student_id}-${selection.course_id}`,
	)
	const allCurrentAssignmentsSelected =
		currentAssignmentKeys.length > 0 &&
		currentAssignmentKeys.every((key) => selectedAssignments.includes(key))

	const mainContent = (
		<div className="min-w-0">
			<div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
				<div className="flex flex-col gap-1">
					<h1 className="font-heading text-xl font-semibold tracking-tight">
						Participation
					</h1>
					<p className="max-w-3xl text-sm text-muted-foreground">
						Manage student progress, course rosters, and every
						active assignment.
					</p>
				</div>
				<Button onClick={() => openAssignment()}>
					<PlusIcon data-icon="inline-start" />
					Assign students
				</Button>
			</div>

			<Tabs
				value={tab}
				onValueChange={changeTab}
				className="min-w-0 gap-0"
			>
				<div className="relative">
					<Separator
						aria-hidden="true"
						className="absolute bottom-1 left-0"
					/>
					<div className="overflow-x-auto overflow-y-hidden pb-2">
						<TabsList
							variant="line"
							className="h-10 min-w-max justify-start"
						>
							<TabsTrigger value="students">Students</TabsTrigger>
							<TabsTrigger value="courses">Courses</TabsTrigger>
							<TabsTrigger value="assignments">
								All assignments
							</TabsTrigger>
						</TabsList>
					</div>
				</div>

				<TabsContent value="students">
					<TableToolbar
						data={data}
						filters={filters}
						onChange={changeFilters}
						tab="students"
					/>
					{currentStudents.length === 0 ? (
						<EmptyTable
							title="No matching students"
							description="Change the search or filters."
						/>
					) : (
						<Table containerLabel="Student participation table">
							<TableCaption className="sr-only">
								Students matching the participation filters.
							</TableCaption>
							<TableHeader>
								<TableRow>
									<TableHead>Student</TableHead>
									<TableHead>Grade</TableHead>
									<TableHead>Progress</TableHead>
									<TableHead>Selected slots</TableHead>
									<TableHead>Status</TableHead>
									<TableHead className="text-right">
										Actions
									</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{currentStudents.map((row) => {
									const grade = data.grades.find(
										(item) =>
											item.grade === row.student.grade,
									)
									return (
										<TableRow
											key={row.student.id}
											data-state={
												detail?.kind === "student" &&
												detail.id === row.student.id
													? "selected"
													: undefined
											}
											tabIndex={0}
											onClick={() =>
												setDetail({
													kind: "student",
													id: row.student.id,
												})
											}
											onKeyDown={(event) =>
												openRow(event, () =>
													setDetail({
														kind: "student",
														id: row.student.id,
													}),
												)
											}
										>
											<TableCell>
												<p className="font-medium">
													{row.student.name}
												</p>
												<p className="font-mono text-xs text-muted-foreground">
													{row.student.id}
												</p>
											</TableCell>
											<TableCell>
												<Badge variant="outline">
													G{row.student.grade}
												</Badge>
											</TableCell>
											<TableCell>
												{row.selections.length}/
												{grade?.max_own_choices ?? 0}
											</TableCell>
											<TableCell className="whitespace-normal">
												<PeriodBadges
													periodIDs={row.selections.map(
														(selection) =>
															selection.period_id,
													)}
												/>
											</TableCell>
											<TableCell>
												<CompletionBadge
													status={row.status}
												/>
											</TableCell>
											<TableCell>
												<div className="flex justify-end">
													<StudentActions
														student={row.student}
														onOpen={() =>
															setDetail({
																kind: "student",
																id: row.student
																	.id,
															})
														}
														onEdit={() =>
															editStudent(
																row.student,
															)
														}
														onAssign={() =>
															openAssignment(
																row.student.id,
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
					<PaginationControls
						page={page}
						total={studentRows.length}
						onPageChange={setPage}
					/>
				</TabsContent>

				<TabsContent value="courses">
					<TableToolbar
						data={data}
						filters={filters}
						onChange={changeFilters}
						tab="courses"
					/>
					{currentCourses.length === 0 ? (
						<EmptyTable
							title="No matching courses"
							description="Change the search or filters."
						/>
					) : (
						<Table containerLabel="Course participation table">
							<TableCaption className="sr-only">
								Courses matching the participation filters.
							</TableCaption>
							<TableHeader>
								<TableRow>
									<TableHead>Course</TableHead>
									<TableHead>Schedule</TableHead>
									<TableHead>Capacity</TableHead>
									<TableHead>Participants</TableHead>
									<TableHead>Availability</TableHead>
									<TableHead className="text-right">
										Actions
									</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{currentCourses.map((course) => {
									const closed = course.max_students === 0
									const full =
										!closed &&
										course.current_students >=
											course.max_students
									return (
										<TableRow
											key={course.id}
											data-state={
												detail?.kind === "course" &&
												detail.id === course.id
													? "selected"
													: undefined
											}
											tabIndex={0}
											onClick={() =>
												setDetail({
													kind: "course",
													id: course.id,
												})
											}
											onKeyDown={(event) =>
												openRow(event, () =>
													setDetail({
														kind: "course",
														id: course.id,
													}),
												)
											}
										>
											<TableCell>
												<p className="font-medium">
													{course.name}
												</p>
												<p className="text-xs text-muted-foreground">
													{course.id} ·{" "}
													{course.category_id}
												</p>
											</TableCell>
											<TableCell className="whitespace-normal">
												<PeriodBadges
													periodIDs={
														course.period_ids
													}
												/>
											</TableCell>
											<TableCell>
												{course.current_students}/
												{course.max_students}
											</TableCell>
											<TableCell>
												{
													selectionsForCourse(
														data.selections,
														course.id,
													).length
												}
											</TableCell>
											<TableCell>
												<Badge
													variant={
														closed || full
															? "destructive"
															: "secondary"
													}
												>
													{closed
														? "Closed"
														: full
															? "Full"
															: "Open"}
												</Badge>
											</TableCell>
											<TableCell>
												<div className="flex justify-end">
													<CourseActions
														course={course}
														onOpen={() =>
															setDetail({
																kind: "course",
																id: course.id,
															})
														}
														onAssign={() =>
															openAssignment(
																undefined,
																course.id,
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
					<PaginationControls
						page={page}
						total={courseRows.length}
						onPageChange={setPage}
					/>
				</TabsContent>

				<TabsContent value="assignments">
					<TableToolbar
						data={data}
						filters={filters}
						onChange={changeFilters}
						tab="assignments"
					>
						{selectedAssignments.length > 0 ? (
							<>
								<DropdownMenu>
									<DropdownMenuTrigger
										render={
											<Button variant="outline">
												Change type
											</Button>
										}
									/>
									<DropdownMenuContent align="end">
										<DropdownMenuGroup>
											<DropdownMenuItem
												onClick={() =>
													void bulkUpdate("normal")
												}
											>
												Student choice
											</DropdownMenuItem>
											<DropdownMenuItem
												onClick={() =>
													void bulkUpdate("invite")
												}
											>
												Invitation
											</DropdownMenuItem>
											<DropdownMenuItem
												onClick={() =>
													void bulkUpdate("force")
												}
											>
												Forced
											</DropdownMenuItem>
										</DropdownMenuGroup>
									</DropdownMenuContent>
								</DropdownMenu>
								<Button
									variant="destructive"
									onClick={() => setBulkDeleteOpen(true)}
								>
									<Trash2Icon data-icon="inline-start" />
									Remove {selectedAssignments.length}
								</Button>
							</>
						) : null}
					</TableToolbar>
					{currentAssignments.length === 0 ? (
						<EmptyTable
							title="No matching assignments"
							description="Change the search or filters."
						/>
					) : (
						<Table containerLabel="All assignments table">
							<TableCaption className="sr-only">
								Assignments matching the participation filters.
							</TableCaption>
							<TableHeader>
								<TableRow>
									<TableHead>
										<Checkbox
											aria-label="Select all assignments on this page"
											checked={
												allCurrentAssignmentsSelected
											}
											onCheckedChange={(checked) =>
												setSelectedAssignments(
													(current) =>
														checked
															? [
																	...new Set([
																		...current,
																		...currentAssignmentKeys,
																	]),
																]
															: current.filter(
																	(key) =>
																		!currentAssignmentKeys.includes(
																			key,
																		),
																),
												)
											}
										/>
									</TableHead>
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
								{currentAssignments.map((selection) => {
									const key = `${selection.student_id}-${selection.course_id}`
									return (
										<TableRow
											key={key}
											tabIndex={0}
											onClick={() => {
												if (
													selection.student_id !==
													undefined
												) {
													setDetail({
														kind: "student",
														id: selection.student_id,
													})
												}
											}}
											onKeyDown={(event) =>
												openRow(event, () => {
													if (
														selection.student_id !==
														undefined
													) {
														setDetail({
															kind: "student",
															id: selection.student_id,
														})
													}
												})
											}
											data-state={
												selectedAssignments.includes(
													key,
												) ||
												(detail?.kind === "student" &&
													detail.id ===
														selection.student_id)
													? "selected"
													: undefined
											}
										>
											<TableCell>
												<Checkbox
													aria-label={`Select ${selection.student_name ?? selection.student_id} in ${selection.course_name ?? selection.course_id}`}
													checked={selectedAssignments.includes(
														key,
													)}
													onClick={(event) =>
														event.stopPropagation()
													}
													onCheckedChange={(
														checked,
													) =>
														setSelectedAssignments(
															(current) =>
																checked
																	? [
																			...current,
																			key,
																		]
																	: current.filter(
																			(
																				item,
																			) =>
																				item !==
																				key,
																		),
														)
													}
												/>
											</TableCell>
											<TableCell>
												<p className="font-medium">
													{selection.student_name ??
														selection.student_id}
												</p>
												<p className="text-xs text-muted-foreground">
													G{selection.student_grade}
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
												<SelectionTypeBadge
													type={
														selection.selection_type
													}
												/>
											</TableCell>
											<TableCell>
												<div className="flex justify-end">
													<SelectionActions
														selection={selection}
														onUpdateType={
															updateSelectionType
														}
														onDelete={
															deleteSelection
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
					<PaginationControls
						page={page}
						total={assignmentRows.length}
						onPageChange={setPage}
					/>
				</TabsContent>
			</Tabs>
		</div>
	)

	return (
		<>
			{isWideDetailLayout ? (
				<ResizablePanelGroup
					orientation="horizontal"
					defaultLayout={desktopLayout}
					onLayoutChanged={(layout, meta) => {
						if (meta.isUserInteraction)
							saveParticipationLayout(layout)
					}}
					className="h-[calc(100dvh-3rem)] min-h-[32rem]"
				>
					<ResizablePanel
						id={PARTICIPATION_MAIN_PANEL_ID}
						minSize="55%"
					>
						<ScrollArea className="h-full">
							<div className="pr-6 pb-6">{mainContent}</div>
						</ScrollArea>
					</ResizablePanel>
					<ResizableHandle
						withHandle
						aria-label="Resize participation details"
					/>
					<ResizablePanel
						id={PARTICIPATION_DETAILS_PANEL_ID}
						minSize="280px"
						maxSize="45%"
					>
						<aside
							aria-label="Participation details"
							className="h-full pl-6"
						>
							<ParticipationDetailPanel
								target={detail}
								data={data}
								onClose={() => setDetail(null)}
								onAssignStudent={(studentID) =>
									openAssignment(studentID)
								}
								onAssignCourse={(courseID) =>
									openAssignment(undefined, courseID)
								}
								onEditStudent={editStudent}
								onUpdateType={updateSelectionType}
								onDeleteSelection={deleteSelection}
							/>
						</aside>
					</ResizablePanel>
				</ResizablePanelGroup>
			) : (
				mainContent
			)}

			<ParticipationSheet
				open={detail !== null && !isWideDetailLayout}
				target={detail}
				data={data}
				onOpenChange={(open) => {
					if (!open) setDetail(null)
				}}
				onAssignStudent={(studentID) => openAssignment(studentID)}
				onAssignCourse={(courseID) =>
					openAssignment(undefined, courseID)
				}
				onEditStudent={editStudent}
				onUpdateType={updateSelectionType}
				onDeleteSelection={deleteSelection}
			/>

			{studentDialogOpen ? (
				<StudentDialog
					key={editingStudent?.id ?? "new-participation-student"}
					student={editingStudent}
					grades={data.grades}
					open={studentDialogOpen}
					onOpenChange={setStudentDialogOpen}
					refresh={refresh}
				/>
			) : null}

			{assignmentOpen ? (
				<AssignmentDialog
					key={assignmentKey}
					open={assignmentOpen}
					onOpenChange={setAssignmentOpen}
					data={data}
					refresh={refresh}
					{...(initialStudentID === undefined
						? {}
						: { initialStudentID })}
					{...(initialCourseID === undefined
						? {}
						: { initialCourseID })}
				/>
			) : null}

			<AlertDialog open={bulkDeleteOpen} onOpenChange={setBulkDeleteOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogMedia>
							<Trash2Icon />
						</AlertDialogMedia>
						<AlertDialogTitle>
							Remove {selectedAssignments.length} assignments?
						</AlertDialogTitle>
						<AlertDialogDescription>
							This removes the selected student-course
							relationships and frees their exact CCA slots.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel disabled={bulkBusy}>
							Cancel
						</AlertDialogCancel>
						<AlertDialogAction
							variant="destructive"
							disabled={bulkBusy}
							onClick={() => void bulkDelete()}
						>
							{bulkBusy ? (
								<Spinner data-icon="inline-start" />
							) : (
								<Trash2Icon data-icon="inline-start" />
							)}
							Remove assignments
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</>
	)
}
