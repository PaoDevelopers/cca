// An enrollment file that would break rules, held as a draft until the
// administrator has decided every conflict. Nothing is saved until
// Upload, and then everything is saved at once.
//
// A decision is about rows: which of the file's rows to leave out.
// What is accepted follows from that — every violation whose rows all
// stay is sent back as accepted, and one whose row was left out no
// longer arises. A row can appear in more than one conflict; leaving it
// out in any of them leaves it out.
import type { EnrollmentImportCheck, ImportViolation } from "@common/adminApi"
import { Button } from "@common/components/ui/button"
import {
	Dialog,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	closeDialog,
} from "@common/components/ui/dialog"
import { Label } from "@common/components/ui/input"
import { useState, type ReactElement } from "react"

interface Props {
	check: EnrollmentImportCheck
	// Whether this student already holds the course, outside the file.
	// An import never removes a stored enrollment, so a clash with one
	// can only be settled by keeping or dropping the new row.
	holds: (studentID: string, courseID: string) => boolean
	courseLabel: (id: string) => string
	upload: (drop: string[], accept: string[]) => void
	cancel: () => void
}

interface Option {
	label: string
	drop: string[]
}

interface Conflict {
	key: string
	studentID: string
	details: string[]
	options: Option[]
}

const row = (courseID: string, studentID: string): string =>
	`${courseID}:${studentID}`

function conflictKey(v: ImportViolation): string {
	const studentID = v.student_id ?? ""
	if (v.rule === "clash" && v.other_course_id !== null) {
		const pair = [v.course_id, v.other_course_id].sort().join("+")
		return `clash:${studentID}:${pair}`
	}
	return `row:${row(v.course_id, studentID)}`
}

export function EnrollmentImportDraft({
	check,
	holds,
	courseLabel,
	upload,
	cancel,
}: Props): ReactElement {
	function optionsFor(v: ImportViolation, studentID: string): Option[] {
		const here = row(v.course_id, studentID)
		const other = v.other_course_id
		if (v.rule !== "clash" || other === null) {
			return [
				{ label: "Keep", drop: [] },
				{ label: "Drop", drop: [here] },
			]
		}
		if (holds(studentID, other)) {
			return [
				{ label: `Keep ${courseLabel(v.course_id)} anyway`, drop: [] },
				{ label: `Drop ${courseLabel(v.course_id)}`, drop: [here] },
			]
		}
		const there = row(other, studentID)
		return [
			{ label: `Keep ${courseLabel(v.course_id)}`, drop: [there] },
			{ label: `Keep ${courseLabel(other)}`, drop: [here] },
			{ label: "Drop both", drop: [here, there] },
		]
	}

	// One conflict per clashing pair of courses, however many periods
	// they share, and one per row for every other rule.
	const byKey = new Map<string, Conflict>()
	for (const v of check.violations) {
		const studentID = v.student_id ?? ""
		const key = conflictKey(v)
		const conflict = byKey.get(key)
		if (conflict !== undefined) {
			conflict.details.push(v.detail)
			continue
		}
		byKey.set(key, {
			key,
			studentID,
			details: [v.detail],
			options: optionsFor(v, studentID),
		})
	}
	const conflicts = [...byKey.values()].sort((a, b): number =>
		a.studentID.localeCompare(b.studentID),
	)

	// Chosen option index per conflict key.
	const [choices, setChoices] = useState<Record<string, number>>({})

	const decided = conflicts.filter(
		(c): boolean => choices[c.key] !== undefined,
	).length

	function dropAll(): void {
		const next: Record<string, number> = {}
		for (const c of conflicts) {
			// The last option always drops every row in the conflict.
			next[c.key] = c.options.length - 1
		}
		setChoices(next)
	}

	function submit(): void {
		const drop = new Set(
			check.unknown.map((u): string => row(u.course_id, u.student_id)),
		)
		for (const c of conflicts) {
			const chosen = c.options[choices[c.key] ?? -1]
			for (const d of chosen?.drop ?? []) {
				drop.add(d)
			}
		}
		const accept = check.violations
			.filter((v): boolean => {
				const studentID = v.student_id ?? ""
				const rows = [row(v.course_id, studentID)]
				if (v.other_course_id !== null) {
					rows.push(row(v.other_course_id, studentID))
				}
				return rows.every((r): boolean => !drop.has(r))
			})
			.map((v): string => v.code)
		upload([...drop], accept)
	}

	return (
		<Dialog
			className="max-w-3xl"
			aria-labelledby="import-draft-title"
			onClose={(value): void => {
				if (value === "upload") {
					submit()
				} else {
					cancel()
				}
			}}
		>
			<DialogHeader>
				<DialogTitle id="import-draft-title">
					{conflicts.length} conflict
					{conflicts.length === 1 ? "" : "s"} in this file
				</DialogTitle>
				<DialogDescription>
					Choose what to do with each one. Rows without a conflict are
					uploaded as they are. Nothing has been saved yet.
				</DialogDescription>
			</DialogHeader>

			{check.unknown.length > 0 && (
				<section className="mb-4">
					<h3 className="mb-1 text-sm font-medium">
						Left out: student or course does not exist
					</h3>
					<ul className="list-disc pl-5 text-sm">
						{check.unknown.map((u) => (
							<li key={row(u.course_id, u.student_id)}>
								<strong>{u.student_id}</strong> in{" "}
								{courseLabel(u.course_id)}
							</li>
						))}
					</ul>
				</section>
			)}

			{conflicts.length > 0 && (
				<div className="mb-2 flex justify-end">
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={dropAll}
					>
						Drop all conflicting rows
					</Button>
				</div>
			)}

			<ul className="flex flex-col gap-2">
				{conflicts.map((c) => (
					<li key={c.key}>
						<fieldset className="rounded-md border p-3">
							<legend className="px-1">
								<strong>{c.studentID}</strong>
							</legend>
							<ul className="mb-2 list-disc pl-5 text-sm">
								{c.details.map((detail, i) => (
									<li key={i}>{detail}</li>
								))}
							</ul>
							<div className="flex flex-wrap gap-x-4 gap-y-2">
								{c.options.map((option, i) => (
									<Label key={i} className="font-normal">
										<input
											type="radio"
											className="accent-primary"
											name={c.key}
											checked={choices[c.key] === i}
											onChange={(): void => {
												setChoices({
													...choices,
													[c.key]: i,
												})
											}}
										/>
										{option.label}
									</Label>
								))}
							</div>
						</fieldset>
					</li>
				))}
			</ul>

			<DialogFooter className="sticky -bottom-6 items-center bg-background py-3">
				<span className="mr-auto text-sm text-muted-foreground">
					{decided} of {conflicts.length} decided
				</span>
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
				<Button
					type="button"
					size="default"
					disabled={decided < conflicts.length}
					onClick={(event): void => {
						closeDialog(event.currentTarget, "upload")
					}}
				>
					Upload
				</Button>
			</DialogFooter>
		</Dialog>
	)
}
