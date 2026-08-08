<script lang="ts">
	// A popover documenting the active filter syntax and the fields
	// available on the current page.
	import type { FilterMode } from "@common/cel"

	interface Props {
		fields: Record<string, string>
		mode: FilterMode
		// A worked expression, shown for CEL only.
		example?: string | undefined
	}

	const { fields, mode, example }: Props = $props()

	const uid = $props.id()
</script>

<button
	type="button"
	class="details"
	popovertarget="{uid}-help"
	aria-label="Filter syntax help"
	style="anchor-name: --{uid}-help"
>
	(help)
</button>
<div
	id="{uid}-help"
	popover="auto"
	class="anchored"
	style="position-anchor: --{uid}-help"
>
	{#if mode === "cel"}
		<p>
			A <a href="https://cel.dev/">CEL</a> expression evaluated per row,
			returning true or false. Strings need quotes and match exactly;
			<code>.contains()</code>, <code>.lowerAscii()</code> and
			<code>.matches()</code> are available, and <code>in</code> tests list
			membership.
		</p>
		{#if example !== undefined}
			<p>e.g. <code>{example}</code></p>
		{/if}
	{:else}
		<p>
			Space-separated terms; rows must match all of them. A bare term
			matches anywhere. <code>field:value</code> matches one field, and
			<code>field:a,b</code> matches either value. Matching is case-insensitive;
			fields marked exact match whole values only, the rest match substrings.
		</p>
	{/if}
	<dl class="grid">
		{#each Object.entries(fields) as [name, description] (name)}
			<dt><code>{name}</code></dt>
			<dd>{description}</dd>
		{/each}
	</dl>
</div>

<style>
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
		max-width: 26rem;

		p {
			margin-top: 0;
		}
	}
</style>
