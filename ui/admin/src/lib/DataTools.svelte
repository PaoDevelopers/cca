<script lang="ts">
	// Bulk operations for one data section. The full reset is gated
	// behind typing an exact confirmation phrase.
	import { clearSection, importSpreadsheet } from "@common/adminApi"

	interface Props {
		section: string
		exportHref?: string | undefined
		importAction?: string | undefined
		busy: boolean
		run: (action: () => Promise<void>) => Promise<boolean>
	}

	const { section, exportHref, importAction, busy, run }: Props = $props()

	const confirmation = "I confirm that I wish to destroy all data here."

	let files = $state<FileList | null>(null)
	let phrase = $state("")
</script>

<details class="data-tools">
	<summary>Data tools</summary>

	{#if exportHref !== undefined}
		<p><a href={exportHref}>Export {section} as CSV</a></p>
	{/if}

	{#if importAction !== undefined}
		<!--
			An import form with no example is how a wrong file gets
			made: the columns are not guessable, and the identifier
			forms — an uppercase course id, a lowercase student
			localpart, a grade named by id rather than by name — are
			not either.

			Named by section rather than passed in, so a section that
			imports always has one. Which files exist, and that their
			headers still match the importers, is held by
			TestTheShippedExamplesHaveTheHeadersTheImportersWant.
		-->
		<p>
			<a href="/admin/static/{section}_example.csv" download>
				Download {section} CSV template
			</a>
		</p>
		<p class="hint">
			Upload that CSV format directly, or put the same columns in the
			first row of one visible worksheet in an XLSX file. Paste formulas
			as values first.
		</p>

		<form
			onsubmit={(event): void => {
				event.preventDefault()
				const file = files?.item(0)
				if (file === null || file === undefined) {
					return
				}
				// No override: an import accepts nothing. A row that
				// would break a rule is reported so somebody decides
				// on it deliberately, rather than being waved through
				// by the act of uploading a file.
				void run(async (): Promise<void> => {
					await importSpreadsheet(importAction, file)
					files = null
				})
			}}
		>
			<label>
				Import CSV or XLSX
				<input
					type="file"
					accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
					bind:files
					required
				/>
			</label>
			<button disabled={busy}>Import into {section}</button>
		</form>
	{/if}

	<form
		onsubmit={(event): void => {
			event.preventDefault()
			if (phrase !== confirmation) {
				return
			}
			void run(async (): Promise<void> => {
				await clearSection(section)
				phrase = ""
			})
		}}
	>
		<label>
			Type &ldquo;{confirmation}&rdquo; to enable the reset
			<input type="text" bind:value={phrase} />
		</label>
		<button
			disabled={busy || phrase !== confirmation}
			aria-label="Delete all {section}"
		>
			Delete all {section}
		</button>
	</form>
</details>

<style>
	details {
		margin-top: 1.5rem;

		form {
			display: flex;
			flex-wrap: wrap;
			gap: 0.5rem 1rem;
			align-items: baseline;
			margin-top: 0.5rem;
		}

		label {
			display: inline-flex;
			align-items: baseline;
			gap: 0.25rem;
		}

		input[type="text"] {
			width: 24rem;
			max-width: 100%;
		}

		.hint {
			max-width: 46rem;
			color: color-mix(in oklab, var(--fg) 65%, var(--bg));
		}
	}
</style>
