<script lang="ts">
	// A compact multi-select. Checked options are ORed together by the
	// caller's filter.
	//
	// Each option is an id and a name, because those are two different
	// things here and only one of them is for reading: the filter is by
	// id, and the student is shown "Monday 1" rather than "MON1". This
	// took a pair rather than a list of ids for a while, and the list
	// of ids is what a student saw.
	interface Option {
		id: string
		name: string
	}

	interface Props {
		label: string
		options: Option[]
		selected: string[]
	}

	let { label, options, selected = $bindable() }: Props = $props()

	const uid = $props.id()
</script>

<span>
	<button
		type="button"
		popovertarget="{uid}-pop"
		style="anchor-name: --{uid}-pop"
		aria-label="Filter by {label}"
	>
		{label}{selected.length > 0 ? ` (${selected.length})` : ""} &#9662;
	</button>
	<div
		id="{uid}-pop"
		popover="auto"
		class="anchored"
		style="position-anchor: --{uid}-pop"
	>
		<ul>
			{#each options as option (option.id)}
				<li>
					<label>
						<input
							type="checkbox"
							value={option.id}
							bind:group={selected}
						/>
						{option.name}
					</label>
				</li>
			{:else}
				<li>Nothing to filter by.</li>
			{/each}
		</ul>
		{#if selected.length > 0}
			<button
				type="button"
				onclick={(): void => {
					selected = []
				}}
			>
				Clear
			</button>
		{/if}
	</div>
</span>

<style>
	ul {
		list-style: none;
		margin: 0.25rem 0 0;
		padding: 0;
		max-height: 16rem;
		overflow-y: auto;
	}

	li label {
		display: flex;
		align-items: baseline;
		gap: 0.25rem;
	}
</style>
