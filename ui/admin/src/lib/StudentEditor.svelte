<script lang="ts">
	// The editable form of one student's row. Mounted only while that
	// row is being edited, so it can own its draft outright.
	import type { AdminStudent } from "@common/adminApi"
	import type { LegalSex } from "@common/types"

	interface Props {
		student: AdminStudent
		gradeIDs: string[]
		sexes: LegalSex[]
		budget: string
		requirements: string
		busy: boolean
		onsave: (draft: AdminStudent) => void
		oncancel: () => void
	}

	const {
		student,
		gradeIDs,
		sexes,
		budget,
		requirements,
		busy,
		onsave,
		oncancel,
	}: Props = $props()

	// The editor deliberately snapshots the student it was opened for:
	// the row mounts it per edit, so prop reactivity is unwanted — a
	// refetch mid-edit would otherwise discard what is being typed.
	// svelte-ignore state_referenced_locally
	let name = $state(student.name)
	// svelte-ignore state_referenced_locally
	let gradeID = $state(student.grade_id)
	// svelte-ignore state_referenced_locally
	let legalSex = $state<LegalSex>(student.legal_sex)
</script>

<tr>
	<th scope="row">{student.id}</th>
	<td>
		<label>
			Name
			<input type="text" bind:value={name} />
		</label>
	</td>
	<td>
		<label>
			Grade
			<select bind:value={gradeID}>
				{#each gradeIDs as option (option)}
					<option value={option}>
						{option}
					</option>
				{/each}
			</select>
		</label>
	</td>
	<td>
		<label>
			Legal sex
			<select bind:value={legalSex}>
				{#each sexes as sex (sex)}
					<option value={sex}>{sex}</option>
				{/each}
			</select>
		</label>
	</td>
	<td>{budget}</td>
	<td>{requirements}</td>
	<td>
		<button aria-label="Cancel editing {student.id}" onclick={oncancel}>
			Cancel
		</button>
		<button
			disabled={busy}
			aria-label="Save {student.id}"
			onclick={(): void => {
				onsave({
					id: student.id,
					name: name.trim(),
					grade_id: gradeID,
					legal_sex: legalSex,
				})
			}}
		>
			Save
		</button>
	</td>
</tr>

<style>
	td label {
		display: inline-flex;
		align-items: baseline;
		gap: 0.25rem;
	}
</style>
