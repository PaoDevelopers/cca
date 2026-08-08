<script lang="ts">
	import {
		createPeriod,
		deletePeriod,
		renamePeriod,
		setPeriodOrder,
	} from "@common/adminApi"
	import { reordered } from "./ordering"
	import DataTools from "./DataTools.svelte"
	import { getAdminData } from "./data.svelte"

	const data = getAdminData()
	data.want("periods")

	let newID = $state("")
	let newName = $state("")
	let confirmingDelete = $state<string | null>(null)

	function move(id: string, direction: "up" | "down"): void {
		void run(() => setPeriodOrder(reordered(data.periodIDs, id, direction)))
	}

	function run(action: () => Promise<void>): Promise<boolean> {
		return data.run(action, "periods")
	}
</script>

<section aria-labelledby="periods-heading">
	<!--
		The page had no heading below the h1 at all, so heading
		navigation went straight from "CCA administration" to nothing and
		the section was an unlabelled region. tabindex because it is also
		where the data layer puts the focus when the control that started
		a write no longer exists.
	-->
	<h2 id="periods-heading" tabindex="-1">Periods</h2>

	{#if !data.ready("periods")}
		<p>Loading&hellip;</p>
	{:else}
		<div class="scrolls">
			<table>
				<thead>
					<tr>
						<th scope="col">ID</th>
						<th scope="col">Name</th>
						<th scope="col">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each data.periods as period (period.id)}
						<tr>
							<th scope="row">{period.id}</th>
							<td>
								<!--
								The name is inert prose: it may carry a
								wall-clock time, but nothing computes
								with it. Committed on blur.
							-->
								<input
									type="text"
									aria-label="Name of {period.id}"
									value={period.name}
									onchange={(event): void => {
										const name =
											event.currentTarget.value.trim()
										if (
											name === "" ||
											name === period.name
										) {
											return
										}
										void run(() =>
											renamePeriod(period.id, name),
										)
									}}
								/>
							</td>
							<td>
								<button
									aria-label="View courses meeting in {period.id}"
									onclick={(): void => {
										window.location.hash = `#/courses?q=${encodeURIComponent(
											`period:${period.id}`,
										)}`
									}}
								>
									Courses
								</button>
								<button
									aria-label="Move {period.id} up"
									disabled={data.busy}
									onclick={(): void => {
										move(period.id, "up")
									}}
								>
									&uarr;
								</button>
								<button
									aria-label="Move {period.id} down"
									disabled={data.busy}
									onclick={(): void => {
										move(period.id, "down")
									}}
								>
									&darr;
								</button>
								{#if confirmingDelete === period.id}
									<button
										aria-label="Cancel deleting {period.id}"
										onclick={(): void => {
											confirmingDelete = null
										}}
									>
										X
									</button>
									<button
										disabled={data.busy}
										aria-label="Confirm deleting {period.id}"
										onclick={(): void => {
											confirmingDelete = null
											void run(() =>
												deletePeriod(period.id),
											)
										}}
									>
										Confirm delete
									</button>
								{:else}
									<button
										disabled={data.busy}
										aria-label="Delete {period.id}"
										onclick={(): void => {
											confirmingDelete = period.id
										}}
									>
										Delete
									</button>
								{/if}
							</td>
						</tr>
					{:else}
						<tr>
							<td colspan="3">No periods yet.</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<form
			onsubmit={(event): void => {
				event.preventDefault()
				const id = newID.trim()
				const name = newName.trim()
				if (id === "" || name === "") {
					return
				}
				void run(async (): Promise<void> => {
					await createPeriod(id, name)
					newID = ""
					newName = ""
				})
			}}
		>
			<label>
				New period ID
				<input type="text" bind:value={newID} required />
			</label>
			<label>
				Name
				<input type="text" bind:value={newName} required />
			</label>
			<button disabled={data.busy}>Add</button>
		</form>

		<DataTools section="periods" busy={data.busy} {run} />
	{/if}
</section>

<style>
	form {
		margin-top: 1rem;
	}
</style>
