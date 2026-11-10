import type { CourseInput } from "@common/adminApi"
import { Button } from "@common/components/ui/button"
import { closeDialog } from "@common/components/ui/dialog"
import {
	Checkbox,
	Input,
	Label,
	NativeSelect,
	Textarea,
} from "@common/components/ui/input"
import {
	courseTermSuggestions,
	type Course,
	type LegalSex,
} from "@common/types"
import { useRef, useState, type ReactElement, type ReactNode } from "react"

interface Props {
	// null means creating a new course.
	initial: Course | null
	categories: string[]
	periods: string[]
	gradeIDs: string[]
	busy: boolean
	// Resolves to whether the write took effect; the editor closes the
	// sheet it sits in only then, so a refused save keeps the draft.
	onsave: (input: CourseInput) => Promise<boolean>
}

const sexes: LegalSex[] = ["F", "M", "X"]

function toggled<T>(list: T[], value: T, on: boolean): T[] {
	return on ? [...list, value] : list.filter((v): boolean => v !== value)
}

// One checkbox per option, writing a list.
function Picks<T extends string>({
	legend,
	options,
	value,
	onChange,
	empty,
}: {
	legend: string
	options: T[]
	value: T[]
	onChange: (value: T[]) => void
	empty?: string
}): ReactElement {
	return (
		<fieldset className="flex flex-wrap gap-x-4 gap-y-2 rounded-md border p-3">
			<legend className="px-1 text-sm font-medium">{legend}</legend>
			{options.length === 0 && (
				<span className="text-sm text-muted-foreground">{empty}</span>
			)}
			{options.map((option) => (
				<Label key={option} className="font-normal">
					<Checkbox
						value={option}
						checked={value.includes(option)}
						onChange={(event): void => {
							onChange(
								toggled(
									value,
									option,
									event.currentTarget.checked,
								),
							)
						}}
					/>
					{option}
				</Label>
			))}
		</fieldset>
	)
}

function Field({
	label,
	children,
	className,
}: {
	label: string
	children: ReactNode
	className?: string
}): ReactElement {
	return (
		<Label className={`flex-col items-start gap-1.5 ${className ?? ""}`}>
			{label}
			{children}
		</Label>
	)
}

export function CourseEditor({
	initial,
	categories,
	periods,
	gradeIDs,
	busy,
	onsave,
}: Props): ReactElement {
	// The editor deliberately snapshots the course it was opened for:
	// the page mounts it per target, so later prop changes are unwanted.
	const [id, setID] = useState(initial?.id ?? "")
	const [name, setName] = useState(initial?.name ?? "")
	const [description, setDescription] = useState(initial?.description ?? "")
	const [teacher, setTeacher] = useState(initial?.teacher ?? "")
	const [teacherEmail, setTeacherEmail] = useState(
		initial?.teacher_email ?? "",
	)
	const [location, setLocation] = useState(initial?.location ?? "")
	const [term, setTerm] = useState(initial?.term ?? "Season")
	const [cost, setCost] = useState(initial?.cost ?? "")
	const [categoryID, setCategoryID] = useState(initial?.category_id ?? "")
	const [inviteOnly, setInviteOnly] = useState(initial?.invite_only ?? false)
	// The empty box is a real setting — no cap — so the draft is the
	// text, not a number: coercing it would turn "" into 0, which is the
	// opposite setting and admits nobody.
	const baseMax = initial?.max_students ?? null
	const [maxStudents, setMaxStudents] = useState(
		baseMax === null ? "" : String(baseMax),
	)
	const [selectedPeriods, setSelectedPeriods] = useState<string[]>([
		...(initial?.period_ids ?? []),
	])
	const [allowedSexes, setAllowedSexes] = useState<LegalSex[]>([
		...(initial?.allowed_legal_sexes ?? []),
	])
	const [allowedGrades, setAllowedGrades] = useState<string[]>([
		...(initial?.allowed_grade_ids ?? []),
	])
	const form = useRef<HTMLFormElement>(null)

	return (
		<form
			ref={form}
			className="editor flex flex-col gap-4"
			onSubmit={(event): void => {
				event.preventDefault()
				// An empty box, and the word the import takes for it, both
				// mean no cap.
				const raw = maxStudents.trim().toLowerCase()
				const max =
					raw === "" || raw === "unlimited"
						? null
						: Number.parseInt(raw, 10)
				if (max !== null && (Number.isNaN(max) || max < 0)) {
					return
				}
				// The accept list is filled in by the caller if the server
				// comes back with violations; a first attempt always
				// accepts nothing.
				void onsave({
					id: id.trim(),
					name: name.trim(),
					description: description.trim(),
					category_id: categoryID,
					teacher: teacher.trim(),
					teacher_email: teacherEmail.trim(),
					location: location.trim(),
					term: term.trim(),
					cost: cost.trim(),
					max_students: max,
					invite_only: inviteOnly,
					period_ids: selectedPeriods,
					allowed_legal_sexes: allowedSexes,
					allowed_grade_ids: allowedGrades,
					accept: [],
				}).then((saved): void => {
					if (saved && form.current !== null) {
						closeDialog(form.current, "saved")
					}
				})
			}}
		>
			<div className="grid gap-4 sm:grid-cols-2">
				<Field label="ID">
					<Input
						type="text"
						value={id}
						onChange={(event): void => {
							setID(event.currentTarget.value)
						}}
						disabled={initial !== null}
						required
					/>
				</Field>
				<Field label="Name">
					<Input
						type="text"
						value={name}
						onChange={(event): void => {
							setName(event.currentTarget.value)
						}}
						required
					/>
				</Field>
				<Field label="Teacher">
					<Input
						type="text"
						value={teacher}
						onChange={(event): void => {
							setTeacher(event.currentTarget.value)
						}}
						required
					/>
				</Field>
				<Field label="Teacher e-mail">
					<Input
						type="email"
						value={teacherEmail}
						onChange={(event): void => {
							setTeacherEmail(event.currentTarget.value)
						}}
					/>
				</Field>
				<Field label="Location">
					<Input
						type="text"
						value={location}
						onChange={(event): void => {
							setLocation(event.currentTarget.value)
						}}
						required
					/>
				</Field>
				{/*
					Free text with suggestions, not a fixed set, and not
					required: the software never acts on a term's value, so
					constraining it here would only stop the department
					writing what their spreadsheet says — or leaving it
					blank, which is what a department that does not divide
					its season into terms writes.
				*/}
				<Field label="Term">
					<Input
						type="text"
						value={term}
						list="course-terms"
						onChange={(event): void => {
							setTerm(event.currentTarget.value)
						}}
					/>
					<datalist id="course-terms">
						{courseTermSuggestions.map((option) => (
							<option key={option} value={option} />
						))}
					</datalist>
				</Field>
				<Field label="Cost">
					<Input
						type="text"
						value={cost}
						onChange={(event): void => {
							setCost(event.currentTarget.value)
						}}
					/>
				</Field>
				<Field label="Category">
					<NativeSelect
						className="w-full"
						value={categoryID}
						onChange={(event): void => {
							setCategoryID(event.currentTarget.value)
						}}
						required
					>
						<option value="" disabled>
							Choose
						</option>
						{categories.map((category) => (
							<option key={category} value={category}>
								{category}
							</option>
						))}
					</NativeSelect>
				</Field>
				{/*
					Left empty, the course takes everyone. Not a number
					input: an empty one cannot be told from one holding
					nothing yet, and the word "unlimited", which the import
					accepts, could not be typed into it at all.
				*/}
				<Field label="Capacity">
					<Input
						type="text"
						inputMode="numeric"
						placeholder="unlimited"
						aria-label="Capacity, empty for no limit"
						value={maxStudents}
						onChange={(event): void => {
							setMaxStudents(event.currentTarget.value)
						}}
					/>
				</Field>
				<Label className="self-end pb-2.5">
					<Checkbox
						checked={inviteOnly}
						onChange={(event): void => {
							setInviteOnly(event.currentTarget.checked)
						}}
					/>
					Invite only
				</Label>
			</div>

			<Field label="Description">
				<Textarea
					rows={3}
					value={description}
					onChange={(event): void => {
						setDescription(event.currentTarget.value)
					}}
				/>
			</Field>

			<Picks
				legend="Periods"
				options={periods}
				value={selectedPeriods}
				onChange={setSelectedPeriods}
				empty="No periods defined."
			/>
			<Picks
				legend="Allowed legal sexes (none checked = all)"
				options={sexes}
				value={allowedSexes}
				onChange={setAllowedSexes}
			/>
			<Picks
				legend="Allowed grades (none checked = all)"
				options={gradeIDs}
				value={allowedGrades}
				onChange={setAllowedGrades}
				empty="No grades defined."
			/>

			{/*
				No override checkbox. Saving an edit that would break a rule
				comes back with exactly what it would break, and is
				confirmed then — so an administrator agrees to specific
				consequences rather than switching off the checks in
				advance.
			*/}
			<div className="flex justify-end gap-2">
				<Button
					type="button"
					variant="outline"
					size="default"
					onClick={(event): void => {
						closeDialog(event.currentTarget, "cancel")
					}}
				>
					Cancel
				</Button>
				<Button size="default" disabled={busy}>
					{initial === null ? "Create" : "Save"}
				</Button>
			</div>
		</form>
	)
}
