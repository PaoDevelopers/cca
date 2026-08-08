<script lang="ts">
	import ErrorPopup from "@common/ErrorPopup.svelte"
	import Footer from "@common/Footer.svelte"
	import { errorMessage } from "@common/http"
	import { AdminData, setAdminData } from "./lib/data.svelte"
	import CategoriesPage from "./lib/CategoriesPage.svelte"
	import CoursesPage from "./lib/CoursesPage.svelte"
	import GradesPage from "./lib/GradesPage.svelte"
	import PeriodsPage from "./lib/PeriodsPage.svelte"
	import EnrollmentsPage from "./lib/EnrollmentsPage.svelte"
	import StudentsPage from "./lib/StudentsPage.svelte"
	import ViolationDialog from "./lib/ViolationDialog.svelte"

	const pages = {
		"#/": "Home",
		"#/categories": "Categories",
		"#/periods": "Periods",
		"#/grades": "Grades",
		"#/courses": "Courses",
		"#/students": "Students",
		"#/enrollments": "Enrollments",
	} as const
	type Route = keyof typeof pages

	// The hash may carry a query after "?", e.g. "#/enrollments?q=…".
	// q is the filter box's value: the pages bind to it rather than
	// copying it, so landing on a link, clearing the box and clicking
	// the current tab all agree without anything having to be synced.
	function parseHash(): { route: Route; query: string } {
		const hash = window.location.hash
		const cut = hash.indexOf("?")
		const base = cut === -1 ? hash : hash.slice(0, cut)
		const params = new URLSearchParams(
			cut === -1 ? "" : hash.slice(cut + 1),
		)
		return {
			route: base in pages ? (base as Route) : "#/",
			query: params.get("q") ?? "",
		}
	}

	let route = $state<Route>(parseHash().route)
	let query = $state<string>(parseHash().query)

	function onhashchange(): void {
		const parsed = parseHash()
		route = parsed.route
		query = parsed.query
	}

	// One data layer and one event socket for the whole app; the pages
	// read it out of context.
	const data = new AdminData()
	setAdminData(data)

	$effect(() => data.connect())
</script>

<svelte:window {onhashchange} />

<header>
	<h1>CCA administration</h1>
	<!--
		Link-based navigation, not ARIA tabs: each entry changes the
		hash route, so <nav> plus aria-current="page" is the accurate
		markup. The tab look is purely presentational.
	-->
	<nav>
		{#each Object.entries(pages) as [hash, label] (hash)}
			<a href={hash} aria-current={route === hash ? "page" : undefined}>
				{label}
			</a>
		{/each}
	</nav>
</header>

<!--
	A render error would otherwise abort the update and leave the last
	good DOM in place: the tab appears to do nothing and nothing says
	why. The boundary replaces the section instead, so the failure is
	visible and recoverable. It catches rendering and effects only —
	failed requests are ApiErrors and go to the popover below.
-->
<!--
	tabindex so the data layer has somewhere to put the focus when the
	control that started a write no longer exists — a save that closes
	its own editor, a delete that removes its own row. Landing here
	continues the tab order inside the page rather than restarting it
	at the top of the document.
-->
<main tabindex="-1">
	<svelte:boundary>
		{#if data.unauthenticated}
			<p role="alert">
				You are not signed in, or your session has expired.
				<a href="/admin/">Sign in</a>
			</p>
		{:else if route === "#/categories"}
			<CategoriesPage />
		{:else if route === "#/periods"}
			<PeriodsPage />
		{:else if route === "#/grades"}
			<GradesPage />
		{:else if route === "#/courses"}
			<CoursesPage bind:filter={query} />
		{:else if route === "#/students"}
			<StudentsPage bind:filter={query} />
		{:else if route === "#/enrollments"}
			<EnrollmentsPage bind:filter={query} />
		{:else}
			<section>
				<p>Choose a section above.</p>
				<form method="post" action="/admin/logout">
					<button>Sign out</button>
				</form>
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
	message={data.error}
	ondismiss={(): void => {
		data.error = null
	}}
/>

<!--
	Outside the boundary and outside every page: any negotiable write
	can raise it, and the answer has to survive the page it came from.
-->
{#if data.pendingViolations !== null}
	<ViolationDialog
		violations={data.pendingViolations.violations}
		answer={data.pendingViolations.answer}
	/>
{/if}

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

	h1 {
		font-size: 1.2rem;
		font-weight: normal;
		margin: 0;
		padding-bottom: 0.4rem;
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
	 * Tabs are navigation chrome rather than prose links, so they drop
	 * the underline and blue from base.css and read as muted labels
	 * until they are current or hovered.
	 */
	nav a:link,
	nav a:visited {
		padding: 0.4rem 0.75rem;
		border-bottom: 3px solid transparent;
		color: color-mix(in oklab, var(--fg) 65%, var(--bg));
		text-decoration: none;
		white-space: nowrap;
	}

	nav a:hover {
		border-bottom-color: color-mix(in oklab, var(--bg) 60%, var(--fg));
		color: var(--fg);
	}

	nav a[aria-current="page"] {
		border-bottom-color: var(--fg);
		color: var(--fg);
	}

	form {
		margin-top: 1rem;
	}
</style>
