<script lang="ts">
	// Controls bind to fields of the filter the parent passes in, so it
	// is taken as a binding: mutating a plain prop reaches the parent
	// too, but only because the object is a state proxy, and Svelte
	// warns about writing to state the component does not own.
	import type { Category, Period } from "@common/types"
	import type { CourseFilter } from "./enrollment"
	import MultiSelect from "./MultiSelect.svelte"

	interface Props {
		filter: CourseFilter
		categories: Category[]
		periods: Period[]
	}

	let { filter = $bindable(), categories, periods }: Props = $props()

	// The facets pick by id and label by name.
	const categoryOptions = $derived(
		categories.map((c): { id: string; name: string } => ({
			id: c.id,
			name: c.name,
		})),
	)
	const periodOptions = $derived(
		periods.map((p): { id: string; name: string } => ({
			id: p.id,
			name: p.name,
		})),
	)
</script>

<p class="facets">
	<label>
		Search
		<input type="search" bind:value={filter.search} />
	</label>
	<MultiSelect
		label="Category"
		options={categoryOptions}
		bind:selected={filter.categories}
	/>
	<MultiSelect
		label="Period"
		options={periodOptions}
		bind:selected={filter.periods}
	/>
	<label>
		<input type="checkbox" bind:checked={filter.hideFull} />
		Hide full
	</label>
	<label>
		<input type="checkbox" bind:checked={filter.hideInviteOnly} />
		Hide invite-only
	</label>
	<label>
		<input type="checkbox" bind:checked={filter.hideIncompatible} />
		Hide incompatible
	</label>
	<label>
		<input type="checkbox" bind:checked={filter.hideConflicting} />
		Hide conflicting
	</label>
</p>

<style>
	.facets {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem 1rem;
		align-items: baseline;

		label {
			display: inline-flex;
			align-items: baseline;
			gap: 0.25rem;
		}
	}
</style>
