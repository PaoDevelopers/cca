<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue"
import ReviewPage from "./pages/ReviewPage.vue"
import SelectionPage from "./pages/SelectionPage.vue"
import type { Choice, Course, GradeRequirement, Student } from "./types"

interface CourseWithSelection extends Course {
	selected: boolean
}

type SelectionResponse = Pick<Choice, "course_id" | "period" | "selection_type">

function cancelAction(): void {
	pendingAction.value = null
	confirmInput.value = ""
	showInputError.value = false
}

function reloadPage(): void {
	window.location.reload()
}

const activeTab = ref<"Selection" | "Review">("Selection")
const ccas = ref<CourseWithSelection[]>([])
const userInfo = ref<Student | null>(null)
const searchQuery = ref<string>("")
const searchInput = ref<string>("")
const searchScope = ref<"global" | "period">("global")
const currentPeriod = ref<string>("")
const viewMode = ref<"grid" | "table">("grid")
const errorMessage = ref<string | null>(null)
let errorTimeout: number | null = null
const infoMessage = ref<string | null>(null)
let infoTimeout: number | null = null
const updatingCcaId = ref<string | null>(null)
const selectionPageRef = ref<{ loadPeriods: () => Promise<void> } | null>(null)
const grades = ref<GradeRequirement[]>([])
const periods = ref<string[]>([])
const disableClientRestriction = ref(false)
let ws: WebSocket | null = null
let reconnectAttempts = 0
const maxReconnectAttempts = 5
let reconnectTimeout: number | null = null
let isIntentionalClose = false
const isDisconnected = ref(false)
const confirmModal = ref<HTMLDialogElement | null>(null)
const initialLoadComplete = ref(false)
const reconnectModal = ref<HTMLDialogElement | null>(null)
const headerRef = ref<HTMLElement | null>(null)
const pendingAction = ref<{
	type: "unselect" | "replace"
	course: CourseWithSelection
	existing?: CourseWithSelection
} | null>(null)
const confirmInput = ref("")
const showInputError = ref(false)
let searchDebounceTimeout: number | null = null
let headerResizeObserver: ResizeObserver | null = null
let hasWindowResizeListener = false

const extractErrorMessage = async (res: Response): Promise<string> => {
	const text = await res.text()
	if (text.length === 0) {
		return `Request failed with status ${res.status}`
	}
	try {
		const parsed: unknown = JSON.parse(text)
		if (typeof parsed === "string") return parsed
		if (parsed !== null && typeof parsed === "object") {
			const record = parsed as Record<string, unknown>
			const message = record["message"]
			if (typeof message === "string") {
				return message
			}
			const error = record["error"]
			if (typeof error === "string") {
				return error
			}
			const fallback = JSON.stringify(record)
			return fallback === "{}"
				? `Request failed with status ${res.status}`
				: fallback
		}
		return String(parsed)
	} catch {
		return text.trim() === "null"
			? `Request failed with status ${res.status}`
			: text
	}
}

const fetchJson = async <T,>(
	input: RequestInfo,
	init?: RequestInit,
): Promise<T> => {
	const res = await fetch(input, init)
	if (
		res.type === "opaqueredirect" ||
		(res.status >= 300 && res.status < 400)
	) {
		if (typeof window !== "undefined") {
			window.location.href = "/"
		}
		throw new Error("Redirecting to root")
	}
	if (!res.ok) {
		throw new Error(await extractErrorMessage(res))
	}
	return (await res.json()) as T
}

const extractCourseId = (value: unknown): string | null => {
	if (value !== null && typeof value === "object") {
		const courseId = (value as SelectionResponse).course_id
		if (typeof courseId === "string") return courseId
		const fallback = (value as { courseID?: unknown }).courseID
		if (typeof fallback === "string") return fallback
	}
	return typeof value === "string" ? value : null
}

const applySelections = (
	selections: SelectionResponse[] | null | undefined,
): void => {
	const list = Array.isArray(selections) ? selections : []
	const selectedIds = new Set<string>()
	list.forEach((selection) => {
		const courseId = extractCourseId(selection)
		if (courseId !== null) {
			selectedIds.add(courseId)
		}
	})
	ccas.value = ccas.value.map((course) => ({
		...course,
		selected: selectedIds.has(course.id),
	}))
}

const requestSelectionUpdate = async (
	method: "PUT" | "DELETE",
	courseId: string,
): Promise<boolean> => {
	try {
		const res = await fetch("/student/api/my_selections", {
			method,
			credentials: "include",
			redirect: "manual",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(courseId),
		})
		if (
			res.type === "opaqueredirect" ||
			(res.status >= 300 && res.status < 400)
		) {
			if (typeof window !== "undefined") {
				window.location.href = "/"
			}
			return false
		}
		if (!res.ok) {
			const errMsg = await extractErrorMessage(res)
			console.error("Selection update failed:", errMsg)
			errorMessage.value = errMsg
			if (errorTimeout !== null) clearTimeout(errorTimeout)
			errorTimeout = window.setTimeout((): void => {
				errorMessage.value = null
			}, 5000)
			return false
		}
		const selections = (await res.json()) as SelectionResponse[] | null
		applySelections(selections)
		errorMessage.value = null
		return true
	} catch (err) {
		const errMsg =
			err instanceof Error ? err.message : "Unable to update selections."
		console.error("Selection update error:", err)
		errorMessage.value = errMsg
		if (errorTimeout !== null) clearTimeout(errorTimeout)
		errorTimeout = window.setTimeout((): void => {
			errorMessage.value = null
		}, 5000)
		return false
	}
}

const loadCourses = async (): Promise<void> => {
	const [coursesData, selectionsData] = await Promise.all([
		fetchJson<Course[]>("/student/api/courses", {
			credentials: "include",
			redirect: "manual",
		}),
		fetchJson<SelectionResponse[] | null>("/student/api/my_selections", {
			credentials: "include",
			redirect: "manual",
		}),
	])
	ccas.value = coursesData.map((course: Course) => ({
		...course,
		current_students:
			typeof course.current_students === "number"
				? course.current_students
				: 0,
		selected: false,
	}))
	applySelections(selectionsData)
}

const loadGrades = async (): Promise<void> => {
	const gradesRes = await fetch("/student/api/grades", {
		credentials: "include",
		redirect: "manual",
	})
	if (
		gradesRes.type === "opaqueredirect" ||
		(gradesRes.status >= 300 && gradesRes.status < 400)
	) {
		if (typeof window !== "undefined") {
			window.location.href = "/"
		}
		return
	}
	const gradeData = (await gradesRes.json()) as GradeRequirement[]
	grades.value = gradeData
}

const loadPeriods = async (): Promise<void> => {
	const periodsRes = await fetch("/student/api/periods", {
		credentials: "include",
		redirect: "manual",
	})
	if (
		periodsRes.type === "opaqueredirect" ||
		(periodsRes.status >= 300 && periodsRes.status < 400)
	) {
		if (typeof window !== "undefined") {
			window.location.href = "/"
		}
		return
	}
	periods.value = (await periodsRes.json()) as string[]
}

const fetchAllData = async (): Promise<void> => {
	const user = await fetchJson<Student>("/student/api/user_info", {
		credentials: "include",
		redirect: "manual",
	})
	userInfo.value = user
	await Promise.all([loadCourses(), loadGrades(), loadPeriods()])
}

const handleDataLoadError = (err: unknown): void => {
	console.error("Failed to load data:", err)
	errorMessage.value =
		err instanceof Error ? err.message : "Failed to load data."
}

const showInfoToast = (message: string): void => {
	infoMessage.value = message
	if (infoTimeout !== null) clearTimeout(infoTimeout)
	infoTimeout = window.setTimeout((): void => {
		infoMessage.value = null
		infoTimeout = null
	}, 30000)
}

const closeWebSocket = (): void => {
	if (ws !== null) {
		isIntentionalClose = true
		ws.close()
		ws = null
	}
}

const startWebSocket = (): void => {
	closeWebSocket()
	const protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
	const wsUrl = `${protocol}//${window.location.host}/student/api/events`
	const socket = new WebSocket(wsUrl)
	ws = socket

	socket.onopen = (): void => {
		console.log("WebSocket connected")
		reconnectAttempts = 0
		isDisconnected.value = false
	}

	socket.onmessage = (event: MessageEvent<string>): void => {
		try {
			const message: string = event.data
			const parts: string[] = message.split(",")
			const eventType = parts[0]
			if (eventType === undefined) {
				console.warn(
					"Received WebSocket message without event type:",
					message,
				)
				return
			}

			switch (eventType) {
				case "hello":
					void (async (): Promise<void> => {
						try {
							await fetchAllData()
						} catch (err) {
							handleDataLoadError(err)
						}
					})()
					break

				case "invalidate_periods":
					void (async (): Promise<void> => {
						try {
							await Promise.all([loadCourses(), loadPeriods()])
						} catch (err) {
							console.error("Failed to refresh periods:", err)
						}
					})()
					break

				case "invalidate_courses":
					void (async (): Promise<void> => {
						try {
							await loadCourses()
						} catch (err) {
							console.error("Failed to refresh courses:", err)
						}
					})()
					break

				case "invalidate_categories":
					void (async (): Promise<void> => {
						try {
							await loadCourses()
						} catch (err) {
							console.error("Failed to refresh categories:", err)
						}
					})()
					break

				case "invalidate_grades":
					void (async (): Promise<void> => {
						try {
							await loadGrades()
						} catch (err) {
							console.error("Failed to refresh grades:", err)
						}
					})()
					break

				case "invalidate_selections":
					void (async (): Promise<void> => {
						try {
							await Promise.all([
								loadCourses(),
								loadGrades(),
								loadPeriods(),
							])
						} catch (err) {
							console.error("Failed to refresh selections:", err)
						}
					})()
					break

				case "course_count_update": {
					const courseId = parts[1]
					const countPart = parts[2]
					if (courseId === undefined || countPart === undefined) {
						console.warn(
							"Invalid course_count_update payload:",
							parts,
						)
						break
					}
					const count = Number.parseInt(countPart, 10)
					const target = ccas.value.find(
						(course: CourseWithSelection) => course.id === courseId,
					)
					if (target !== undefined) {
						target.current_students = Number.isNaN(count)
							? 0
							: count
					}
					break
				}

				case "notify":
					if (parts.length > 1) {
						showInfoToast(parts.slice(1).join(","))
					}
					break

				default:
					console.warn("Unknown WebSocket event type:", eventType)
					break
			}
		} catch (err) {
			console.error("Failed to parse WebSocket message:", err)
		}
	}

	socket.onerror = (error: Event): void => {
		console.error("WebSocket error:", error)
	}

	socket.onclose = (): void => {
		console.log("WebSocket disconnected")
		if (ws !== socket) return

		ws = null

		if (!isIntentionalClose) {
			isDisconnected.value = true
		}

		if (isIntentionalClose) {
			isIntentionalClose = false
			return
		}

		if (reconnectAttempts < maxReconnectAttempts) {
			reconnectAttempts++
			const delay = Math.min(
				1000 * Math.pow(2, reconnectAttempts - 1),
				10000,
			)
			console.log(
				`Reconnecting in ${delay}ms (attempt ${reconnectAttempts}/${maxReconnectAttempts})`,
			)
			reconnectTimeout = window.setTimeout(() => {
				startWebSocket()
			}, delay)
		} else {
			reconnectModal.value?.showModal()
		}
	}
}

const handleReconnectClick = (): void => {
	if (!isDisconnected.value) return
	if (reconnectTimeout !== null) {
		clearTimeout(reconnectTimeout)
		reconnectTimeout = null
	}
	reconnectAttempts = 0
	startWebSocket()
}

onMounted(async (): Promise<void> => {
	try {
		await fetchAllData()
		startWebSocket()
	} catch (err) {
		handleDataLoadError(err)
	} finally {
		initialLoadComplete.value = true
	}
	if (typeof window !== "undefined") {
		if (!hasWindowResizeListener) {
			window.addEventListener("resize", updateHeaderOffset)
			hasWindowResizeListener = true
		}
		window.requestAnimationFrame(() => {
			updateHeaderOffset()
			if ("ResizeObserver" in window && headerRef.value !== null) {
				if (headerResizeObserver !== null) {
					headerResizeObserver.disconnect()
				}
				headerResizeObserver = new ResizeObserver(() => {
					updateHeaderOffset()
				})
				headerResizeObserver.observe(headerRef.value)
			}
		})
	}
})

const toggleCCA = async (id: string): Promise<void> => {
	const course = ccas.value.find((c: CourseWithSelection) => c.id === id)
	if (course === undefined || updatingCcaId.value !== null) return

	if (course.selected) {
		pendingAction.value = { type: "unselect", course }
		confirmModal.value?.showModal()
		return
	}

	const existingSelection = ccas.value.find(
		(c) => c.period === course.period && c.selected,
	)
	if (existingSelection !== undefined) {
		pendingAction.value = {
			type: "replace",
			course,
			existing: existingSelection,
		}
		confirmModal.value?.showModal()
		return
	}

	updatingCcaId.value = id
	errorMessage.value = null
	try {
		await requestSelectionUpdate("PUT", course.id)
	} finally {
		updatingCcaId.value = null
	}
}

const confirmAction = async (): Promise<void> => {
	if (pendingAction.value === null) return
	const action = pendingAction.value

	const needsConfirmation =
		(action.type === "unselect" &&
			action.course.membership === "invite_only") ||
		(action.type === "replace" &&
			action.existing?.membership === "invite_only")
	if (needsConfirmation && confirmInput.value !== "I am really sure") {
		showInputError.value = true
		window.setTimeout((): void => {
			showInputError.value = false
		}, 1500)
		return
	}

	confirmModal.value?.close()
	updatingCcaId.value = action.course.id
	errorMessage.value = null

	try {
		if (action.type === "unselect") {
			await requestSelectionUpdate("DELETE", action.course.id)
		} else {
			const existingSelection = action.existing
			if (existingSelection === undefined) {
				console.error(
					"Missing existing selection while confirming replace action",
				)
				return
			}
			const removed = await requestSelectionUpdate(
				"DELETE",
				existingSelection.id,
			)
			if (removed) {
				await requestSelectionUpdate("PUT", action.course.id)
			}
		}
	} finally {
		updatingCcaId.value = null
		pendingAction.value = null
		confirmInput.value = ""
		showInputError.value = false
	}
}

const filteredCCAs = computed<CourseWithSelection[]>(() => {
	if (searchQuery.value.length === 0) return ccas.value

	const query = searchQuery.value.toLowerCase()
	let filtered = ccas.value

	if (searchScope.value === "period" && currentPeriod.value !== "") {
		filtered = filtered.filter((c) => c.period === currentPeriod.value)
	}

	return filtered.filter(
		(c) =>
			c.name.toLowerCase().includes(query) ||
			c.id.toLowerCase().includes(query) ||
			c.description.toLowerCase().includes(query) ||
			c.teacher.toLowerCase().includes(query) ||
			c.location.toLowerCase().includes(query),
	)
})

watch(
	() => searchInput.value,
	(newValue): void => {
		if (searchDebounceTimeout !== null) {
			clearTimeout(searchDebounceTimeout)
		}
		searchDebounceTimeout = window.setTimeout((): void => {
			searchQuery.value = newValue.trim()
			searchDebounceTimeout = null
		}, 250)
	},
)

watch(
	() => searchQuery.value,
	(newValue): void => {
		if (newValue !== searchInput.value) {
			searchInput.value = newValue
		}
	},
	{ immediate: true },
)

const userGradeBinding = computed(() => {
	const grade = userInfo.value?.grade
	if (typeof grade === "string" && grade.length > 0) {
		return { userGrade: grade }
	}
	return {}
})

const currentGradeInfo = computed(() => {
	const gradeId = userInfo.value?.grade
	if (typeof gradeId !== "string" || gradeId.length === 0) return null
	return grades.value.find((grade) => grade.grade === gradeId) ?? null
})

const updateHeaderOffset = (): void => {
	if (typeof document === "undefined") return
	const headerEl = headerRef.value
	if (headerEl === null) return
	const height = Math.ceil(headerEl.getBoundingClientRect().height)
	document.documentElement.style.setProperty(
		"--cca-header-offset",
		`${height}px`,
	)
}

const cleanup = (): void => {
	closeWebSocket()
	if (reconnectTimeout !== null) {
		clearTimeout(reconnectTimeout)
		reconnectTimeout = null
	}
	if (infoTimeout !== null) {
		clearTimeout(infoTimeout)
		infoTimeout = null
	}
	if (errorTimeout !== null) {
		clearTimeout(errorTimeout)
		errorTimeout = null
	}
	if (searchDebounceTimeout !== null) {
		clearTimeout(searchDebounceTimeout)
		searchDebounceTimeout = null
	}
	if (hasWindowResizeListener && typeof window !== "undefined") {
		window.removeEventListener("resize", updateHeaderOffset)
		hasWindowResizeListener = false
	}
	if (headerResizeObserver !== null) {
		headerResizeObserver.disconnect()
		headerResizeObserver = null
	}
}

const handleBeforeUnload = (): void => cleanup()

if (typeof window !== "undefined") {
	window.addEventListener("beforeunload", handleBeforeUnload)
}

onBeforeUnmount((): void => {
	if (typeof window !== "undefined") {
		window.removeEventListener("beforeunload", handleBeforeUnload)
	}
	cleanup()
})
</script>

<template>
	<div class="min-h-screen bg-surface-solid flex flex-col text-ink">
		<header ref="headerRef" class="bg-surface-solid sticky top-0 z-50">
			<div
				class="flex justify-between items-center px-8 py-5 border-b border-subtle bg-surface-solid"
			>
				<div class="flex flex-col items-start">
					<h1 class="text-xl font-light tracking-wide">
						CCA Selection
					</h1>
					<div
						v-if="currentGradeInfo"
						class="flex items-center gap-2 text-xs text-ink-muted mt-1"
					>
						<span>Grade {{ currentGradeInfo.grade }}</span>
						<span class="text-ink-muted/70">·</span>
						<span
							:class="[
								'font-medium',
								currentGradeInfo.enabled
									? 'text-primary'
									: 'text-danger',
							]"
						>
							{{
								currentGradeInfo.enabled
									? "Enabled"
									: "Disabled"
							}}
						</span>
						<span class="text-ink-muted/70">·</span>
						<span>
							Max own choices:
							{{ currentGradeInfo.max_own_choices }}
						</span>
					</div>
				</div>
				<div class="flex items-center gap-3 text-sm text-ink-muted">
					<button
						v-if="isDisconnected"
						type="button"
						class="px-2 py-1 cca-danger rounded cca-bg-danger-soft font-medium transition-colors"
						@click="handleReconnectClick"
						aria-label="Reconnect to the server"
						title="Attempt to reconnect"
					>
						Disconnected
					</button>
					<template v-if="userInfo">
						<span class="text-ink font-medium">{{
							userInfo.name
						}}</span>
						<span class="text-ink-muted">·</span>
						<span>{{ userInfo.grade }}</span>
						<span class="text-ink-muted">·</span>
						<span> ID: {{ userInfo.id }} </span>
					</template>
				</div>
			</div>
			<div class="border-b border-subtle bg-surface-solid">
				<div
					class="flex flex-wrap justify-between items-center px-8 py-4 gap-4"
				>
					<div class="flex gap-12">
						<button
							type="button"
							@click="activeTab = 'Selection'"
							class="text-sm pb-2"
							:class="
								activeTab === 'Selection'
									? 'border-b-2 border-primary text-primary'
									: 'text-ink-muted hover:text-ink'
							"
							:aria-pressed="activeTab === 'Selection'"
							:aria-current="
								activeTab === 'Selection' ? 'page' : undefined
							"
						>
							Selection
						</button>
						<button
							type="button"
							@click="activeTab = 'Review'"
							class="text-sm pb-2"
							:class="
								activeTab === 'Review'
									? 'border-b-2 border-primary text-primary'
									: 'text-ink-muted hover:text-ink'
							"
							:aria-pressed="activeTab === 'Review'"
							:aria-current="
								activeTab === 'Review' ? 'page' : undefined
							"
						>
							Review
						</button>
					</div>
					<div class="flex flex-wrap gap-4 items-center text-ink">
						<label
							class="inline-flex items-center gap-2 text-sm"
							for="disable-client-restriction"
						>
							<input
								id="disable-client-restriction"
								v-model="disableClientRestriction"
								type="checkbox"
								class="toggle toggle-sm toggle-primary"
								aria-describedby="disable-client-restriction-help"
							/>
							<span id="disable-client-restriction-help">
								Disable client restriction
							</span>
						</label>
						<div class="flex items-center gap-2 text-sm">
							<label class="sr-only" for="search-scope"
								>Search scope</label
							>
							<select
								id="search-scope"
								v-model="searchScope"
								class="text-xs border border-gray-300 rounded px-2 py-1.5 bg-surface"
								aria-label="Search scope"
							>
								<option value="global">Search globally</option>
								<option value="period" v-if="currentPeriod">
									Search in {{ currentPeriod }}
								</option>
							</select>
							<label class="sr-only" for="search-input"
								>Search CCAs</label
							>
							<input
								id="search-input"
								v-model="searchInput"
								type="search"
								placeholder="Search CCAs..."
								class="border border-gray-300 rounded px-3 py-1.5 w-24 sm:w-52 bg-surface"
								enterkeyhint="search"
								autocomplete="off"
							/>
						</div>
					</div>
				</div>
			</div>
		</header>

		<div v-if="errorMessage" class="toast toast-top toast-center z-[60]">
			<div
				role="alert"
				class="alert alert-error !bg-danger-soft !text-danger !border-danger-soft"
			>
				<svg
					xmlns="http://www.w3.org/2000/svg"
					class="h-6 w-6 shrink-0 stroke-current"
					fill="none"
					viewBox="0 0 24 24"
				>
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z"
					/>
				</svg>
				<span>{{ errorMessage }}</span>
			</div>
		</div>

		<div v-if="infoMessage" class="toast toast-top toast-center z-[60]">
			<div
				role="alert"
				class="alert alert-success !bg-primary-soft !text-primary !border-primary-soft"
			>
				<svg
					xmlns="http://www.w3.org/2000/svg"
					fill="none"
					viewBox="0 0 24 24"
					class="h-6 w-6 shrink-0 stroke-current"
				>
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
					></path>
				</svg>
				<span>{{ infoMessage }}</span>
			</div>
		</div>

		<SelectionPage
			v-if="activeTab === 'Selection' && initialLoadComplete"
			v-bind="userGradeBinding"
			ref="selectionPageRef"
			:ccas="filteredCCAs"
			:search-active="searchScope === 'global' && !!searchQuery"
			:grades="grades"
			:periods="periods"
			:initial-period="currentPeriod"
			:initial-view-mode="viewMode"
			:disable-client-restriction="disableClientRestriction"
			:updating-cca-id="updatingCcaId"
			@toggle="toggleCCA"
			@period-change="currentPeriod = $event"
			@view-mode-change="viewMode = $event"
		/>
		<ReviewPage
			v-else-if="activeTab === 'Review' && initialLoadComplete"
			v-bind="userGradeBinding"
			:ccas="ccas"
			:grades="grades"
			:periods="periods"
		/>
		<div v-else class="flex flex-1">
			<aside
				class="w-56 border-r border-subtle bg-surface p-8 sticky self-start overflow-y-auto"
				style="
					top: var(--cca-header-offset);
					max-height: calc(100vh - var(--cca-header-offset));
				"
			>
				<div class="space-y-2">
					<div class="skeleton h-6 w-full"></div>
					<div class="skeleton h-6 w-full"></div>
					<div class="skeleton h-6 w-full"></div>
					<div class="skeleton h-6 w-full"></div>
				</div>
			</aside>
			<main class="flex-1 p-8 bg-subtle">
				<div class="flex justify-between items-center mb-6">
					<div class="skeleton h-10 w-64"></div>
					<div class="flex gap-2">
						<div class="skeleton h-10 w-10 rounded"></div>
						<div class="skeleton h-10 w-10 rounded"></div>
					</div>
				</div>
				<div
					class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6"
				>
					<div class="skeleton h-64 w-full"></div>
					<div class="skeleton h-64 w-full"></div>
					<div class="skeleton h-64 w-full"></div>
				</div>
			</main>
		</div>

		<footer
			class="px-4 border-t border-subtle bg-surface py-4 text-sm text-ink-muted"
		>
			<p>
				Copyright © 2025
				<a href="https://runxiyu.org" class="text-primary">Runxi Yu</a>
				and Henry Yang.
			</p>

			<p>
				This program is Free Software: you can redistribute it and/or
				modify it under the terms of the
				<a
					href="https://www.gnu.org/licenses/agpl-3.0.en.html"
					class="text-primary"
					>GNU Affero General Public License as published by the Free
					Software Foundation, version 3</a
				>
				only. This program is distributed in the hope that it will be
				useful, but WITHOUT ANY WARRANTY; without even the implied
				warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
				See the GNU Affero General Public License for more details.
			</p>

			<p>
				The source code is available
				<a class="text-primary" href="https://sr.ht/~runxiyu/cca/"
					>on SourceHut</a
				>.
			</p>
		</footer>

		<dialog ref="reconnectModal" class="modal">
			<div class="modal-box">
				<h3 class="font-bold text-lg">Connection Lost</h3>
				<p class="py-4">
					Unable to reconnect to the server. Please refresh the page
					to continue.
				</p>
				<div class="modal-action">
					<button class="btn btn-primary" @click="reloadPage">
						Refresh Page
					</button>
				</div>
			</div>
		</dialog>

		<dialog ref="confirmModal" class="modal">
			<div class="modal-box">
				<h3 class="font-bold text-lg">Confirm Action</h3>
				<p class="py-4" v-if="pendingAction?.type === 'unselect'">
					Do you really want to unselect
					<strong>{{ pendingAction.course.name }}</strong
					>?
				</p>
				<p
					class="text-sm text-danger mb-4"
					v-if="
						pendingAction?.type === 'unselect' &&
						pendingAction.course.membership === 'invite_only'
					"
				>
					This is an invite-only CCA. You cannot select it back after
					unselecting.
				</p>
				<p class="py-4" v-else-if="pendingAction?.type === 'replace'">
					Do you really want to unselect
					<strong>{{ pendingAction.existing?.name }}</strong> and
					select <strong>{{ pendingAction.course.name }}</strong
					>?
				</p>
				<p
					class="text-sm text-danger mb-4"
					v-if="
						pendingAction?.type === 'replace' &&
						pendingAction.existing?.membership === 'invite_only'
					"
				>
					This is an invite-only CCA. You cannot select it back after
					unselecting.
				</p>
				<div
					v-if="
						(pendingAction?.type === 'unselect' &&
							pendingAction.course.membership ===
								'invite_only') ||
						(pendingAction?.type === 'replace' &&
							pendingAction.existing?.membership ===
								'invite_only')
					"
					class="mb-4"
				>
					<label
						class="block text-sm mb-2"
						:class="{ 'text-danger': showInputError }"
						>Type "I am really sure" to confirm:</label
					>
					<input
						v-model="confirmInput"
						type="text"
						class="input input-bordered w-full"
						:class="{ 'input-error': showInputError }"
					/>
				</div>
				<div class="modal-action">
					<form method="dialog">
						<button class="btn btn-ghost" @click="cancelAction">
							Cancel
						</button>
						<button
							class="btn btn-primary ml-2"
							@click.prevent="confirmAction"
						>
							Confirm
						</button>
					</form>
				</div>
			</div>
		</dialog>
	</div>
</template>
