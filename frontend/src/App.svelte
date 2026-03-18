<script lang="ts">
	import { onDestroy, onMount } from "svelte"
	import type { Category, Choice, Course, Period, Student } from "./types"
	import {
		fetchCategories,
		fetchCourses,
		fetchPeriods,
		fetchSelections,
		fetchUser,
		mutateSelection,
	} from "./lib/api"

	type Page = "select" | "review"
	type ViewMode = "cards" | "table"
	type ToastTone = "error" | "success"
	type WSState = "connecting" | "connected" | "retrying" | "stopped"
	type SelectionFilter = "" | "joined"

	interface Toast {
		id: number
		message: string
		tone: ToastTone
	}

	const TOAST_DURATION_MS = 4200
	const MAX_RECONNECT_DELAY_MS = 10_000
	const BASE_RECONNECT_DELAY_MS = 2_000
	const RECONNECT_TIMEOUT_MS = 60_000

	let page = $state<Page>("select")
	let viewMode = $state<ViewMode>("cards")
	let user = $state<Student | null>(null)
	let courses = $state<Course[]>([])
	let periods = $state<Period[]>([])
	let categories = $state<Category[]>([])
	let selections = $state<Choice[]>([])
	let loading = $state(true)
	let refreshing = $state(false)
	let savingCourseId = $state<string | null>(null)
	let search = $state("")
	let periodFilter = $state("")
	let categoryFilter = $state("")
	let selectionFilter = $state<SelectionFilter>("")
	let toasts = $state<Toast[]>([])
	let toastSeed = 0
	let ws: WebSocket | null = null
	let wsState = $state<WSState>("connecting")
	let wsRetryTimer: ReturnType<typeof setTimeout> | null = null
	let wsDisconnectedAt: number | null = null
	let confirmModal = $state<{
		course: Course
		existingChoice: Choice | undefined
		periodChoice: Choice | undefined
	} | null>(null)
	let confirmText = $state("")

	const periodOptions = $derived.by((): string[] => {
		if (periods.length > 0) {
			return periods.map((period): string => period.id)
		}
		const derived = new Set<string>()
		for (const course of courses) {
			derived.add(course.period)
		}
		for (const selection of selections) {
			derived.add(selection.period)
		}
		return Array.from(derived)
	})

	const filteredCourses = $derived.by((): Course[] => {
		const needle = search.trim().toLowerCase()
		return courses.filter((course): boolean =>
			matchesCourse(course, needle),
		)
	})

	const reviewRows = $derived.by(
		(): Array<{ period: string; selection: Choice | undefined }> => {
			const ids =
				periods.length > 0
					? periods.map((period): string => period.id)
					: ((): string[] => {
							const derived = new Set<string>()
							for (const course of courses) {
								derived.add(course.period)
							}
							for (const selection of selections) {
								derived.add(selection.period)
							}
							return Array.from(derived)
						})()
			return ids.map(
				(id): { period: string; selection: Choice | undefined } => ({
					period: id,
					selection: selectionForPeriod(id),
				}),
			)
		},
	)

	const courseMap = $derived.by((): Record<string, Course> => {
		const map: Record<string, Course> = {}
		for (const course of courses) {
			map[course.id] = course
		}
		return map
	})

	const wsLabelText = $derived.by((): string => {
		if (wsState === "connected") {
			return ""
		}
		if (wsState === "connecting") {
			return "Connecting..."
		}
		if (wsState === "retrying") {
			return "Disconnected (retrying)"
		}
		return "Disconnected; please refresh"
	})

	function matchesCourse(course: Course, needle: string): boolean {
		if (periodFilter && course.period !== periodFilter) {
			return false
		}
		if (categoryFilter && course.category_id !== categoryFilter) {
			return false
		}
		if (selectionFilter === "joined" && !selectionForCourse(course.id)) {
			return false
		}
		if (needle.length === 0) {
			return true
		}
		return (
			course.name.toLowerCase().includes(needle) ||
			course.description.toLowerCase().includes(needle) ||
			course.teacher.toLowerCase().includes(needle) ||
			course.location.toLowerCase().includes(needle) ||
			course.id.toLowerCase().includes(needle)
		)
	}

	onMount(async (): Promise<void> => {
		await loadAll()
		connectWebSocket()
	})

	onDestroy(() => {
		clearRetryTimer()
		if (ws) {
			ws.close()
			ws = null
		}
	})

	function addToast(message: string, tone: ToastTone = "error"): void {
		const id = ++toastSeed
		toasts = [...toasts, { id, message, tone }]
		setTimeout(() => {
			toasts = toasts.filter((toast) => toast.id !== id)
		}, TOAST_DURATION_MS)
	}

	function selectionForPeriod(periodId: string): Choice | undefined {
		return selections.find((selection) => selection.period === periodId)
	}

	function selectionForCourse(courseId: string): Choice | undefined {
		return selections.find((selection) => selection.course_id === courseId)
	}

	function seatsOpen(course: Course): number {
		return Math.max(0, course.max_students - course.current_students)
	}

	function isFull(course: Course): boolean {
		return seatsOpen(course) === 0
	}

	function labelForSelection(choice: Choice | undefined): string {
		if (!choice) {
			return ""
		}
		switch (choice.selection_type) {
			case "invite":
				return "Invited"
			case "force":
				return "Assigned"
			default:
				return "Selected"
		}
	}

	function isActionMuted(course: Course): boolean {
		const existingChoice = selectionForCourse(course.id)
		const periodChoice = selectionForPeriod(course.period)

		if (
			existingChoice?.selection_type === "force" ||
			periodChoice?.selection_type === "force"
		) {
			return true
		}

		if (
			!existingChoice &&
			(course.membership === "invite_only" || isFull(course))
		) {
			return true
		}

		return false
	}

	async function loadAll(options?: { silent?: boolean }): Promise<void> {
		const silent = options?.silent === true
		if (silent) {
			refreshing = true
		} else {
			loading = true
		}
		const errors: string[] = []
		try {
			await Promise.all([
				(async (): Promise<void> => {
					try {
						const student = await fetchUser()
						user = student
					} catch (error) {
						const message =
							error instanceof Error
								? error.message
								: "Unable to load user information."
						errors.push(message)
					}
				})(),
				(async (): Promise<void> => {
					try {
						const courseList = await fetchCourses()
						courses = courseList
					} catch (error) {
						const message =
							error instanceof Error
								? error.message
								: "Unable to load courses."
						errors.push(message)
					}
				})(),
				(async (): Promise<void> => {
					try {
						const periodList = await fetchPeriods()
						periods = periodList
					} catch (error) {
						const message =
							error instanceof Error
								? error.message
								: "Unable to load periods."
						errors.push(message)
					}
				})(),
				(async (): Promise<void> => {
					try {
						const categoryList = await fetchCategories()
						categories = categoryList
					} catch (error) {
						const message =
							error instanceof Error
								? error.message
								: "Unable to load categories."
						errors.push(message)
					}
				})(),
				(async (): Promise<void> => {
					try {
						const choiceList = await fetchSelections()
						selections = choiceList
					} catch (error) {
						const message =
							error instanceof Error
								? error.message
								: "Unable to load selections."
						errors.push(message)
					}
				})(),
			])
			if (errors.length > 0) {
				addToast(errors.join(" "), "error")
			}
		} catch (error) {
			const message =
				error instanceof Error ? error.message : "Failed to load data."
			addToast(message, "error")
		} finally {
			if (silent) {
				refreshing = false
			} else {
				loading = false
			}
		}
	}

	async function refreshLists(): Promise<void> {
		await loadAll({ silent: true })
	}

	function requestUpdateSelection(course: Course): void {
		if (savingCourseId !== null) {
			console.log("requestUpdateSelection blocked saving in progress")
			return
		}

		const existingChoice = selectionForCourse(course.id)
		const periodChoice = selectionForPeriod(course.period)

		if (
			existingChoice ||
			(periodChoice && periodChoice.course_id !== course.id)
		) {
			confirmModal = { course, existingChoice, periodChoice }
			confirmText = ""
		} else {
			updateSelection(course).catch((error) => {
				console.error("updateSelection error:", error)
			})
		}
	}

	async function updateSelection(course: Course): Promise<void> {
		if (savingCourseId !== null) {
			console.log("updateSelection blocked saving in progress")
			return
		}

		const existingChoice = selectionForCourse(course.id)
		const periodChoice = selectionForPeriod(course.period)

		savingCourseId = course.id
		try {
			if (periodChoice && periodChoice.course_id !== course.id) {
				selections = await mutateSelection(
					"DELETE",
					periodChoice.course_id,
				)
			}

			const method = existingChoice ? "DELETE" : "PUT"
			selections = await mutateSelection(method, course.id)

			addToast(
				existingChoice ? "Selection removed." : "Selection saved.",
				"success",
			)
		} catch (error) {
			const message =
				error instanceof Error
					? error.message
					: "Unable to update selection."
			addToast(message, "error")
		} finally {
			savingCourseId = null
		}
	}

	function confirmUpdate(): void {
		if (!confirmModal) return

		const needsInviteConfirm =
			Boolean(confirmModal.existingChoice) &&
			courseMap[confirmModal.existingChoice.course_id]?.membership ===
				"invite_only"

		if (needsInviteConfirm === true && confirmText !== "I am sure") {
			return
		}

		updateSelection(confirmModal.course).catch((error) => {
			console.error("confirmUpdate error:", error)
		})
		confirmModal = null
		confirmText = ""
	}

	function cancelUpdate(): void {
		confirmModal = null
		confirmText = ""
	}

	function buildWSUrl(): string {
		const protocol = window.location.protocol === "https:" ? "wss" : "ws"
		return `${protocol}://${window.location.host}/student/api/events`
	}

	function clearRetryTimer(): void {
		if (wsRetryTimer !== null) {
			clearTimeout(wsRetryTimer)
			wsRetryTimer = null
		}
	}

	function scheduleReconnect(): void {
		clearRetryTimer()
		wsDisconnectedAt ??= Date.now()
		const elapsed = Date.now() - wsDisconnectedAt
		if (elapsed >= RECONNECT_TIMEOUT_MS) {
			wsState = "stopped"
			return
		}
		wsState = "retrying"
		const delay = Math.min(
			MAX_RECONNECT_DELAY_MS,
			BASE_RECONNECT_DELAY_MS + elapsed / 4,
		)
		wsRetryTimer = setTimeout(() => {
			connectWebSocket()
		}, delay)
	}

	function handleMessage(data: string): void {
		if (
			data === "invalidate_selections" ||
			data.startsWith("course_count_update")
		) {
			loadAll({ silent: true }).catch((error) => {
				console.error("handleMessage loadAll error:", error)
			})
		}
	}

	function connectWebSocket(manual = false): void {
		clearRetryTimer()
		if (ws) {
			ws.close()
			ws = null
		}
		if (manual) {
			wsDisconnectedAt = Date.now()
		}
		wsState = "connecting"
		try {
			const socket = new WebSocket(buildWSUrl())
			ws = socket
			socket.onopen = (): void => {
				wsState = "connected"
				wsDisconnectedAt = null
				clearRetryTimer()
				loadAll({ silent: true }).catch((error) => {
					console.error("onopen loadAll error:", error)
				})
			}
			socket.onmessage = (event): void => {
				handleMessage(String(event.data))
			}
			socket.onclose = (): void => {
				ws = null
				if (wsState !== "stopped") {
					scheduleReconnect()
				}
			}
			socket.onerror = (): void => {
				socket.close()
			}
		} catch {
			scheduleReconnect()
		}
	}
</script>

<svelte:head>
	<title>CCA Student</title>
</svelte:head>

<div class="app-shell">
	<header class="top-bar">
		<div class="maintitle">
			<div class="stack">
				<h1>YKPS CCAs</h1>
			</div>
		</div>
		<div class="spacer"></div>
		{#if wsLabelText}
			<button
				type="button"
				class={`ws-status ${wsState !== "connected" ? "retrying" : ""}`}
				onclick={(): void => connectWebSocket(true)}
			>
				{wsLabelText}
			</button>
		{/if}
		{#if user}
			<div class="row">
				<div class="badge accent">{user.name}</div>
				<div class="badge subtle">Grade {user.grade}</div>
				<div class="badge subtle">ID {user.id}</div>
			</div>
		{/if}
		<div class="page-tabs" role="tablist" aria-label="Pages">
			<button
				role="tab"
				class={`page-tab ${page === "select" ? "active" : ""}`}
				aria-selected={page === "select"}
				onclick={(): void => (page = "select")}
			>
				Select
			</button>
			<button
				role="tab"
				class={`page-tab ${page === "review" ? "active" : ""}`}
				aria-selected={page === "review"}
				onclick={(): void => (page = "review")}
			>
				Review
			</button>
		</div>
	</header>

	<main>
		{#if loading}
			<div>Loading student data...</div>
		{:else if page === "select"}
			<div class="toolbar">
				<div class="section-actions">
					<button class="ghost" onclick={(): void => refreshLists()}>
						{refreshing ? "Refreshing..." : "Refresh all"}
					</button>
				</div>
			</div>

			<div class="filters">
				<div class="field">
					<label for="period-filter">Period</label>
					<select id="period-filter" bind:value={periodFilter}>
						<option value="">All periods</option>
						{#each periodOptions as option}
							<option value={option}>{option}</option>
						{/each}
					</select>
				</div>
				<div class="field">
					<label for="category-filter">Category</label>
					<select id="category-filter" bind:value={categoryFilter}>
						<option value="">All categories</option>
						{#each categories as category}
							<option value={category.id}>{category.id}</option>
						{/each}
					</select>
				</div>
				<div class="field">
					<label for="selection-filter">Selection status</label>
					<select id="selection-filter" bind:value={selectionFilter}>
						<option value="">All selection statuses</option>
						<option value="joined">Only already joined</option>
					</select>
				</div>
				<div class="field">
					<label for="search-filter">Search</label>
					<input
						id="search-filter"
						type="text"
						placeholder="Search by name, teacher, or location"
						bind:value={search}
					/>
				</div>
				<div class="view-toggle">
					<button
						class={`pill ${viewMode === "cards" ? "active" : ""}`}
						onclick={(): void => (viewMode = "cards")}
						aria-pressed={viewMode === "cards"}
					>
						Cards
					</button>
					<button
						class={`pill ${viewMode === "table" ? "active" : ""}`}
						onclick={(): void => (viewMode = "table")}
						aria-pressed={viewMode === "table"}
					>
						Table
					</button>
				</div>
			</div>

			<div class="status-row">
				<span class="badge subtle"
					>{filteredCourses.length} courses shown</span
				>
				{#if periodFilter}
					<span class="badge subtle">Period {periodFilter}</span>
				{/if}
				{#if categoryFilter}
					<span class="badge subtle">Category {categoryFilter}</span>
				{/if}
				{#if selectionFilter === "joined"}
					<span class="badge subtle">Already joined</span>
				{/if}
				{#if search.trim()}
					<span class="badge subtle">Matching “{search.trim()}”</span>
				{/if}
			</div>

			{#if filteredCourses.length === 0}
				<div class="muted">No courses match your filters.</div>
			{:else if viewMode === "cards"}
				<div class="course-grid" aria-live="polite">
					{#each filteredCourses as course}
						<article class="course-card">
							<header>
								<div class="stack">
									<h3>{course.name}</h3>
									<div class="meta-row">
										<span class="badge accent"
											>Period {course.period}</span
										>
										<span class="badge subtle"
											>{course.category_id}</span
										>
										{#if course.membership === "invite_only"}
											<span class="badge danger"
												>Invite only</span
											>
										{/if}
									</div>
								</div>
								<span
									class={`badge ${isFull(course) ? "danger" : "success"}`}
								>
									{isFull(course)
										? "Full"
										: `${seatsOpen(course)} open`}
								</span>
							</header>
							<p>{course.description}</p>
							<div class="meta-row">
								<span>Teacher: {course.teacher}</span>
								<span>Location: {course.location}</span>
							</div>
							<div class="meta-row">
								<span>Course ID: {course.id}</span>
								{#if selectionForCourse(course.id)}
									<span class="badge subtle">
										{labelForSelection(
											selectionForCourse(course.id),
										)}
									</span>
								{/if}
							</div>
							<div class="meta-row">
								<button
									class={`${selectionForCourse(course.id) ? "ghost" : "primary"} ${isActionMuted(course) ? "muted-action" : ""}`}
									aria-label="Toggle course selection"
									onclick={(): void =>
										requestUpdateSelection(course)}
									disabled={savingCourseId === course.id}
								>
									{#if savingCourseId === course.id}
										Saving...
									{:else if selectionForCourse(course.id)}
										{selectionForCourse(course.id)
											?.selection_type === "force"
											? "Locked"
											: "Remove"}
									{:else}
										Select
									{/if}
								</button>
								{#if selectionForPeriod(course.period) && selectionForPeriod(course.period)?.course_id !== course.id}
									<span class="selection-note">
										Selecting replaces current {course.period}
										choice.
									</span>
								{/if}
							</div>
						</article>
					{/each}
				</div>
			{:else}
				<div class="table-wrapper" aria-live="polite">
					<table>
						<thead>
							<tr>
								<th>Course</th>
								<th>Period</th>
								<th>Category</th>
								<th>Teacher</th>
								<th>Location</th>
								<th>Seats</th>
								<th>Membership</th>
								<th>Selection</th>
							</tr>
						</thead>
						<tbody>
							{#each filteredCourses as course}
								<tr>
									<td>
										<div>{course.name}</div>
										<div class="muted">{course.id}</div>
									</td>
									<td>{course.period}</td>
									<td>{course.category_id}</td>
									<td>{course.teacher}</td>
									<td>{course.location}</td>
									<td>
										{course.current_students}/{course.max_students}
									</td>
									<td
										>{course.membership === "invite_only"
											? "Invite only"
											: "Free"}</td
									>
									<td>
										<div class="chip-row">
											{#if selectionForCourse(course.id)}
												<span class="badge subtle">
													{labelForSelection(
														selectionForCourse(
															course.id,
														),
													)}
												</span>
											{/if}
											{#if isFull(course) && !selectionForCourse(course.id)}
												<span class="badge danger"
													>Full</span
												>
											{/if}
										</div>
										<button
											class="ghost"
											onclick={(): void =>
												requestUpdateSelection(course)}
											class:muted-action={isActionMuted(
												course,
											)}
											disabled={savingCourseId ===
												course.id}
										>
											{#if savingCourseId === course.id}
												Saving...
											{:else if selectionForCourse(course.id)}
												{selectionForCourse(course.id)
													?.selection_type === "force"
													? "Locked"
													: "Remove"}
											{:else}
												Select
											{/if}
										</button>
										{#if selectionForPeriod(course.period) && selectionForPeriod(course.period)?.course_id !== course.id}
											<div class="selection-note">
												Selecting replaces current {course.period}
												choice.
											</div>
										{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		{:else if reviewRows.length === 0}
			<div class="muted">No periods available.</div>
		{:else}
			<div
				class="review-table"
				role="table"
				aria-label="Selections by period"
			>
				<div class="review-row header" role="row">
					<div role="columnheader">Period</div>
					<div role="columnheader">Course ID</div>
					<div role="columnheader">Course Name</div>
					<div role="columnheader">Teacher</div>
					<div role="columnheader">Location</div>
				</div>
				{#each reviewRows as row}
					{@const course = row.selection
						? courseMap[row.selection.course_id]
						: undefined}
					<div
						class={`review-row ${row.selection ? "" : "muted"}`}
						role="row"
					>
						<div role="cell">{row.period}</div>
						<div role="cell">
							{#if row.selection}
								{row.selection.course_id}
							{:else}
								—
							{/if}
						</div>
						<div role="cell">
							{#if course}
								{course.name}
							{:else if row.selection}
								Unknown course
							{:else}
								—
							{/if}
						</div>
						<div role="cell">
							{#if course}
								{course.teacher}
							{:else}
								—
							{/if}
						</div>
						<div role="cell">
							{#if course}
								{course.location}
							{:else}
								—
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</main>
</div>

<div class="toast-container" aria-live="polite">
	{#each toasts as toast (toast.id)}
		<div class={`toast ${toast.tone}`}>
			{toast.message}
		</div>
	{/each}
</div>

{#if confirmModal}
	{@const isRemoving = Boolean(confirmModal.existingChoice)}
	{@const isReplacing =
		Boolean(confirmModal.periodChoice) &&
		confirmModal.periodChoice.course_id !== confirmModal.course.id}
	{@const targetCourse =
		isRemoving && confirmModal.existingChoice
			? courseMap[confirmModal.existingChoice.course_id]
			: undefined}
	{@const replacingCourse =
		isReplacing && confirmModal.periodChoice
			? courseMap[confirmModal.periodChoice.course_id]
			: undefined}
	{@const needsInviteConfirm =
		targetCourse?.membership === "invite_only" ||
		replacingCourse?.membership === "invite_only"}
	{@const canConfirm = !needsInviteConfirm || confirmText === "I am sure"}

	<div
		class="modal-backdrop"
		role="presentation"
		onclick={cancelUpdate}
		onkeydown={(e: KeyboardEvent): void => {
			if (e.key === "Escape") {
				cancelUpdate()
			}
		}}
	>
		<div
			class="modal-content"
			role="dialog"
			aria-labelledby="modal-title"
			tabindex="-1"
			onclick={(e: MouseEvent): void => {
				e.stopPropagation()
			}}
			onkeydown={(e: KeyboardEvent): void => {
				e.stopPropagation()
			}}
		>
			<h3 id="modal-title">Confirm Selection Change</h3>

			{#if isRemoving}
				<p>
					Are you sure you want to remove your selection for <strong
						>{targetCourse?.name ?? "this course"}</strong
					>?
				</p>
				<p class="warning-text">
					If you remove your selection, you may not be able to rejoin
					if it becomes full.
				</p>
				{#if targetCourse?.membership === "invite_only"}
					<p class="warning-text">
						This is an invitation-only course. You will
						need another invitation to rejoin.
					</p>
				{/if}
			{:else if isReplacing}
				<p>
					Selecting <strong>{confirmModal.course.name}</strong> will
					remove your current selection for
					<strong
						>{replacingCourse?.name ?? "the other course"}</strong
					>
					in period {confirmModal.course.period}.
				</p>
				<p class="warning-text">
					If you remove your selection, you may not be able to
					rejoin if it becomes full.
				</p>
				{#if replacingCourse?.membership === "invite_only"}
					<p class="warning-text">
						The course you're removing is invitation-only.
						You will need another invitation to rejoin.
					</p>
				{/if}
			{/if}

			{#if needsInviteConfirm}
				<div class="confirm-field">
					<label for="confirm-text"
						>Type <strong>"I am sure"</strong> to confirm:</label
					>
					<input
						id="confirm-text"
						type="text"
						bind:value={confirmText}
						placeholder="I am sure"
					/>
				</div>
			{/if}

			<div class="modal-actions">
				<button class="ghost" onclick={cancelUpdate}>Cancel</button>
				<button
					class="primary"
					onclick={confirmUpdate}
					disabled={!canConfirm}
				>
					Confirm
				</button>
			</div>
		</div>
	</div>
{/if}
