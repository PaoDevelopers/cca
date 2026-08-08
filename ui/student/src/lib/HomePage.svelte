<script lang="ts">
	// The landing view: identity, requirements, and the timetable.
	// Every number here is read from the server, not derived in the
	// browser: budget spent, categories spanned, and each requirement
	// with whether it is met all come from /student/api/user_info. The
	// rules have one definition, and this is a view of it.
	import type { Category, Grade, StudentInfo } from "@common/types"
	import type { ScheduleRow } from "./enrollment"

	interface Props {
		user: StudentInfo | null
		grade: Grade | null
		categories: Category[]
		schedule: ScheduleRow[]
	}

	const { user, grade, categories, schedule }: Props = $props()

	// Requirements name category ids; the labels come from the
	// categories list, which is the consumer's job to map.
	const categoryName = $derived(
		new Map(categories.map((c): [string, string] => [c.id, c.name])),
	)

	function requirementLabel(id: number): string {
		const requirement = grade?.requirements.find(
			(r): boolean => r.id === id,
		)
		if (requirement === undefined) {
			return ""
		}
		return requirement.category_ids
			.map((c): string => categoryName.get(c) ?? c)
			.join(" / ")
	}

	const allMet = $derived(
		user !== null &&
			user.requirements.every((r): boolean => r.met) &&
			user.distinct_categories_used >= user.min_distinct_categories,
	)
</script>

{#if user !== null}
	<section>
		<h2>Your account</h2>
		<dl class="grid">
			<dt>Name</dt>
			<dd>{user.name}</dd>
			<dt>Grade</dt>
			<dd>{grade?.name ?? user.grade_id}</dd>
			<dt>ID</dt>
			<dd>{user.id}</dd>
		</dl>
		<form method="post" action="/student/logout">
			<button>Sign out</button>
		</form>
	</section>
{/if}

{#if grade !== null && user !== null}
	<section>
		<h2>Status</h2>
		<!--
			Openness is the server's answer, derived from the window's
			two bounds. Nothing here recomputes it from the clock: the
			page and the write functions must agree, or a student is
			offered a button that cannot work.
		-->
		{#if grade.is_open}
			<p>
				Enrollment is <strong>open</strong> for your grade.
				{#if grade.closes_at !== null}
					It closes on {new Date(grade.closes_at).toLocaleString()}.
				{/if}
			</p>
		{:else}
			<p>
				Enrollment is <strong>closed</strong> for your grade. You may
				not make any changes.
				{#if grade.opens_at !== null && new Date(grade.opens_at) > new Date()}
					It opens on {new Date(grade.opens_at).toLocaleString()}.
				{/if}
			</p>
		{/if}
		<p>
			Your selections occupy
			<strong>{user.budgeted_periods_used}</strong>
			{user.budgeted_periods_used === 1
				? "period"
				: "periods"}{#if user.max_budgeted_periods !== null}, out of a
				maximum of
				<strong>{user.max_budgeted_periods}</strong>{/if}.
		</p>
		{#if user.min_distinct_categories > 0}
			<p>
				Your enrollments span
				<strong>{user.distinct_categories_used}</strong>
				{user.distinct_categories_used === 1
					? "category"
					: "categories"}; at least
				<strong>{user.min_distinct_categories}</strong>
				{user.min_distinct_categories === 1 ? "is" : "are"} required.
			</p>
		{/if}
		{#if user.requirements.length > 0 || user.min_distinct_categories > 0}
			{#if allMet}
				<p>You have satisfied your requirements.</p>
			{:else}
				<p>
					You have <strong>not</strong> satisfied your requirements.
				</p>
			{/if}
		{/if}
	</section>
{/if}

{#if user !== null && user.requirements.length > 0}
	<section>
		<h2>Requirements</h2>
		<div class="scrolls">
			<table>
				<thead>
					<tr>
						<th scope="col" rowspan="2">Categories</th>
						<th scope="colgroup" colspan="2">Periods</th>
						<th scope="col" rowspan="2">Status</th>
					</tr>
					<tr>
						<th scope="col">Enrolled</th>
						<th scope="col">Required</th>
					</tr>
				</thead>
				<tbody>
					{#each user.requirements as req (req.id)}
						<tr>
							<th scope="row">{requirementLabel(req.id)}</th>
							<td>{req.satisfied_periods}</td>
							<td>{req.min_period_count}</td>
							<td>{req.met ? "OK" : "Unsatisfied"}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>
{/if}

<section>
	<h2>Your schedule</h2>
	<div class="scrolls">
		<table>
			<thead>
				<tr>
					<th scope="col">Period</th>
					<th scope="col">ID</th>
					<th scope="col">Title</th>
					<th scope="col">Location</th>
					<th scope="col">Categories</th>
				</tr>
			</thead>
			<tbody>
				{#each schedule as row (row.key)}
					{@const empty =
						row.courses.length === 0 ? "Empty" : undefined}
					<tr>
						<th scope="row">{row.label}</th>
						<td aria-label={empty}>
							{#each row.courses as course, position (course.id)}
								{#if position > 0},{" "}{/if}
								<code>{course.id}</code>
							{/each}
						</td>
						<td aria-label={empty}>
							{row.courses.map((c): string => c.name).join(", ")}
						</td>
						<td aria-label={empty}>
							{row.courses
								.map((c): string => c.location)
								.join(", ")}
						</td>
						<td aria-label={empty}>
							{row.courses
								.map((c): string => c.category_id)
								.join(", ")}
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5">Nothing scheduled yet.</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
</section>
