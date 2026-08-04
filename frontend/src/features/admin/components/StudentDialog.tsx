import { useState } from "react"

import { adminRequest, jsonBody } from "@/api"
import { Button } from "@/components/ui/button"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { Spinner } from "@/components/ui/spinner"
import { runMutation } from "@/features/admin/lib/page-utils"
import type { Grade, LegalSex, Student } from "@/types"

export function StudentDialog({
	student,
	grades,
	open,
	onOpenChange,
	refresh,
}: {
	student: Student | null
	grades: readonly Grade[]
	open: boolean
	onOpenChange: (open: boolean) => void
	refresh: () => Promise<void>
}): React.JSX.Element {
	const [id, setID] = useState(student === null ? "" : String(student.id))
	const [name, setName] = useState(student?.name ?? "")
	const [grade, setGrade] = useState(student?.grade ?? grades[0]?.grade ?? "")
	const [legalSex, setLegalSex] = useState<LegalSex>(
		student?.legal_sex ?? "X",
	)
	const [busy, setBusy] = useState(false)

	async function submit(
		event: React.FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault()
		setBusy(true)
		const saved = await runMutation(
			() =>
				adminRequest(
					student === null ? "/students" : `/students/${student.id}`,
					{
						method: student === null ? "POST" : "PUT",
						body: jsonBody({
							id: Number(id),
							name,
							grade,
							legal_sex: legalSex,
						}),
					},
				),
			refresh,
			student === null ? "Student created." : "Student updated.",
		)
		setBusy(false)
		if (saved) onOpenChange(false)
	}

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent>
				<form onSubmit={(event) => void submit(event)}>
					<DialogHeader>
						<DialogTitle>
							{student === null
								? "Add student"
								: `Edit ${student.name}`}
						</DialogTitle>
						<DialogDescription>
							Student IDs are used for authentication and cannot
							be changed after creation.
						</DialogDescription>
					</DialogHeader>
					<FieldGroup className="py-5">
						<Field data-disabled={student !== null}>
							<FieldLabel htmlFor="student-id">
								Student ID
							</FieldLabel>
							<Input
								id="student-id"
								type="number"
								min="1"
								value={id}
								disabled={student !== null}
								onChange={(event) => setID(event.target.value)}
								required
							/>
						</Field>
						<Field>
							<FieldLabel htmlFor="student-name">Name</FieldLabel>
							<Input
								id="student-name"
								value={name}
								onChange={(event) =>
									setName(event.target.value)
								}
								required
							/>
						</Field>
						<FieldGroup className="grid gap-4 sm:grid-cols-2">
							<Field>
								<FieldLabel htmlFor="student-grade">
									Grade
								</FieldLabel>
								<NativeSelect
									className="w-full"
									id="student-grade"
									value={grade}
									onChange={(event) =>
										setGrade(event.target.value)
									}
								>
									{grades.map((item) => (
										<NativeSelectOption
											key={item.grade}
											value={item.grade}
										>
											{item.grade}
										</NativeSelectOption>
									))}
								</NativeSelect>
							</Field>
							<Field>
								<FieldLabel htmlFor="student-sex">
									Legal sex
								</FieldLabel>
								<NativeSelect
									className="w-full"
									id="student-sex"
									value={legalSex}
									onChange={(event) =>
										setLegalSex(
											event.target.value as LegalSex,
										)
									}
								>
									<NativeSelectOption value="F">
										F
									</NativeSelectOption>
									<NativeSelectOption value="M">
										M
									</NativeSelectOption>
									<NativeSelectOption value="X">
										X
									</NativeSelectOption>
								</NativeSelect>
							</Field>
						</FieldGroup>
					</FieldGroup>
					<DialogFooter>
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
								Number(id) <= 0 ||
								name.trim() === "" ||
								grade === ""
							}
						>
							{busy ? <Spinner data-icon="inline-start" /> : null}
							Save student
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}
