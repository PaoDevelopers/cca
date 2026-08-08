<script lang="ts">
	import {
		closeAllWindows,
		createGrade,
		setGradeOrder,
	} from "@common/adminApi"
	import DataTools from "./DataTools.svelte"
	import { getAdminData } from "./data.svelte"
	import GradeCard from "./GradeCard.svelte"
	import { reordered } from "./ordering"

	const data = getAdminData()
	data.want("grades", "categories")

	let newID = $state("")
	let newName = $state("")

	function run(action: () => Promise<void>): Promise<boolean> {
		return data.run(action, "grades")
	}

	// The budget cap is the one grade setting that can invalidate an
	// enrollment, so it is the one that goes through the accept
	// protocol. Everything else is plain DML.
	function runAccepting(
		action: (accept: string[]) => Promise<void>,
	): Promise<boolean> {
		return data.runAccepting(action, "grades", "students")
	}

	function move(id: string, direction: "up" | "down"): void {
		const ids = data.grades.map((g): string => g.id)
		void run(() => setGradeOrder(reordered(ids, id, direction)))
	}
</script>

<section aria-labelledby="grades-heading">
	<!--
		The page had no heading below the h1 at all, so heading
		navigation went straight from "CCA administration" to nothing and
		the section was an unlabelled region. tabindex because it is also
		where the data layer puts the focus when the control that started
		a write no longer exists.
	-->
	<h2 id="grades-heading" tabindex="-1">Grades</h2>

	{#if !data.ready("grades", "categories")}
		<p>Loading&hellip;</p>
	{:else}
		<p>
			<button
				type="button"
				disabled={data.busy ||
					!data.grades.some((g): boolean => g.is_open)}
				onclick={(): void => {
					void run(() => closeAllWindows())
				}}
			>
				Close every open window
			</button>
			<span class="note">
				Ends enrollment everywhere at once. Schedules are kept: a grade
				that has not opened yet is left as it is.
			</span>
		</p>

		<div class="cards wide">
			{#each data.grades as grade (grade.id)}
				<GradeCard
					{grade}
					categories={data.categories}
					busy={data.busy}
					{run}
					{runAccepting}
					move={(direction): void => {
						move(grade.id, direction)
					}}
				/>
			{:else}
				<p>No grades yet.</p>
			{/each}
		</div>

		<form
			onsubmit={(event): void => {
				event.preventDefault()
				const id = newID.trim()
				const name = newName.trim()
				if (id === "" || name === "") {
					return
				}
				// Created closed, with no cap: opening a window and
				// setting a budget are separate, deliberate acts.
				void run(async (): Promise<void> => {
					await createGrade(id, name, null, 0)
					newID = ""
					newName = ""
				})
			}}
		>
			<label>
				New grade ID
				<input type="text" bind:value={newID} required />
			</label>
			<label>
				Name
				<input type="text" bind:value={newName} required />
			</label>
			<button disabled={data.busy}>Add</button>
		</form>

		<DataTools section="grades" busy={data.busy} {run} />
	{/if}
</section>

<style>
	form {
		margin-top: 1rem;
	}
</style>
