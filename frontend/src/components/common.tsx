import { AlertCircleIcon, CalendarClockIcon } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip"
import { formatCCATimeSlotLabel } from "@/lib/cca-schedule"
import type { CourseBlockReason } from "@/types"

export function PeriodBadges({
	periodIDs,
}: {
	periodIDs: readonly string[]
}): React.JSX.Element {
	if (periodIDs.length === 0) {
		return <Badge variant="destructive">No timetable</Badge>
	}
	return (
		<div className="flex flex-wrap gap-1.5">
			{periodIDs.map((periodID) => (
				<Badge key={periodID} variant="outline">
					<CalendarClockIcon data-icon="inline-start" />
					{formatCCATimeSlotLabel(periodID)}
				</Badge>
			))}
		</div>
	)
}

export function BlockReasonBadges({
	reasons,
}: {
	reasons: readonly CourseBlockReason[]
}): React.JSX.Element {
	return (
		<div className="flex flex-wrap gap-1.5">
			{reasons.map((reason) => {
				const isClash = reason.code === "schedule_conflict"
				const key = `${reason.code}-${reason.conflicting_course_id ?? ""}`

				if (!isClash) {
					return (
						<Badge key={key} variant="destructive">
							{reason.message}
						</Badge>
					)
				}

				return (
					<Tooltip key={key}>
						<TooltipTrigger
							render={
								<Button
									type="button"
									variant="secondary"
									size="xs"
									aria-label={`Clash: ${reason.message}`}
								/>
							}
						>
							Clash
						</TooltipTrigger>
						<TooltipContent>{reason.message}</TooltipContent>
					</Tooltip>
				)
			})}
		</div>
	)
}

export function ErrorAlert({
	message,
}: {
	message: string
}): React.JSX.Element {
	return (
		<Alert variant="destructive">
			<AlertCircleIcon />
			<AlertTitle>Something went wrong</AlertTitle>
			<AlertDescription>{message}</AlertDescription>
		</Alert>
	)
}

export function PageSkeleton(): React.JSX.Element {
	return (
		<main
			id="main-content"
			className="flex min-h-svh flex-col gap-4 p-6"
			aria-busy="true"
			aria-label="Loading page"
		>
			<Skeleton className="h-8 max-w-64" />
			<div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
				{Array.from({ length: 6 }, (_, index) => (
					<Skeleton key={index} className="h-48" />
				))}
			</div>
		</main>
	)
}
