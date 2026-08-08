<script lang="ts">
	// A destructive action behind a two-step confirmation.
	//
	// The pending state is per instance, so arming one does not disarm
	// another: several can be armed at once, and each disarms only
	// itself. That is the point — a page-wide "which one is armed"
	// would let a refetch that reordered the rows move the arming from
	// the row somebody looked at to the row above it.
	interface Props {
		// The action verb, e.g. "Delete"; also builds accessible names.
		action: string
		// Reads after the verb: "the enrollment of BB for 1".
		target: string
		disabled: boolean
		onconfirm: () => void
	}

	const { action, target, disabled, onconfirm }: Props = $props()

	let armed = $state(false)
</script>

{#if armed}
	<button
		aria-label="Cancel {action.toLowerCase()} of {target}"
		onclick={(): void => {
			armed = false
		}}
	>
		X
	</button>
	<button
		{disabled}
		aria-label="Confirm {action.toLowerCase()} of {target}"
		onclick={(): void => {
			armed = false
			onconfirm()
		}}
	>
		Confirm {action.toLowerCase()}
	</button>
{:else}
	<button
		{disabled}
		aria-label="{action} {target}"
		onclick={(): void => {
			armed = true
		}}
	>
		{action}
	</button>
{/if}
