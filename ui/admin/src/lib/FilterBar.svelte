<script lang="ts">
	// The filter control shared by the admin lists. CEL gets a
	// textarea; its expressions run off the end of a one-line input.
	import FilterHelp from "./FilterHelp.svelte"
	import type { FilterMode } from "@common/cel"

	interface Props {
		query: string
		mode: FilterMode
		// Field descriptions for the simple-mode help popover.
		fields: Record<string, string>
		// Dotted paths available to a CEL expression.
		celFields: Record<string, string>
		// A worked CEL expression for the help popover.
		celExample: string
		// Parse or evaluation error from the current CEL expression.
		error?: string | null | undefined
		onclear?: (() => void) | undefined
	}

	let {
		query = $bindable(),
		mode = $bindable(),
		fields,
		celFields,
		celExample,
		error = null,
		onclear,
	}: Props = $props()
</script>

<p class="filterbar">
	<label>
		Filter
		{#if mode === "cel"}
			<textarea rows="2" spellcheck="false" bind:value={query}></textarea>
		{:else}
			<input type="search" bind:value={query} />
		{/if}
	</label>
	<button
		type="button"
		aria-label="Clear the filter"
		disabled={query === ""}
		onclick={(): void => {
			query = ""
			onclear?.()
		}}
	>
		Clear
	</button>
	<label>
		<span class="visually-hidden">Filter mode</span>
		<select bind:value={mode}>
			<option value="simple">Simple</option>
			<option value="cel">CEL</option>
		</select>
	</label>
	<FilterHelp
		{mode}
		fields={mode === "cel" ? celFields : fields}
		example={mode === "cel" ? celExample : undefined}
	/>
</p>

{#if error !== null}
	<p class="filtererror" role="alert">{error}</p>
{/if}

<style>
	p.filterbar {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.5rem;

		label {
			display: inline-flex;
			align-items: baseline;
			gap: 0.25rem;
		}

		textarea {
			font: inherit;
			min-width: 24rem;
			resize: both;
		}
	}

	p.filtererror {
		margin: 0.25rem 0;
		border: 1px solid var(--danger);
		background-color: color-mix(in oklab, var(--bg) 92%, var(--danger));
		padding: 0.4rem 0.6rem;
	}

	.visually-hidden {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}
</style>
