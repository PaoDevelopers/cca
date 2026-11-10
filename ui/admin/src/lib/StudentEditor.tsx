// The Sheet that adds a student or edits one. Mounted only while it is
// open, so it can own its draft outright.
import type { AdminStudent } from "@common/adminApi"
import { Button } from "@common/components/ui/button"
import {
	Dialog,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	closeDialog,
} from "@common/components/ui/dialog"
import { Input, Label, NativeSelect } from "@common/components/ui/input"
import type { LegalSex } from "@common/types"
import { useState, type ReactElement } from "react"

interface Props {
	// null to add a new student.
	student: AdminStudent | null
	gradeIDs: string[]
	sexes: LegalSex[]
	// Standing, shown read-only while editing.
	budget: string
	requirements: string
	busy: boolean
	onsave: (draft: AdminStudent) => void
	onclose: () => void
}

export function StudentEditor({
	student,
	gradeIDs,
	sexes,
	budget,
	requirements,
	busy,
	onsave,
	onclose,
}: Props): ReactElement {
	// The editor deliberately snapshots the student it was opened for:
	// it is mounted per edit, so a refetch mid-edit does not discard
	// what is being typed.
	const [id, setID] = useState(student?.id ?? "")
	const [name, setName] = useState(student?.name ?? "")
	const [gradeID, setGradeID] = useState(student?.grade_id ?? "")
	const [legalSex, setLegalSex] = useState<LegalSex>(
		student?.legal_sex ?? "F",
	)

	return (
		<Dialog
			side="right"
			aria-labelledby="student-editor-title"
			onClose={onclose}
		>
			<form
				className="flex flex-col gap-4"
				onSubmit={(event): void => {
					event.preventDefault()
					const trimmed = id.trim()
					if (trimmed === "" || gradeID === "") {
						return
					}
					onsave({
						id: trimmed,
						name: name.trim(),
						grade_id: gradeID,
						legal_sex: legalSex,
					})
				}}
			>
				<DialogHeader>
					<DialogTitle id="student-editor-title">
						{student === null
							? "Add student"
							: `Edit ${student.id}`}
					</DialogTitle>
					{student !== null && (
						<DialogDescription>
							Budget {budget === "" ? "—" : budget} · Requirements{" "}
							{requirements}
						</DialogDescription>
					)}
				</DialogHeader>

				<Label className="flex-col items-start">
					Student ID
					<Input
						type="text"
						value={id}
						placeholder="s22537"
						required
						// The id is the key; changing it is a different
						// student, which is an add.
						readOnly={student !== null}
						onChange={(event): void => {
							setID(event.currentTarget.value)
						}}
					/>
				</Label>
				<Label className="flex-col items-start">
					Name
					<Input
						type="text"
						value={name}
						required
						onChange={(event): void => {
							setName(event.currentTarget.value)
						}}
					/>
				</Label>
				<Label className="flex-col items-start">
					Grade
					<NativeSelect
						value={gradeID}
						required
						onChange={(event): void => {
							setGradeID(event.currentTarget.value)
						}}
					>
						<option value="" disabled>
							Choose
						</option>
						{gradeIDs.map((option) => (
							<option key={option} value={option}>
								{option}
							</option>
						))}
					</NativeSelect>
				</Label>
				<Label className="flex-col items-start">
					Legal sex
					<NativeSelect
						value={legalSex}
						onChange={(event): void => {
							setLegalSex(event.currentTarget.value as LegalSex)
						}}
					>
						{sexes.map((sex) => (
							<option key={sex} value={sex}>
								{sex}
							</option>
						))}
					</NativeSelect>
				</Label>

				<DialogFooter>
					<Button
						type="button"
						variant="outline"
						size="default"
						aria-label={
							student === null
								? "Cancel adding a student"
								: `Cancel editing ${student.id}`
						}
						onClick={(event): void => {
							closeDialog(event.currentTarget, "cancel")
						}}
					>
						Cancel
					</Button>
					<Button
						size="default"
						disabled={busy}
						aria-label={
							student === null ? "Add" : `Save ${student.id}`
						}
					>
						{student === null ? "Add" : "Save"}
					</Button>
				</DialogFooter>
			</form>
		</Dialog>
	)
}
