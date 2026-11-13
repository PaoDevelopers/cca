// An enrollment file that would break rules, held open until the
// administrator decides. Upload places everything decided and sets
// every undecided conflict aside; closing places nothing and sets the
// whole file aside. Either way what is set aside waits as a draft on
// the Enrollments page, so nothing is lost by walking away.
//
// A decision is about rows: which of the file's rows to leave out.
// What is accepted follows from that — every violation whose rows all
// stay is sent back as accepted, and one whose row was left out no
// longer arises. A row can appear in more than one conflict; leaving it
// out in any of them leaves it out.
//
// A conflict left without a choice is decided later: its rows are left
// out for now, the server keeps them, the Enrollments page offers to
// resume them, and resuming brings them back through this same dialog.
// Meanwhile the student's address is copied from here to email them.
import type { EnrollmentImportCheck, ImportViolation } from "@common/adminApi"
import { Button } from "@common/components/ui/button"
import {
	Dialog,
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
	copyAddresses: (studentIDs: string[]) => void
	upload: (drop: string[], accept: string[], later: string[]) => void
	// Closed without uploading, by the button, Escape or the backdrop:
	// the whole file is kept as a draft.
	hold: () => void
}

interface Option {
	label: string
	drop: string[]
}

interface Conflict {
	key: string
	studentID: string
	violations: ImportViolation[]
	options: Option[]
}

export const row = (courseID: string, studentID: string): string =>
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
	copyAddresses,
	upload,
	hold,
}: Props): ReactElement {
	// Drop stays last in every list: "Drop all" relies on it.
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
		// Keeping both is a real choice, not an oversight to be fixed
		// later: a student on two teams that share a slot is placed in
		// both, and the coaches split the sessions between them.
		return [
			{ label: `Keep ${courseLabel(v.course_id)}`, drop: [there] },
			{ label: `Keep ${courseLabel(other)}`, drop: [here] },
			{ label: "Keep both anyway", drop: [] },
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
			conflict.violations.push(v)
			continue
		}
		byKey.set(key, {
			key,
			studentID,
			violations: [v],
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
		const later = new Set<string>()
		for (const c of conflicts) {
			const chosen = choices[c.key]
			if (chosen === undefined) {
				// Nobody chose: it waits, rather than the file being
				// refused or a guess being made for the student. Every
				// option's rows are this conflict's rows, and the last
				// option, Drop, names them all.
				for (const r of c.options.at(-1)?.drop ?? []) {
					later.add(r)
				}
				continue
			}
			for (const d of c.options[chosen]?.drop ?? []) {
				drop.add(d)
			}
		}
		// Dropped in one conflict and set aside in another is dropped:
		// there is nothing left to decide about a row already let go.
		for (const d of drop) {
			later.delete(d)
		}
		const accept = check.violations
			.filter((v): boolean => {
				const studentID = v.student_id ?? ""
				const rows = [row(v.course_id, studentID)]
				if (v.other_course_id !== null) {
					rows.push(row(v.other_course_id, studentID))
				}
				return rows.every((r): boolean => !drop.has(r) && !later.has(r))
			})
			.map((v): string => v.code)
		upload([...drop], accept, [...later])
	}

	return (
		<Dialog
			// Important: cn joins rather than merges, so the dialog's own
			// max-w-lg would otherwise win.
			className="max-w-6xl!"
			aria-labelledby="import-draft-title"
			onClose={(value): void => {
				if (value === "upload") {
					submit()
				} else {
					hold()
				}
			}}
		>
			<DialogHeader>
				<div className="flex flex-wrap items-center justify-between gap-2">
					<DialogTitle id="import-draft-title">
						{conflicts.length} conflict
						{conflicts.length === 1 ? "" : "s"} in this file
					</DialogTitle>
					{conflicts.length > 0 && (
						<div className="flex flex-wrap gap-2">
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={(): void => {
									copyAddresses(
										conflicts.map(
											(c): string => c.studentID,
										),
									)
								}}
							>
								Copy all their email addresses
							</Button>
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
				</div>
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

			<ul className="flex flex-col gap-2">
				{conflicts.map((c) => (
					<li key={c.key}>
						<fieldset className="rounded-md border p-3">
							<legend className="px-1">
								<strong>{c.studentID}</strong>
							</legend>
							<div className="mb-2 flex flex-wrap items-start justify-between gap-2">
								<ul className="list-disc pl-5 text-sm">
									{c.violations.map((v, i) => (
										<li key={i}>{v.detail}</li>
									))}
								</ul>
								<Button
									type="button"
									variant="outline"
									size="sm"
									aria-label={`Copy email of ${c.studentID}`}
									onClick={(): void => {
										copyAddresses([c.studentID])
									}}
								>
									Copy email
								</Button>
							</div>
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

			<DialogFooter className="sticky -bottom-6 flex-wrap items-center bg-background py-3">
				<span className="mr-auto basis-full text-sm text-muted-foreground sm:basis-auto">
					{decided} of {conflicts.length} decided
					{decided < conflicts.length &&
						`; ${String(conflicts.length - decided)} will wait as a draft`}
				</span>
				<Button
					type="button"
					variant="outline"
					size="default"
					onClick={(event): void => {
						closeDialog(event.currentTarget, "cancel")
					}}
				>
					Keep all as draft
				</Button>
				<Button
					type="button"
					size="default"
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
