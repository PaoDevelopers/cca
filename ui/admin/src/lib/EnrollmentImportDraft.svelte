<script lang="ts">
	// An enrollment file that would break rules, held as a draft until
	// the administrator has decided every conflict. Nothing is saved
	// until Upload, and then everything is saved at once.
	//
	// A decision is about rows: which of the file's rows to leave out.
	// What is accepted follows from that — every violation whose rows
	// all stay is sent back as accepted, and one whose row was left out
	// no longer arises. A row can appear in more than one conflict;
	// leaving it out in any of them leaves it out.
	import type {
		EnrollmentImportCheck,
		ImportViolation,
	} from "@common/adminApi"

	interface Props {
		check: EnrollmentImportCheck
		// Whether this student already holds the course, outside the
		// file. An import never removes a stored enrollment, so a clash
		// with one can only be settled by keeping or dropping the new row.
		holds: (studentID: string, courseID: string) => boolean
		courseLabel: (id: string) => string
		upload: (drop: string[], accept: string[]) => void
		cancel: () => void
	}

	const { check, holds, courseLabel, upload, cancel }: Props = $props()

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

	// One conflict per clashing pair of courses, however many periods
	// they share, and one per row for every other rule.
	const conflicts = $derived.by((): Conflict[] => {
		const byKey = new Map<string, Conflict>()
		for (const v of check.violations) {
			const studentID = v.student_id ?? ""
			const conflict = byKey.get(conflictKey(v))
			if (conflict !== undefined) {
				conflict.details.push(v.detail)
				continue
			}
			byKey.set(conflictKey(v), {
				key: conflictKey(v),
				studentID,
				details: [v.detail],
				options: optionsFor(v, studentID),
			})
		}
		return [...byKey.values()].sort((a, b): number =>
			a.studentID.localeCompare(b.studentID),
		)
	})

	function conflictKey(v: ImportViolation): string {
		const studentID = v.student_id ?? ""
		if (v.rule === "clash" && v.other_course_id !== null) {
			const pair = [v.course_id, v.other_course_id].sort().join("+")
			return `clash:${studentID}:${pair}`
		}
		return `row:${row(v.course_id, studentID)}`
	}

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

	// Chosen option index per conflict key.
	let choices = $state<Record<string, number>>({})

	const decided = $derived(
		conflicts.filter((c): boolean => choices[c.key] !== undefined).length,
	)

	function dropAll(): void {
		const next: Record<string, number> = {}
		for (const c of conflicts) {
			// The last option always drops every row in the conflict.
			next[c.key] = c.options.length - 1
		}
		choices = next
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
		dialog?.close("upload")
		upload([...drop], accept)
	}

	let dialog = $state<HTMLDialogElement | null>(null)

	$effect((): void => {
		dialog?.showModal()
	})
</script>

<dialog
	bind:this={dialog}
	aria-labelledby="import-draft-title"
	onclose={(): void => {
		if (dialog?.returnValue !== "upload") {
			cancel()
		}
	}}
>
	<h2 id="import-draft-title">
		{conflicts.length} conflict{conflicts.length === 1 ? "" : "s"} in this file
	</h2>
	<p class="note">
		Choose what to do with each one. Rows without a conflict are uploaded as
		they are. Nothing has been saved yet.
	</p>

	{#if check.unknown.length > 0}
		<h3>Left out: student or course does not exist</h3>
		<ul>
			{#each check.unknown as u (row(u.course_id, u.student_id))}
				<li>
					<strong>{u.student_id}</strong> in {courseLabel(
						u.course_id,
					)}
				</li>
			{/each}
		</ul>
	{/if}

	{#if conflicts.length > 0}
		<div class="bulk">
			<button type="button" onclick={dropAll}
				>Drop all conflicting rows</button
			>
		</div>
	{/if}

	<ul class="conflicts">
		{#each conflicts as c (c.key)}
			<li>
				<fieldset>
					<legend><strong>{c.studentID}</strong></legend>
					<ul>
						{#each c.details as detail, i (i)}
							<li>{detail}</li>
						{/each}
					</ul>
					<div class="options">
						{#each c.options as option, i (i)}
							<label>
								<input
									type="radio"
									name={c.key}
									checked={choices[c.key] === i}
									onchange={(): void => {
										choices = { ...choices, [c.key]: i }
									}}
								/>
								{option.label}
							</label>
						{/each}
					</div>
				</fieldset>
			</li>
		{/each}
	</ul>

	<div class="actions">
		<span class="note">{decided} of {conflicts.length} decided</span>
		<button
			type="button"
			onclick={(): void => {
				dialog?.close("cancel")
			}}>Cancel</button
		>
		<button
			type="button"
			disabled={decided < conflicts.length}
			onclick={submit}>Upload</button
		>
	</div>
</dialog>

<style>
	dialog {
		width: 48rem;
		max-width: calc(100vw - 2rem);
		max-height: 85vh;
		overflow-y: auto;
		padding: 1rem;
		border: 1px solid color-mix(in oklab, var(--fg) 30%, var(--bg));
		border-radius: 0.25rem;
		background: var(--bg);
		color: var(--fg);
	}

	dialog::backdrop {
		background: color-mix(in oklab, black 50%, transparent);
	}

	h2 {
		margin-top: 0;
		font-size: 1rem;
	}

	h3 {
		font-size: 0.875rem;
	}

	ul {
		padding-left: 1.25rem;
	}

	.conflicts {
		list-style: none;
		padding-left: 0;

		> li + li {
			margin-top: 0.5rem;
		}
	}

	fieldset {
		border: 1px solid color-mix(in oklab, var(--fg) 20%, var(--bg));
		border-radius: 0.25rem;
	}

	.options {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem 1rem;
	}

	.note {
		font-size: 0.875rem;
		color: color-mix(in oklab, var(--fg) 70%, var(--bg));
	}

	.bulk,
	.actions {
		display: flex;
		gap: 0.5rem;
		justify-content: flex-end;
		align-items: baseline;
	}

	.actions {
		position: sticky;
		bottom: -1rem;
		padding: 0.5rem 0;
		background: var(--bg);
	}
</style>
