<script lang="ts">
	import {
		closeWindowNow,
		deleteGrade,
		openWindowNow,
		setGradeBudget,
		setGradeRequirements,
		setGradeWindow,
		updateGrade,
	} from "@common/adminApi"
	import type { Category, Grade } from "@common/types"
	import { formatInstant, fromLocalInput, toLocalInput } from "./datetime"

	interface Props {
		grade: Grade
		categories: Category[]
		busy: boolean
		run: (action: () => Promise<void>) => Promise<boolean>
		// For the budget cap, which is the one field here that can
		// break an existing enrollment.
		runAccepting: (
			action: (accept: string[]) => Promise<void>,
		) => Promise<boolean>
		move: (direction: "up" | "down") => void
	}

	const { grade, categories, busy, run, runAccepting, move }: Props = $props()

	// The card is labelled by the grade name, which is not the
	// article's first child.
	const uid = $props.id()

	let confirmingDelete = $state(false)

	// Drafts are seeded once and then belong to whoever is typing:
	// re-deriving them from the prop would discard a half-typed value
	// every time the list is refetched, which happens after any grade
	// write anywhere — including another card on this page.
	// svelte-ignore state_referenced_locally
	let nameDraft = $state(grade.name)

	// And follows the server while nobody is typing in it, like the
	// window boxes below. Seeded once and never re-read, the box went
	// on showing a name somebody else had changed for as long as the
	// page stayed open — which for this card is the whole session,
	// because GradesPage keys it by grade id.
	let nameDirty = $state(false)

	$effect((): void => {
		const fresh = grade.name
		if (!nameDirty) {
			nameDraft = fresh
		}
	})
	// The two numeric settings are read from the event rather than
	// bound: Svelte coerces bind:value on a number input to a number,
	// and an empty box then becomes null rather than "", which is the
	// difference between "no cap" and "the box is blank" — and the
	// difference matters here, because no cap is a real setting.
	const budgetValue = $derived(
		grade.max_budgeted_periods === null
			? ""
			: String(grade.max_budgeted_periods),
	)
	// The window is two instants, and each box owns one of them.
	//
	// Both used to be sent on every change, which quietly reverted the
	// other administrator's work: the drafts are seeded once and never
	// re-read, so a card that had been open while somebody else moved
	// the opening time still held the old one, and editing the closing
	// time wrote it back. Nobody saw it happen — the write succeeded,
	// and the value it restored was a value that had genuinely been
	// there.
	//
	// So each write sends only the boxes this administrator actually
	// edited, and leaves the other key out of the request entirely.
	// Omission is what makes it safe: an absent bound is one the
	// database does not touch, so a box nobody edited cannot carry the
	// value it was built with into a write. Sending "the server's
	// current value for the other" would be the bug again, one read
	// earlier.
	// svelte-ignore state_referenced_locally
	let opensDraft = $state(toLocalInput(grade.opens_at))
	// svelte-ignore state_referenced_locally
	let closesDraft = $state(toLocalInput(grade.closes_at))

	// And a box nobody is editing follows the server. Dirty until the
	// write lands, so a half-set date is not yanked away mid-edit by a
	// refetch some other card triggered.
	let opensDirty = $state(false)
	let closesDirty = $state(false)

	$effect((): void => {
		const fresh = toLocalInput(grade.opens_at)
		if (!opensDirty) {
			opensDraft = fresh
		}
	})

	$effect((): void => {
		const fresh = toLocalInput(grade.closes_at)
		if (!closesDirty) {
			closesDraft = fresh
		}
	})

	// Spell out the nullable-bound semantics. A dash beside an empty box
	// made the two very different kinds of absence look the same: no
	// opening keeps the window closed, while no closing lets an open
	// window continue indefinitely.
	const windowSummary = $derived.by((): string => {
		if (grade.is_open) {
			return grade.closes_at === null
				? `Open since ${formatInstant(grade.opens_at)}; no automatic closing time.`
				: `Open since ${formatInstant(grade.opens_at)}; closes ${formatInstant(grade.closes_at)}.`
		}
		if (grade.opens_at === null) {
			return grade.closes_at === null
				? "Closed; no opening or closing time is set."
				: `Closed; no opening time is set. The saved closing time is ${formatInstant(grade.closes_at)}.`
		}
		return grade.closes_at === null
			? `Closed; opens ${formatInstant(grade.opens_at)} and then stays open.`
			: `Closed; scheduled ${formatInstant(grade.opens_at)} to ${formatInstant(grade.closes_at)}.`
	})

	// The requirement set is one form and one save, so it is edited as
	// a whole here and sent as a whole. Nothing is written until Save.
	// The count is held as text for the same reason the budget above
	// is: bind:value on a number input yields null for an empty box,
	// and zero is a meaningful requirement here — permanently met — so
	// an empty box that became zero would quietly tell every student
	// they had satisfied something they had not.
	interface RequirementDraft {
		count: string
		category_ids: string[]
	}

	function draftsOf(of: Grade): RequirementDraft[] {
		return of.requirements.map((r) => ({
			count: String(r.min_period_count),
			category_ids: [...r.category_ids],
		}))
	}

	// svelte-ignore state_referenced_locally
	let requirementDrafts = $state<RequirementDraft[]>(draftsOf(grade))

	let requirementError = $state<string | null>(null)

	// The requirement set is written declaratively — the whole list
	// replaces the whole list — so a card built from a stale copy of it
	// deletes whatever has been added since. This card lives for the
	// whole session (GradesPage keys it by grade id), so "since" can be
	// days: administrator A opens the page on Monday, B adds a
	// requirement on Tuesday, A adjusts a period count on Friday and
	// writes back Monday's list, and B's requirement is gone with no
	// error and nothing to see. It is the bug the window bounds above
	// were fixed for, in the one shape that cannot be fixed by leaving
	// a field out: there is no half of this list to omit.
	//
	// So the same rule instead. Untouched, the drafts follow the
	// server. Touched, they are the administrator's and are left alone
	// — and if the server's list moves underneath them while they are
	// touched, the write is refused rather than allowed to silently
	// undo it.
	let requirementsDirty = $state(false)
	// svelte-ignore state_referenced_locally
	let requirementsBasis = $state(JSON.stringify(draftsOf(grade)))
	const requirementsOnServer = $derived(JSON.stringify(draftsOf(grade)))
	const requirementsMovedElsewhere = $derived(
		requirementsDirty && requirementsOnServer !== requirementsBasis,
	)

	$effect((): void => {
		const fresh = requirementsOnServer
		if (!requirementsDirty) {
			requirementDrafts = JSON.parse(fresh) as RequirementDraft[]
			requirementsBasis = fresh
		}
	})

	function touchRequirements(): void {
		requirementsDirty = true
	}

	interface Requirement {
		min_period_count: number
		category_ids: string[]
	}

	// What the drafts mean, or the first reason they mean nothing.
	function readRequirements():
		{ ok: true; value: Requirement[] } | { ok: false; why: string } {
		const value: Requirement[] = []
		for (const [i, draft] of requirementDrafts.entries()) {
			if (draft.category_ids.length === 0) {
				return {
					ok: false,
					why: `Requirement ${String(i + 1)} names no category.`,
				}
			}
			const count = Number.parseInt(draft.count.trim(), 10)
			if (!Number.isInteger(count) || count < 0) {
				return {
					ok: false,
					why: `Requirement ${String(i + 1)} needs a whole number of periods.`,
				}
			}
			value.push({
				min_period_count: count,
				category_ids: draft.category_ids,
			})
		}
		return { ok: true, value }
	}

	// Sends only the boxes this administrator touched. A bound left
	// out is left alone, so the value the other box was built with —
	// which may be older than what the grade now has — never travels.
	function saveWindow(): void {
		const bounds: { opens_at?: string | null; closes_at?: string | null } =
			{}
		if (opensDirty) {
			bounds.opens_at = fromLocalInput(opensDraft)
		}
		if (closesDirty) {
			bounds.closes_at = fromLocalInput(closesDraft)
		}
		if (bounds.opens_at === undefined && bounds.closes_at === undefined) {
			return
		}
		void run(async (): Promise<void> => {
			await setGradeWindow(grade.id, bounds)
			opensDirty = false
			closesDirty = false
		})
	}

	// The manual levers, which the scheduled bounds subsume.
	//
	// They do not go through the boxes. A draft is an instant this
	// administrator chose and is theirs until it is written; "now" is
	// not chosen, it is read, and the reading that matters belongs to
	// the database — the same clock v_grades.is_open is evaluated
	// against. Routing these through the drafts meant a third clock
	// wrote the value, rounded to the minute because that is what
	// toLocalInput emits, so pressing both inside one minute sent
	// closes_at = opens_at and was refused by the CHECK, leaving the
	// window open behind an error about a value nobody typed.
	//
	// So they send nothing and take nothing back. The refetch that
	// follows the write brings the new bounds in through the same
	// effects that follow the server everywhere else, which is also
	// why neither touches a dirty flag: there is no half-finished edit
	// here to protect.
	function openNow(): void {
		void run(() => openWindowNow(grade.id))
	}

	function closeNow(): void {
		void run(() => closeWindowNow(grade.id))
	}
</script>

<article class="card" aria-labelledby="{uid}-title">
	<div class="head">
		<span class="settings">
			<h3 id="{uid}-title">{grade.id}</h3>
			<input
				type="text"
				aria-label="Name of {grade.id}"
				bind:value={nameDraft}
				oninput={(): void => {
					nameDirty = true
				}}
				onchange={(): void => {
					const name = nameDraft.trim()
					if (name === "" || name === grade.name) {
						nameDraft = grade.name
						nameDirty = false
						return
					}
					void run(async (): Promise<void> => {
						await updateGrade(
							grade.id,
							name,
							grade.min_distinct_categories,
						)
						nameDirty = false
					})
				}}
			/>
			<!--
				The word alone read as "Open" with nothing to say what
				was open. It carried aria-label="Enrollment window",
				which ARIA does not allow on a generic element and which
				browsers therefore drop — so the label was written,
				ignored, and looked done. Visible text says it once, for
				everybody.
			-->
			<span class="state" data-open={grade.is_open ? "yes" : "no"}>
				Window: {grade.is_open ? "open" : "closed"}
			</span>
		</span>
		<span>
			<button
				aria-label="View students in {grade.id}"
				onclick={(): void => {
					window.location.hash = `#/students?q=${encodeURIComponent(
						`grade:${grade.id}`,
					)}`
				}}
			>
				Students
			</button>
			<button
				aria-label="Move {grade.id} up"
				disabled={busy}
				onclick={(): void => {
					move("up")
				}}
			>
				&uarr;
			</button>
			<button
				aria-label="Move {grade.id} down"
				disabled={busy}
				onclick={(): void => {
					move("down")
				}}
			>
				&darr;
			</button>
			{#if confirmingDelete}
				<button
					aria-label="Cancel deleting {grade.id}"
					onclick={(): void => {
						confirmingDelete = false
					}}
				>
					X
				</button>
				<button
					disabled={busy}
					aria-label="Confirm deleting {grade.id}"
					onclick={(): void => {
						confirmingDelete = false
						void run(() => deleteGrade(grade.id))
					}}
				>
					Confirm delete
				</button>
			{:else}
				<button
					disabled={busy}
					aria-label="Delete {grade.id}"
					onclick={(): void => {
						confirmingDelete = true
					}}
				>
					Delete
				</button>
			{/if}
		</span>
	</div>

	<!--
		The window is two instants, and whether it is open right now is
		derived from them by the server. Nothing is scheduled: a bound
		that passes while nobody is looking simply takes effect, so
		there is no state to repair after a restart.
	-->
	<fieldset>
		<legend>Enrollment window</legend>
		<label>
			Opens
			<input
				type="datetime-local"
				aria-label="Window opens for {grade.id}"
				bind:value={opensDraft}
				oninput={(): void => {
					opensDirty = true
				}}
				onchange={saveWindow}
			/>
		</label>
		<label>
			Closes
			<input
				type="datetime-local"
				aria-label="Window closes for {grade.id}"
				bind:value={closesDraft}
				oninput={(): void => {
					closesDirty = true
				}}
				onchange={saveWindow}
			/>
		</label>
		<button
			type="button"
			disabled={busy || grade.is_open}
			onclick={openNow}
		>
			Open now
		</button>
		<button
			type="button"
			disabled={busy || !grade.is_open}
			onclick={closeNow}
		>
			Close now
		</button>
		<span class="note">{windowSummary}</span>
		<span class="note bound-help">
			Empty opening = closed. Empty closing = no automatic close.
		</span>
	</fieldset>

	<fieldset>
		<legend>Limits</legend>
		<label>
			<button
				type="button"
				class="details"
				popovertarget="{uid}-budget"
				aria-label="Explain the period budget"
				style="anchor-name: --{uid}-budget"
			>
				Period budget
			</button>
			<input
				type="number"
				min="0"
				placeholder="none"
				aria-label="Period budget for {grade.id}"
				value={budgetValue}
				onchange={(event): void => {
					const box = event.currentTarget
					const raw = box.value.trim()
					const cap = raw === "" ? null : Number.parseInt(raw, 10)
					if (cap !== null && (Number.isNaN(cap) || cap < 0)) {
						box.value = budgetValue
						return
					}
					// Lowering the cap can put students already
					// enrolled over it, so this is the one setting here
					// that can come back asking to be confirmed — and
					// declining, or a refusal, must put the box back:
					// the value it holds otherwise is one the grade
					// does not have.
					void runAccepting((accept) =>
						setGradeBudget(grade.id, cap, accept),
					).then((applied): void => {
						if (!applied) {
							box.value = budgetValue
						}
					})
				}}
			/>
		</label>
		<div
			id="{uid}-budget"
			popover="auto"
			class="anchored"
			style="position-anchor: --{uid}-budget"
		>
			The number of periods a student in this grade may occupy with
			budgeted enrollments. A course meeting twice a week counts 2. Leave
			empty for no cap.
		</div>
	</fieldset>

	<!--
		Requirements are advisory and are edited as a set: rows are
		added and removed here, and one Save replaces the lot. Nothing
		is written until then, so a half-built requirement never
		reaches the server.
	-->
	<fieldset>
		<legend>Requirements</legend>
		<label>
			<button
				type="button"
				class="details"
				popovertarget="{uid}-mdc"
				aria-label="Explain the category minimum"
				style="anchor-name: --{uid}-mdc"
			>
				Distinct categories
			</button>
			<input
				type="number"
				min="0"
				aria-label="Distinct categories for {grade.id}"
				value={grade.min_distinct_categories}
				onchange={(event): void => {
					const box = event.currentTarget
					const min = Number.parseInt(box.value.trim(), 10)
					if (Number.isNaN(min) || min < 0) {
						box.value = String(grade.min_distinct_categories)
						return
					}
					void run(() => updateGrade(grade.id, grade.name, min)).then(
						(applied): void => {
							if (!applied) {
								box.value = String(
									grade.min_distinct_categories,
								)
							}
						},
					)
				}}
			/>
		</label>
		<div
			id="{uid}-mdc"
			popover="auto"
			class="anchored"
			style="position-anchor: --{uid}-mdc"
		>
			Enrollments in scheduled courses must span at least this many
			different categories; 0 disables the requirement. Advisory: it is
			reported to the student, never enforced.
		</div>
		<div class="scrolls requirement-list">
			<table>
				<thead>
					<tr>
						<th scope="col">Categories</th>
						<th scope="col">Periods required</th>
						<th scope="col"></th>
					</tr>
				</thead>
				<tbody>
					{#each requirementDrafts as requirement, index (index)}
						<tr>
							<td>
								{#each categories as category (category.id)}
									<label class="pick">
										<input
											type="checkbox"
											value={category.id}
											bind:group={
												requirement.category_ids
											}
											onchange={touchRequirements}
										/>
										{category.id}
									</label>
								{/each}
							</td>
							<td>
								<input
									type="text"
									inputmode="numeric"
									aria-label="Periods required"
									bind:value={requirement.count}
									oninput={touchRequirements}
								/>
							</td>
							<td>
								<button
									type="button"
									aria-label="Remove this requirement"
									onclick={(): void => {
										touchRequirements()
										requirementDrafts =
											requirementDrafts.filter(
												(_, i) => i !== index,
											)
									}}
								>
									Remove
								</button>
							</td>
						</tr>
					{:else}
						<tr>
							<td colspan="3">No requirements.</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		<button
			type="button"
			disabled={categories.length === 0}
			onclick={(): void => {
				touchRequirements()
				requirementDrafts = [
					...requirementDrafts,
					{ count: "1", category_ids: [] },
				]
			}}
		>
			Add requirement
		</button>
		<button
			type="button"
			disabled={busy}
			onclick={(): void => {
				// Said out loud rather than returning quietly: a
				// button that does nothing is indistinguishable from
				// a broken one.
				if (requirementsMovedElsewhere) {
					requirementError =
						"Somebody else changed these requirements while " +
						"you were editing. Revert to see theirs, then " +
						"make your change again."
					return
				}
				const read = readRequirements()
				if (!read.ok) {
					requirementError = read.why
					return
				}
				requirementError = null
				void run(async (): Promise<void> => {
					await setGradeRequirements(grade.id, read.value)
					requirementsDirty = false
				})
			}}
		>
			Save requirements
		</button>
		<button
			type="button"
			onclick={(): void => {
				requirementDrafts = draftsOf(grade)
				requirementsBasis = requirementsOnServer
				requirementsDirty = false
				requirementError = null
			}}
		>
			Revert
		</button>
		{#if requirementError !== null}
			<p class="requirementerror" role="alert">{requirementError}</p>
		{/if}
	</fieldset>
</article>

<style>
	.head {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 0.5rem;

		h3 {
			font-size: 1em;
			margin: 0;
		}

		.settings {
			display: inline-flex;
			align-items: baseline;
			gap: 0.5rem;
		}
	}

	.state[data-open="yes"] {
		font-weight: bold;
	}

	.state[data-open="no"] {
		color: color-mix(in oklab, var(--fg) 60%, var(--bg));
	}

	fieldset {
		display: flex;
		align-items: baseline;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-top: 0.5rem;
		border: 1px solid color-mix(in oklab, var(--fg) 20%, var(--bg));

		label {
			display: inline-flex;
			align-items: baseline;
			gap: 0.25rem;
		}

		input[type="number"] {
			width: 5rem;
		}

		table {
			width: 100%;
		}
	}

	.note {
		font-size: 0.875rem;
		color: color-mix(in oklab, var(--fg) 65%, var(--bg));
	}

	.bound-help,
	.requirement-list {
		flex-basis: 100%;
	}

	.requirementerror {
		flex-basis: 100%;
		margin: 0;
		color: var(--danger);
	}

	label.pick {
		display: inline-flex;
		align-items: baseline;
		gap: 0.25rem;
		margin-right: 0.5rem;
	}

	button.details {
		background: none;
		border: none;
		padding: 0;
		font: inherit;
		color: inherit;
		text-decoration: underline dotted;
		cursor: pointer;
	}

	div[popover] {
		max-width: 24rem;
	}
</style>
