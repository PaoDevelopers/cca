<script lang="ts">
	import {
		createCategory,
		deleteCategory,
		renameCategory,
	} from "@common/adminApi"
	import DataTools from "./DataTools.svelte"
	import { getAdminData } from "./data.svelte"

	const data = getAdminData()
	data.want("categories")

	let newID = $state("")
	let newName = $state("")
	let confirmingDelete = $state<string | null>(null)

	function run(action: () => Promise<void>): Promise<boolean> {
		return data.run(action, "categories")
	}
</script>

<section aria-labelledby="categories-heading">
	<!--
		The page had no heading below the h1 at all, so heading
		navigation went straight from "CCA administration" to nothing and
		the section was an unlabelled region. tabindex because it is also
		where the data layer puts the focus when the control that started
		a write no longer exists.
	-->
	<h2 id="categories-heading" tabindex="-1">Categories</h2>

	{#if !data.ready("categories")}
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
					{#each data.categories as category (category.id)}
						<tr>
							<th scope="row">{category.id}</th>
							<td>
								<!--
								Committed on blur rather than on every
								keystroke: renaming is one intention, and
								a request per character would be several.
							-->
								<input
									type="text"
									aria-label="Name of {category.id}"
									value={category.name}
									onchange={(event): void => {
										const name =
											event.currentTarget.value.trim()
										if (
											name === "" ||
											name === category.name
										) {
											return
										}
										void run(() =>
											renameCategory(category.id, name),
										)
									}}
								/>
							</td>
							<td>
								<button
									aria-label="View courses in {category.id}"
									onclick={(): void => {
										window.location.hash = `#/courses?q=${encodeURIComponent(
											`category:${category.id}`,
										)}`
									}}
								>
									Courses
								</button>
								{#if confirmingDelete === category.id}
									<button
										aria-label="Cancel deleting {category.id}"
										onclick={(): void => {
											confirmingDelete = null
										}}
									>
										X
									</button>
									<button
										disabled={data.busy}
										aria-label="Confirm deleting {category.id}"
										onclick={(): void => {
											confirmingDelete = null
											void run(() =>
												deleteCategory(category.id),
											)
										}}
									>
										Confirm delete
									</button>
								{:else}
									<button
										disabled={data.busy}
										aria-label="Delete {category.id}"
										onclick={(): void => {
											confirmingDelete = category.id
										}}
									>
										Delete
									</button>
								{/if}
							</td>
						</tr>
					{:else}
						<tr>
							<td colspan="3">No categories yet.</td>
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
					await createCategory(id, name)
					newID = ""
					newName = ""
				})
			}}
		>
			<label>
				New category ID
				<input type="text" bind:value={newID} required />
			</label>
			<label>
				Name
				<input type="text" bind:value={newName} required />
			</label>
			<button disabled={data.busy}>Add</button>
		</form>

		<DataTools section="categories" busy={data.busy} {run} />
	{/if}
</section>

<style>
	form {
		margin-top: 1rem;
	}
</style>
