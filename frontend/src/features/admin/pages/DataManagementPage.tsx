import { useState } from "react"
import { toast } from "sonner"

import { adminRequest, jsonBody } from "@/api"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
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
import {
	Combobox,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
} from "@/components/ui/combobox"
import {
	Field,
	FieldDescription,
	FieldGroup,
	FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { AdminPageProps } from "@/features/admin/app/AdminApp"
import { PageHeading } from "@/features/admin/components/AdminPagePrimitives"
import { domID } from "@/features/admin/lib/page-utils"

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
					Import {format.value === "csv" ? "CSV" : "Excel"}
				</Button>
				<Button
					variant="link"
					render={<a href={`${example}?format=${format.value}`} />}
					nativeButton={false}
				>
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
			const result = await adminRequest<AdminResetResult>("/reset", {
				method: "POST",
				body: jsonBody({
					scope: definition.scope,
					confirmation,
				}),
			})
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
						{definition.title}
					</AlertDialogTrigger>
					<AlertDialogContent>
						<AlertDialogHeader>
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
								) : null}
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
