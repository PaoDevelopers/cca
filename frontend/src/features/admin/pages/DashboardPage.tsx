import { BookOpenIcon, UsersIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "@/components/ui/card"
import {
	Item,
	ItemActions,
	ItemContent,
	ItemGroup,
	ItemTitle,
} from "@/components/ui/item"
import {
	NoResults,
	PageHeading,
	StatCard,
} from "@/features/admin/components/AdminPagePrimitives"
import type { AdminDashboard } from "@/types"

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
