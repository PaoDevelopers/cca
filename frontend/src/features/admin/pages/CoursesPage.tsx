import { PencilIcon, PlusIcon, SaveIcon } from "lucide-react"
import { useMemo, useState } from "react"

import { adminRequest, jsonBody } from "@/api"
import { PeriodBadges } from "@/components/common"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
	Card,
	CardAction,
	CardContent,
	CardDescription,
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
	FieldDescription,
	FieldGroup,
	FieldLabel,
	FieldLegend,
	FieldSet,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
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
import { Textarea } from "@/components/ui/textarea"
import type { AdminPageProps } from "@/features/admin/app/AdminApp"
import {
	DeleteButton,
	NoResults,
	PageHeading,
	SearchBox,
} from "@/features/admin/components/AdminPagePrimitives"
import {
	domID,
	getCourseSearchText,
	runMutation,
} from "@/features/admin/lib/page-utils"
import { useSearchFilter } from "@/hooks/use-search-filter"
import { CCA_DAYS, CCA_SLOTS_PER_DAY, ccaTimeSlotID } from "@/lib/cca-schedule"
import type { Course, CoursePayload, LegalSex, MembershipType } from "@/types"

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
				adminRequest(
					course === null
						? "/courses"
						: `/courses/${encodeURIComponent(course.id)}`,
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
																adminRequest(
																	`/courses/${encodeURIComponent(course.id)}`,
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
