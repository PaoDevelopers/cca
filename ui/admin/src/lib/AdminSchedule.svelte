<script lang="ts">
	// One student's week, with the details an administrator wants.
	import type { Course, Enrollment } from "@common/types"
	import { policyLabel } from "./enrollments"

	interface Props {
		studentID: string
		periods: string[]
		courses: Course[]
		enrollments: Enrollment[]
	}

	const { studentID, periods, courses, enrollments }: Props = $props()

	const courseByID = $derived(
		new Map(courses.map((c): [string, Course] => [c.id, c])),
	)

	const mine = $derived(
		enrollments.filter((e): boolean => e.student_id === studentID),
	)

	interface Entry {
		id: string
		name: string
		teacher: string
		location: string
		category: string
		policy: string
	}

	function entry(enrollment: Enrollment): Entry {
		const course = courseByID.get(enrollment.course_id)
		return {
			id: enrollment.course_id,
			name: enrollment.course_name,
			teacher: course?.teacher ?? "",
			location: course?.location ?? "",
			category: course?.category_id ?? "",
			policy: policyLabel(enrollment),
		}
	}

	interface Row {
		// Identifies the row for keying; period IDs and course IDs are
		// drawn from the same namespace as each other's labels, so the
		// prefix keeps a period called "Unscheduled" distinct.
		key: string
		label: string
		entries: Entry[]
	}

	const rows = $derived.by((): Row[] => {
		const out = periods.map((period): Row => ({
			key: `period:${period}`,
			label: period,
			entries: mine
				.filter(
					(e): boolean =>
						courseByID
							.get(e.course_id)
							?.period_ids.includes(period) ?? false,
				)
				.map(entry),
		}))
		// One row per unscheduled course rather than one row listing
		// them all: they share no period, so nothing is said by
		// putting them on the same line.
		for (const enrollment of mine) {
			const course = courseByID.get(enrollment.course_id)
			if (course === undefined || course.period_ids.length === 0) {
				out.push({
					key: `unscheduled:${enrollment.course_id}`,
					label: "Unscheduled",
					entries: [entry(enrollment)],
				})
			}
		}
		return out
	})

	function joined(row: Row, field: keyof Entry): string {
		return row.entries.map((e): string => e[field]).join(", ")
	}
</script>

<div class="scrolls">
	<table>
		<thead>
			<tr>
				<th scope="col">Period</th>
				<th scope="col">ID</th>
				<th scope="col">Title</th>
				<th scope="col">Teacher</th>
				<th scope="col">Location</th>
				<th scope="col">Category</th>
				<th scope="col">Policy</th>
			</tr>
		</thead>
		<tbody>
			{#each rows as row (row.key)}
				{@const empty = row.entries.length === 0 ? "Empty" : undefined}
				<tr>
					<th scope="row">{row.label}</th>
					<td aria-label={empty}>
						{#each row.entries as e, position (e.id)}
							{#if position > 0},{" "}{/if}
							<code>{e.id}</code>
						{/each}
					</td>
					<td aria-label={empty}>{joined(row, "name")}</td>
					<td aria-label={empty}>{joined(row, "teacher")}</td>
					<td aria-label={empty}>{joined(row, "location")}</td>
					<td aria-label={empty}>{joined(row, "category")}</td>
					<td aria-label={empty}>{joined(row, "policy")}</td>
				</tr>
			{:else}
				<tr>
					<td colspan="7">No periods defined.</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>
