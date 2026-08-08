<script lang="ts">
	// An error popover, anchored to whatever the failing action was
	// triggered from. Light-dismisses through ondismiss.
	import type { Attachment } from "svelte/attachments"

	interface Props {
		message: string | null
		ondismiss: () => void
	}

	const { message, ondismiss }: Props = $props()

	// The failing action's button is disabled while the request runs,
	// which drops focus to <body> before the error arrives — so the
	// focused element is useless as an anchor.
	let lastInteraction: HTMLElement | null = null

	// Whether the popover found something to anchor to. Kept as state
	// and applied through the class attribute so the style below is
	// ordinary scoped CSS: setting the class imperatively would hide it
	// from the compiler, which then needs :global to keep the rule.
	let anchored = $state(false)

	// Capture phase, so the originating element is recorded even when a
	// handler further down stops propagation.
	function remember(event: Event): void {
		if (event.target instanceof HTMLElement) {
			lastInteraction = event.target
		}
	}

	// Shown for as long as there is a message. Taking the message as an
	// argument is what ties the attachment to it: Svelte tears this one
	// down and runs a fresh one whenever it changes, which is where the
	// anchor gets released.
	function shown(current: string | null): Attachment<HTMLElement> {
		return (el): (() => void) | void => {
			if (current === null) {
				if (el.matches(":popover-open")) {
					el.hidePopover()
				}
				return
			}
			const source =
				(lastInteraction?.isConnected ?? false) ? lastInteraction : null
			if (source !== null) {
				source.style.setProperty("anchor-name", "--error-source")
			}
			anchored = source !== null
			// One message replacing another re-runs this with the
			// popover still open, and showPopover() throws on an open
			// popover.
			if (!el.matches(":popover-open")) {
				el.showPopover()
			}
			return (): void => {
				source?.style.removeProperty("anchor-name")
			}
		}
	}
</script>

<svelte:window onpointerdowncapture={remember} onkeydowncapture={remember} />

<div
	{@attach shown(message)}
	popover="auto"
	class={{ error: true, anchored }}
	role="alert"
	ontoggle={(event): void => {
		if (event.newState === "closed" && message !== null) {
			ondismiss()
		}
	}}
>
	{message}
</div>

<style>
	div.error {
		position-anchor: --error-source;
		max-width: 24rem;
		/* Messages carry their own line breaks: a rejection is one
		   line and any hint about it is the next. */
		white-space: pre-line;
		border: 1px solid var(--danger);
		background-color: color-mix(in oklab, var(--bg) 92%, var(--danger));
	}

	div.error.anchored {
		margin: 0.25rem 0;
		inset: auto;
		position-area: block-end span-inline-end;
		position-try-fallbacks: flip-block, flip-inline;
	}
</style>
