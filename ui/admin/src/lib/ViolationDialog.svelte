<script lang="ts">
	// What a refused write would break, put to the administrator as a
	// list rather than a single message. Every negotiable write in the
	// app surfaces here: the protocol is uniform, so its presentation
	// is too.
	//
	// Confirming accepts exactly the violations shown. It is not an
	// "override everything" switch — a violation that appears between
	// the refusal and the retry is not in the list and still stops the
	// write, which is what makes confirming safe under contention.
	import type { Violation } from "@common/types"

	interface Props {
		violations: Violation[]
		answer: (accepted: boolean) => void
	}

	const { violations, answer }: Props = $props()

	// Grouped by student where there is one, so a batch reads as "what
	// is wrong with each person" rather than a flat list of codes.
	const groups = $derived.by(
		(): Array<{ who: string; details: string[] }> => {
			const byWho = new Map<string, string[]>()
			for (const v of violations) {
				const who = v.student_id ?? ""
				const list = byWho.get(who)
				if (list === undefined) {
					byWho.set(who, [v.detail])
				} else {
					list.push(v.detail)
				}
			}
			return [...byWho].map(([who, details]) => ({ who, details }))
		},
	)

	// A real <dialog> in modal mode, rather than a div wearing the
	// role: it takes the focus, traps it, closes on Escape, and paints
	// its own backdrop, all of which would otherwise be hand-written
	// and half-right.
	//
	// Every exit goes through close(), including the buttons, because
	// restoring focus to whatever opened the dialog is something the
	// browser does on close() and not on the element being removed.
	// The buttons used to answer directly, which unmounted the dialog
	// without closing it: confirming a violation left the focus on
	// <body>, so the next Tab started again at the top of the page —
	// after a write, which is exactly when somebody is most likely to
	// still be working by keyboard.
	let dialog = $state<HTMLDialogElement | null>(null)

	$effect((): void => {
		dialog?.showModal()
	})
</script>

<dialog
	bind:this={dialog}
	aria-labelledby="violation-title"
	onclose={(): void => {
		// The single exit for every path. Escape and the backdrop close
		// it without a returnValue, and no answer is no.
		//
		// Where the focus lands afterwards is the data layer's: it
		// recorded the button that started the write and puts the focus
		// back when the write finishes, which is after this either way.
		answer(dialog?.returnValue === "accept")
	}}
>
	<h2 id="violation-title">
		This would break {violations.length} rule{violations.length === 1
			? ""
			: "s"}
	</h2>

	<ul>
		{#each groups as group (group.who)}
			<li>
				{#if group.who !== ""}<strong>{group.who}</strong>{/if}
				<ul>
					{#each group.details as detail, i (i)}
						<li>{detail}</li>
					{/each}
				</ul>
			</li>
		{/each}
	</ul>

	<p class="note">
		Continuing accepts exactly these. Nothing has been saved yet.
	</p>

	<div class="actions">
		<button
			type="button"
			onclick={(): void => {
				dialog?.close("cancel")
			}}>Cancel</button
		>
		<button
			type="button"
			class="danger"
			onclick={(): void => {
				dialog?.close("accept")
			}}>Continue anyway</button
		>
	</div>
</dialog>

<style>
	dialog {
		max-width: 32rem;
		max-height: 80vh;
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

	ul {
		padding-left: 1.25rem;
	}

	.note {
		font-size: 0.875rem;
		color: color-mix(in oklab, var(--fg) 70%, var(--bg));
	}

	.actions {
		display: flex;
		gap: 0.5rem;
		justify-content: flex-end;
	}

	.danger {
		color: var(--danger);
	}
</style>
