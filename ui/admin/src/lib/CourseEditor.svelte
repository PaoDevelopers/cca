<script lang="ts">
	import type { CourseInput } from "@common/adminApi"
	import {
		courseTermSuggestions,
		type Course,
		type LegalSex,
	} from "@common/types"

	interface Props {
		// null means creating a new course.
		initial: Course | null
		categories: string[]
		periods: string[]
		gradeIDs: string[]
		busy: boolean
		onsave: (input: CourseInput) => void
		oncancel?: () => void
	}

	const {
		initial,
		categories,
		periods,
		gradeIDs,
		busy,
		onsave,
		oncancel,
	}: Props = $props()

	// The editor deliberately snapshots the course it was opened for:
	// the page remounts it per target, so prop reactivity is unwanted.
	// svelte-ignore state_referenced_locally
	const base = initial

	let id = $state(base?.id ?? "")
	let name = $state(base?.name ?? "")
	let description = $state(base?.description ?? "")
	let teacher = $state(base?.teacher ?? "")
	let teacherEmail = $state(base?.teacher_email ?? "")
	let location = $state(base?.location ?? "")
	let term = $state(base?.term ?? "Season")
	let cost = $state(base?.cost ?? "")
	let categoryID = $state(base?.category_id ?? "")
	let inviteOnly = $state(base?.invite_only ?? false)
	// The empty box is a real setting — no cap — so the draft is the
	// text, not a number: coercing it would turn "" into 0, which is
	// the opposite setting and admits nobody.
	const baseMax = base?.max_students ?? null
	let maxStudents = $state(baseMax === null ? "" : String(baseMax))
	let selectedPeriods = $state<string[]>([...(base?.period_ids ?? [])])
	let allowedSexes = $state<LegalSex[]>([
		...(base?.allowed_legal_sexes ?? []),
	])
	let allowedGrades = $state<string[]>([...(base?.allowed_grade_ids ?? [])])

	const sexes: LegalSex[] = ["F", "M", "X"]
</script>

<form
	class="editor"
	onsubmit={(event): void => {
		event.preventDefault()
		// An empty box, and the word the import takes for it, both mean
		// no cap.
		const raw = maxStudents.trim().toLowerCase()
		const max =
			raw === "" || raw === "unlimited" ? null : Number.parseInt(raw, 10)
		if (max !== null && (Number.isNaN(max) || max < 0)) {
			return
		}
		// The accept list is filled in by the caller if the server
		// comes back with violations; a first attempt always accepts
		// nothing.
		onsave({
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
		})
	}}
>
	<div class="fields">
		<label>
			ID
			<input
				type="text"
				bind:value={id}
				disabled={initial !== null}
				required
			/>
		</label>
		<label>
			Name
			<input type="text" bind:value={name} required />
		</label>
		<label>
			Teacher
			<input type="text" bind:value={teacher} required />
		</label>
		<label>
			Teacher e-mail
			<input type="email" bind:value={teacherEmail} />
		</label>
		<label>
			Location
			<input type="text" bind:value={location} required />
		</label>
		<label>
			<!--
				Free text with suggestions, not a fixed set, and not
				required: the software never acts on a term's value, so
				constraining it here would only stop the department
				writing what their spreadsheet says — or leaving it
				blank, which is what a department that does not divide
				its season into terms writes.
			-->
			Term
			<input type="text" bind:value={term} list="course-terms" />
			<datalist id="course-terms">
				{#each courseTermSuggestions as option (option)}
					<option value={option}></option>
				{/each}
			</datalist>
		</label>
		<label>
			Cost
			<input type="text" bind:value={cost} />
		</label>
		<label>
			Category
			<select bind:value={categoryID} required>
				<option value="" disabled>Choose</option>
				{#each categories as category (category)}
					<option value={category}>{category}</option>
				{/each}
			</select>
		</label>
		<label class="pick">
			<input type="checkbox" bind:checked={inviteOnly} />
			Invite only
		</label>
		<label>
			Capacity
			<!--
				Left empty, the course takes everyone. Not a number
				input: Svelte coerces bind:value on one, and an empty
				box would arrive as null indistinguishably from a box
				holding nothing yet — and the word "unlimited", which
				the import accepts, could not be typed into it at all.
			-->
			<input
				type="text"
				inputmode="numeric"
				placeholder="unlimited"
				aria-label="Capacity, empty for no limit"
				bind:value={maxStudents}
				class="narrow"
			/>
		</label>
	</div>

	<label class="wide">
		Description
		<textarea rows="2" bind:value={description}></textarea>
	</label>

	<fieldset>
		<legend>Periods</legend>
		{#each periods as period (period)}
			<label class="pick">
				<input
					type="checkbox"
					value={period}
					bind:group={selectedPeriods}
				/>
				{period}
			</label>
		{:else}
			No periods defined.
		{/each}
	</fieldset>

	<fieldset>
		<legend>Allowed legal sexes (none checked = all)</legend>
		{#each sexes as sex (sex)}
			<label class="pick">
				<input type="checkbox" value={sex} bind:group={allowedSexes} />
				{sex}
			</label>
		{/each}
	</fieldset>

	<fieldset>
		<legend>Allowed grades (none checked = all)</legend>
		{#each gradeIDs as gradeID (gradeID)}
			<label class="pick">
				<input
					type="checkbox"
					value={gradeID}
					bind:group={allowedGrades}
				/>
				{gradeID}
			</label>
		{:else}
			No grades defined.
		{/each}
	</fieldset>

	<div class="controls">
		<!--
			No override checkbox. Saving an edit that would break a rule
			comes back with exactly what it would break, and is
			confirmed then — so an administrator agrees to specific
			consequences rather than switching off the checks in
			advance.
		-->
		<span></span>
		<span>
			{#if oncancel !== undefined}
				<button type="button" onclick={oncancel}>Cancel</button>
			{/if}
			<button disabled={busy}>
				{initial === null ? "Create" : "Save"}
			</button>
		</span>
	</div>
</form>

<style>
	form.editor {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;

		.fields {
			display: flex;
			flex-wrap: wrap;
			gap: 0.5rem 1rem;
			align-items: baseline;
		}

		label {
			display: inline-flex;
			align-items: baseline;
			gap: 0.25rem;
		}

		label.wide {
			display: flex;

			textarea {
				flex: 1;
			}
		}

		input.narrow {
			width: 4rem;
		}

		fieldset {
			border: 1px solid color-mix(in oklab, var(--bg) 85%, var(--fg));
			display: flex;
			flex-wrap: wrap;
			gap: 0.25rem 0.75rem;
		}

		label.pick {
			display: inline-flex;
			align-items: baseline;
			gap: 0.25rem;
		}

		.controls {
			display: flex;
			justify-content: space-between;
			align-items: baseline;
			gap: 0.5rem;
		}
	}
</style>
