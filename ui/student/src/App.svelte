<script lang="ts">
	// Loads everything once and keeps it fresh over the event
	// websocket.
	//
	// Note what is not loaded: nothing that judges a rule. Whether a
	// course may be taken is asked of the server through
	// /student/api/eligibility, which answers for the whole catalogue
	// in one call. The browser arranges and displays; it does not
	// decide.
	import ErrorPopup from "@common/ErrorPopup.svelte"
	import Footer from "@common/Footer.svelte"
	import { connectEvents } from "@common/events"
	import { AuthError, errorMessage } from "@common/http"
	import {
		drop as dropCourse,
		enroll as enrollInCourse,
		fetchCategories,
		fetchCourses,
		fetchEligibility,
		fetchEnrollments,
		fetchGrades,
		fetchPeriods,
		fetchUser,
		swap as swapIntoCourse,
	} from "@common/studentApi"
	import type {
		Category,
		Course,
		Eligibility,
		Enrollment,
		Grade,
		Period,
		StudentInfo,
		Violation,
	} from "@common/types"
	import CourseFilters from "./lib/CourseFilters.svelte"
	import CourseList from "./lib/CourseList.svelte"
	import HomePage from "./lib/HomePage.svelte"
	import { tick } from "svelte"
	import { coalesce } from "./lib/coalesce"
	import {
		type CourseFilter,
		conflictingEnrollments,
		filterCourses,
		scheduleRows,
		swappable,
		violationsFor,
	} from "./lib/enrollment"

	let user = $state<StudentInfo | null>(null)
	let courses = $state.raw<Course[]>([])
	let periods = $state.raw<Period[]>([])
	let categories = $state.raw<Category[]>([])
	let grades = $state.raw<Grade[]>([])
	let enrollments = $state.raw<Enrollment[]>([])
	let eligibility = $state.raw<Eligibility>({})
	let loading = $state(true)
	let error = $state<string | null>(null)
	let updating = $state<string | null>(null)
	let unauthenticated = $state(false)

	let filter = $state<CourseFilter>({
		search: "",
		categories: [],
		periods: [],
		hideFull: false,
		hideInviteOnly: true,
		hideIncompatible: true,
		hideConflicting: false,
	})

	type Page = "home" | "mine" | "available"
	let page = $state<Page>("home")

	// One preference covers both course lists.
	let view = $state<"cards" | "table">("cards")

	const enrollmentByCourse = $derived(
		new Map(enrollments.map((s): [string, Enrollment] => [s.course_id, s])),
	)

	const selectedCourses = $derived(
		courses.filter((c): boolean => enrollmentByCourse.has(c.id)),
	)

	const availableCourses = $derived(
		courses.filter((c): boolean => !enrollmentByCourse.has(c.id)),
	)

	const gradeInfo = $derived.by((): Grade | null => {
		if (user === null) {
			return null
		}
		const id = user.grade_id
		return grades.find((g): boolean => g.id === id) ?? null
	})

	// The gate on every student write. Derived by the server from the
	// window's two bounds and read from here, never recomputed.
	const windowOpen = $derived(gradeInfo?.is_open ?? false)

	const schedule = $derived(scheduleRows(periods, selectedCourses))

	function violations(course: Course): Violation[] {
		return violationsFor(eligibility, course)
	}

	// Distinct categories across the visible catalogue, for the facet.
	const courseCategories = $derived(
		categories.filter((category): boolean =>
			courses.some((c): boolean => c.category_id === category.id),
		),
	)

	const filteredCourses = $derived(
		filterCourses(availableCourses, filter, violations),
	)

	// An expired session switches to the signed-out view; anything
	// else pops an error.
	function report(err: unknown, fallback: string): void {
		if (err instanceof AuthError) {
			unauthenticated = true
		} else {
			error = errorMessage(err, fallback)
		}
	}

	// Each read applied on its own, and one failure reported rather
	// than allowed to discard the rest.
	//
	// This was a Promise.all with every assignment after the await, so
	// a single rejection threw away six good responses and applied
	// nothing. That is worst exactly where it is most likely: this
	// function is also the reconnect repair, so it runs when everyone
	// reconnects at once, which is when a read is most likely to be
	// refused. One 503 on eligibility left a catalogue frozen at its
	// pre-outage contents for as long as the page stayed open — and
	// the herd that caused the 503 was the same event that was
	// supposed to repair it.
	//
	// The reads are still concurrent; only the all-or-nothing is gone.
	async function load(): Promise<void> {
		const results = await Promise.allSettled([
			fetchUser().then((v): void => {
				user = v
			}),
			fetchCourses().then((v): void => {
				courses = v
			}),
			fetchPeriods().then((v): void => {
				periods = v
			}),
			fetchCategories().then((v): void => {
				categories = v
			}),
			fetchGrades().then((v): void => {
				grades = v
			}),
			fetchEnrollments().then((v): void => {
				enrollments = v
			}),
			fetchEligibility().then((v): void => {
				eligibility = v
			}),
		])

		// One message, from the first failure. Seven copies of "the
		// server is overloaded" says nothing the first one did not, and
		// an expired session has to win over any of them.
		const failed = results.filter(
			(r): r is PromiseRejectedResult => r.status === "rejected",
		)
		const expired = failed.find(
			(r): boolean => r.reason instanceof AuthError,
		)
		if (expired !== undefined) {
			report(expired.reason, "Failed to load")
		} else if (failed[0] !== undefined) {
			report(failed[0].reason, "Failed to load")
		}

		loading = false
	}

	// What just happened, for a screen reader.
	//
	// Nothing else says it. The card changes — a new button, the word
	// "Selected", the count moving — and every one of those changes is
	// silent: the only live region on the page was the error popup, so
	// a student who could not see the screen had no way of learning
	// whether their enrolment had gone through except to navigate back
	// and re-read the card. For twelve hundred teenagers inside a
	// timed window, that is the difference that matters.
	let announcement = $state("")

	// Where the focus goes after a write.
	//
	// Not "back to the button that was pressed": a successful enrolment
	// moves the course out of Available entirely, so that button no
	// longer exists. The list is rebuilt around the gap, and the useful
	// place is where the gap is — the course that took its position, or
	// the one before it if it was last. That is where a sighted student
	// is already looking, and it is the only choice that does not send
	// a keyboard user back to the top of a catalogue of forty courses.
	//
	// Every action button carries its course id, so this is a question
	// about ids rather than about nodes, and survives the whole list
	// being re-rendered.
	function actionButtons(): HTMLElement[] {
		return [...document.querySelectorAll("[data-course-action]")].filter(
			(node): node is HTMLElement => node instanceof HTMLElement,
		)
	}

	// The write is not the last thing that redraws this list. It
	// answers with the new enrollment set, and then the eligibility map
	// and the student's standing arrive separately and redraw it again,
	// each time replacing the buttons and dropping the focus back to
	// <body>. So the intent is held rather than acted on once, and
	// re-applied whenever a redraw has left the focus nowhere.
	//
	// Only when the focus is on <body>: if the student has since put it
	// somewhere themselves, that is theirs and this does not take it.
	let refocus = $state<{ course: string; before: string[] } | null>(null)

	$effect((): void => {
		// Read what redraws the list, so this runs again when it does.
		void enrollments
		void eligibility
		void updating

		if (refocus === null || document.activeElement !== document.body) {
			return
		}

		void applyFocus(refocus.course, refocus.before)
	})

	async function applyFocus(
		courseID: string,
		before: string[],
	): Promise<void> {
		await tick()

		if (document.activeElement !== document.body) {
			return
		}

		const after = actionButtons()
		const survivor = after.find(
			(node): boolean => node.dataset["courseAction"] === courseID,
		)
		if (survivor !== undefined) {
			// Still in this list, with a different label: dropping from
			// Mine, enrolling from Mine, anything on a filtered view
			// that still matches.
			survivor.focus()
			return
		}

		// Gone from this list. Take the position it held, and fall back
		// towards the start, so acting on the last course lands on the
		// new last one rather than nowhere.
		const index = before.indexOf(courseID)
		if (index >= 0 && after.length > 0) {
			const target = after[Math.min(index, after.length - 1)]
			target?.focus()
			return
		}

		// Nothing left to move to — the last course was just dropped,
		// or a filter no longer matches anything. The heading names the
		// list and is where a reader would start again.
		const heading = document.querySelector<HTMLElement>(
			"section > h2[tabindex]",
		)
		heading?.focus()
	}

	// The three student writes share everything but the call itself:
	// each answers with the resulting enrollment set, and each moves
	// what the student may do next, so the eligibility map and their
	// own standing are re-read afterwards.
	async function write(
		course: Course,
		action: () => Promise<Enrollment[]>,
		fallback: string,
		succeeded: string,
	): Promise<void> {
		if (updating !== null) {
			return
		}

		const before = actionButtons().map(
			(node): string => node.dataset["courseAction"] ?? "",
		)

		updating = course.id
		try {
			// The enrollment list comes straight back, so it is never
			// stale. What it implies — eligibility and standing — is
			// re-read through the coalescers, which merge this with
			// the invalidation frame this same write is about to
			// provoke.
			enrollments = await action()
			eligibilityPull.trigger()
			userPull.trigger()
			announcement = `${succeeded} ${course.name}.`
		} catch (err) {
			// Refusals are announced by the error popup, which is
			// already a live region and is assertive because a refusal
			// is not something to mention in passing.
			report(err, fallback)
		} finally {
			updating = null
			refocus = { course: course.id, before }
		}
	}

	function enroll(course: Course): void {
		void write(
			course,
			() => enrollInCourse(course.id),
			"Enrollment failed",
			"Enrolled in",
		)
	}

	function drop(course: Course): void {
		void write(
			course,
			() => dropCourse(course.id),
			"Drop failed",
			"Dropped",
		)
	}

	// One operation, not a drop followed by an enroll: the new course
	// is judged with the replaced ones disregarded, which is the only
	// way to move between two courses that clash.
	function swap(course: Course): void {
		const replacing = conflictingEnrollments(eligibility, course)
		void write(
			course,
			() => swapIntoCourse(course.id, replacing),
			"Swap failed",
			"Swapped into",
		)
	}

	function pull<T>(loader: () => Promise<T>, set: (value: T) => void): void {
		loader()
			.then(set)
			.catch((err: unknown): void => {
				report(err, "Refresh failed")
			})
	}

	// Anything that moves a rule's inputs moves the eligibility map
	// with it, so it is re-read alongside — and coalesced, because a
	// student's own write asks for it twice: once on the write path
	// and once when the server's frame comes back to this session.
	const eligibilityPull = coalesce((): void => {
		pull(fetchEligibility, (e): void => {
			eligibility = e
		})
	})

	const userPull = coalesce((): void => {
		pull(fetchUser, (u): void => {
			user = u
		})
	})

	function pullEligibility(): void {
		eligibilityPull.trigger()
	}

	$effect(() => (): void => {
		eligibilityPull.cancel()
		userPull.cancel()
	})

	$effect(() =>
		connectEvents("/student/api/events", {
			oninvalidate: (resource): void => {
				switch (resource) {
					case "courses":
						pull(fetchCourses, (c): void => {
							courses = c
						})
						pullEligibility()
						break
					case "categories":
						pull(fetchCategories, (c): void => {
							categories = c
						})
						break
					case "periods":
						pull(fetchPeriods, (p): void => {
							periods = p
						})
						pullEligibility()
						break
					case "grades":
						// Includes the window opening or closing: the
						// server wakes at each boundary so an open page
						// repaints then rather than at its next action.
						pull(fetchGrades, (g): void => {
							grades = g
						})
						userPull.trigger()
						break
					case "enrollments":
						pull(fetchEnrollments, (s): void => {
							enrollments = s
						})
						userPull.trigger()
						pullEligibility()
						break
					case "students":
						userPull.trigger()
						pullEligibility()
						break
				}
			},
			oncoursecount: (courseID, currentStudents): void => {
				// Whether this count crossed the capacity line, before
				// the count is replaced.
				//
				// Fullness is a rule, and the rules are the server's:
				// "capacity" is one of the violations the eligibility
				// map carries. The map was never re-read on a count
				// frame, so a course that filled up while a student had
				// the page open kept an enabled Enroll button with no
				// reason beside it, for the rest of the session — and
				// the student found out by having the write refused.
				//
				// Only on the crossing, not on every count. Under a
				// busy window these arrive ten a second, and re-reading
				// the whole map that often would be the more expensive
				// mistake. The coalescer bounds it again on top of
				// that.
				const before = courses.find((c): boolean => c.id === courseID)
				const wasFull =
					before !== undefined &&
					before.current_students >= before.max_students
				const isFull =
					before !== undefined &&
					currentStudents >= before.max_students

				courses = courses.map((c): Course =>
					c.id === courseID
						? { ...c, current_students: currentStudents }
						: c,
				)

				if (wasFull !== isFull) {
					pullEligibility()
				}
			},
			// Whatever changed while the socket was down was never
			// delivered, and there is no way to find out what it was.
			onreconnect: (): void => {
				void load()
			},
			ongiveup: (reason: string): void => {
				error = reason
			},
		}),
	)

	void load()
</script>

<!--
	Polite, so it waits for a pause rather than cutting across whatever
	the student is reading, and outside <main> so that re-rendering a
	page cannot take it away mid-announcement.
-->
<p aria-live="polite" class="for-screen-readers">{announcement}</p>

<header>
	<div class="title">
		<h1>CCA enrollment</h1>
		<p class="support">
			(<a
				href="mailto:sj-cca@ykpaoschool.cn?cc=runxiyu@umich.edu,me@runxiyu.org,s23321@stu.ykpaoschool.cn"
			>
				email
			</a>,
			<a href="https://webirc.runxiyu.org/kiwiirc/#chat">chat</a>)
		</p>
	</div>
	<!--
		Buttons rather than links: these switch in-page state and have
		no URL of their own. aria-current marks the active one; the tab
		look is purely presentational. The strip lives in the header so
		it shares a row with the title, but stays hidden until there is
		something to navigate.
	-->
	{#if !unauthenticated && !loading}
		<nav>
			<button
				aria-current={page === "home" ? "page" : undefined}
				onclick={(): void => {
					page = "home"
				}}
			>
				Home
			</button>
			<button
				aria-current={page === "mine" ? "page" : undefined}
				onclick={(): void => {
					page = "mine"
					refocus = null
				}}
			>
				Your courses ({selectedCourses.length})
			</button>
			<button
				aria-current={page === "available" ? "page" : undefined}
				onclick={(): void => {
					page = "available"
					refocus = null
				}}
			>
				Available courses ({availableCourses.length})
			</button>
		</nav>
	{/if}
</header>

<!--
	A render error would otherwise abort the update and leave the last
	good DOM in place: the tab appears to do nothing and nothing says
	why. The boundary replaces the section instead, so the failure is
	visible and recoverable. It catches rendering and effects only —
	failed requests are ApiErrors and go to the popover below.
-->
<main>
	<svelte:boundary>
		{#if unauthenticated}
			<p role="alert">
				You are not signed in, or your session has expired.
				<a href="/student/">Sign in</a>
			</p>
		{:else if loading}
			<p>Loading&hellip;</p>
		{:else if page === "home"}
			<HomePage {user} grade={gradeInfo} {categories} {schedule} />
		{:else if page === "mine"}
			<section aria-labelledby="mine-heading">
				<!--
					h1 is the page title; without this the list of
					courses under it was an h3 with no h2 above it, so
					heading navigation skipped a level and nothing named
					the list. tabindex because it is also where the
					focus lands when a write empties the list and there
					is no course left to move to.
				-->
				<h2 id="mine-heading" tabindex="-1">Your courses</h2>
				<CourseList
					courses={selectedCourses}
					enrollment={(course: Course): Enrollment | null =>
						enrollmentByCourse.get(course.id) ?? null}
					violations={(): Violation[] => []}
					{windowOpen}
					empty="You have not selected any courses."
					updating={updating !== null}
					{view}
					onview={(v: "cards" | "table"): void => {
						view = v
					}}
					onenroll={enroll}
					ondrop={drop}
				/>
			</section>
		{:else if page === "available"}
			<section aria-labelledby="available-heading">
				<h2 id="available-heading" tabindex="-1">Available courses</h2>
				<CourseFilters
					bind:filter
					categories={courseCategories}
					{periods}
				/>
				<CourseList
					courses={filteredCourses}
					enrollment={(): null => null}
					{violations}
					{windowOpen}
					empty="No courses match your search."
					updating={updating !== null}
					{view}
					onview={(v: "cards" | "table"): void => {
						view = v
					}}
					onenroll={enroll}
					ondrop={drop}
					canSwap={(course: Course): boolean =>
						swappable(eligibility, course, enrollments)}
					onswap={(course: Course): void => {
						// Guarded again here: the button is only shown
						// where this holds, but the handler is public.
						if (swappable(eligibility, course, enrollments)) {
							swap(course)
						}
					}}
				/>
			</section>
		{/if}

		{#snippet failed(err: unknown, reset: () => void)}
			<p role="alert">
				This section could not be displayed: {errorMessage(
					err,
					"unexpected error",
				)}
				<button
					onclick={(): void => {
						reset()
					}}
				>
					Try again
				</button>
			</p>
		{/snippet}
	</svelte:boundary>
</main>

<Footer />

<ErrorPopup
	message={error}
	ondismiss={(): void => {
		error = null
	}}
/>

<style>
	/*
	 * Title on the left, tabs flush right, both sitting on the header
	 * rule. Narrow viewports wrap the tabs onto their own line, where
	 * space-between leaves them left-aligned under the title.
	 */
	header {
		display: flex;
		flex-wrap: wrap;
		justify-content: space-between;
		align-items: end;
		gap: 0 1rem;
		border-bottom: 1px solid color-mix(in oklab, var(--bg) 80%, var(--fg));
		margin-bottom: 1rem;
	}

	.title {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		padding-bottom: 0.4rem;
	}

	h1 {
		font-size: 1.2rem;
		font-weight: normal;
		margin: 0;
	}

	.support {
		margin: 0;
	}

	nav {
		display: flex;
		gap: 0.25rem;
		/* Straddle the header rule so the active tab sits on it. */
		margin-bottom: -1px;
		min-width: 0;
		overflow-x: auto;
		overflow-y: hidden;
	}

	/*
	 * Native button chrome is stripped so tabs read as labels, muted
	 * until current or hovered. The underline is always 3px and only
	 * changes color, so switching tabs shifts nothing; -1px pulls it
	 * down onto the strip's rule.
	 */
	nav button {
		padding: 0.4rem 0.75rem;
		border: none;
		border-bottom: 3px solid transparent;
		margin-bottom: -1px;
		background: none;
		font: inherit;
		color: color-mix(in oklab, var(--fg) 65%, var(--bg));
		white-space: nowrap;
		cursor: pointer;
	}

	nav button:hover {
		border-bottom-color: color-mix(in oklab, var(--bg) 60%, var(--fg));
		color: var(--fg);
	}

	nav button[aria-current="page"] {
		border-bottom-color: var(--fg);
		color: var(--fg);
	}
</style>
