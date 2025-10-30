<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import ReviewPage from './pages/ReviewPage.vue'
import SelectionPage from './pages/SelectionPage.vue'
import type { Choice, Course, GradeRequirement, Student } from './types'

interface CourseWithSelection extends Course {
	selected: boolean
}

type SelectionResponse = Pick<Choice, 'course_id' | 'period' | 'selection_type'>

function cancelAction(): void {
	pendingAction.value = null
	confirmInput.value = ''
	showInputError.value = false
}

function reloadPage(): void {
	window.location.reload()
}

const activeTab = ref<'Selection' | 'Review'>('Selection')
const ccas = ref<CourseWithSelection[]>([])
const userInfo = ref<Student | null>(null)
const searchQuery = ref<string>('')
const searchScope = ref<'global' | 'period'>('global')
const currentPeriod = ref<string>('')
const viewMode = ref<'grid' | 'table'>('grid')
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
const confirmModal = ref<HTMLDialogElement | null>(null)
const initialLoadComplete = ref(false)
const reconnectModal = ref<HTMLDialogElement | null>(null)
const pendingAction = ref<{
	type: 'unselect' | 'replace'
	course: CourseWithSelection
	existing?: CourseWithSelection
} | null>(null)
const confirmInput = ref('')
const showInputError = ref(false)

const extractErrorMessage = async (res: Response): Promise<string> => {
	const text = await res.text()
	if (text.length === 0) {
		return `Request failed with status ${res.status}`
	}
	try {
		const parsed: unknown = JSON.parse(text)
		if (typeof parsed === 'string') return parsed
		if (parsed !== null && typeof parsed === 'object') {
			const record = parsed as Record<string, unknown>
			const message = record.message
			if (typeof message === 'string') {
				return message
			}
			const error = record.error
			if (typeof error === 'string') {
				return error
			}
			const fallback = JSON.stringify(record)
			return fallback === '{}'
				? `Request failed with status ${res.status}`
				: fallback
		}
		return String(parsed)
	} catch {
		return text.trim() === 'null'
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
		res.type === 'opaqueredirect' ||
		(res.status >= 300 && res.status < 400)
	) {
		if (typeof window !== 'undefined') {
			window.location.href = '/'
		}
		throw new Error('Redirecting to root')
	}
	if (!res.ok) {
		throw new Error(await extractErrorMessage(res))
	}
	return (await res.json()) as T
}

const extractCourseId = (value: unknown): string | null => {
	if (value !== null && typeof value === 'object') {
		const courseId = (value as SelectionResponse).course_id
		if (typeof courseId === 'string') return courseId
		const fallback = (value as { courseID?: unknown }).courseID
		if (typeof fallback === 'string') return fallback
	}
	return typeof value === 'string' ? value : null
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
	method: 'PUT' | 'DELETE',
	courseId: string,
): Promise<boolean> => {
	try {
		const res = await fetch('/student/api/my_selections', {
			method,
			credentials: 'include',
			redirect: 'manual',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(courseId),
		})
		if (
			res.type === 'opaqueredirect' ||
			(res.status >= 300 && res.status < 400)
		) {
			if (typeof window !== 'undefined') {
				window.location.href = '/'
			}
			return false
		}
		if (!res.ok) {
			const errMsg = await extractErrorMessage(res)
			console.error('Selection update failed:', errMsg)
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
			err instanceof Error ? err.message : 'Unable to update selections.'
		console.error('Selection update error:', err)
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
		fetchJson<Course[]>('/student/api/courses', {
			credentials: 'include',
			redirect: 'manual',
		}),
		fetchJson<SelectionResponse[] | null>('/student/api/my_selections', {
			credentials: 'include',
			redirect: 'manual',
		}),
	])
	ccas.value = coursesData.map((course: Course) => ({
		...course,
		current_students:
			typeof course.current_students === 'number'
				? course.current_students
				: 0,
		selected: false,
	}))
	applySelections(selectionsData)
}

const loadGrades = async (): Promise<void> => {
	const gradesRes = await fetch('/student/api/grades', {
		credentials: 'include',
		redirect: 'manual',
	})
	if (
		gradesRes.type === 'opaqueredirect' ||
		(gradesRes.status >= 300 && gradesRes.status < 400)
	) {
		if (typeof window !== 'undefined') {
			window.location.href = '/'
		}
		return
	}
	const gradeData = (await gradesRes.json()) as GradeRequirement[]
	grades.value = gradeData
}

const loadPeriods = async (): Promise<void> => {
	const periodsRes = await fetch('/student/api/periods', {
		credentials: 'include',
		redirect: 'manual',
	})
	if (
		periodsRes.type === 'opaqueredirect' ||
		(periodsRes.status >= 300 && periodsRes.status < 400)
	) {
		if (typeof window !== 'undefined') {
			window.location.href = '/'
		}
		return
	}
	periods.value = (await periodsRes.json()) as string[]
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
	const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
	const wsUrl = `${protocol}//${window.location.host}/student/api/events`
	const socket = new WebSocket(wsUrl)
	ws = socket

	socket.onopen = (): void => {
		console.log('WebSocket connected')
		reconnectAttempts = 0
	}

	socket.onmessage = (event: MessageEvent<string>): void => {
		try {
			const message: string = event.data
			const parts: string[] = message.split(',')
			const eventType: string = parts[0]

			switch (eventType) {
				case 'invalidate_periods':
					void (async (): Promise<void> => {
						try {
							await Promise.all([loadCourses(), loadPeriods()])
						} catch (err) {
							console.error('Failed to refresh periods:', err)
						}
					})()
					break

				case 'invalidate_courses':
					void (async (): Promise<void> => {
						try {
							await loadCourses()
						} catch (err) {
							console.error('Failed to refresh courses:', err)
						}
					})()
					break

				case 'invalidate_categories':
					void (async (): Promise<void> => {
						try {
							await loadCourses()
						} catch (err) {
							console.error('Failed to refresh categories:', err)
						}
					})()
					break

				case 'invalidate_grades':
					void (async (): Promise<void> => {
						try {
							await loadGrades()
						} catch (err) {
							console.error('Failed to refresh grades:', err)
						}
					})()
					break

				case 'invalidate_selections':
					void (async (): Promise<void> => {
						try {
							await Promise.all([
								loadCourses(),
								loadGrades(),
								loadPeriods(),
							])
						} catch (err) {
							console.error('Failed to refresh selections:', err)
						}
					})()
					break

				case 'course_count_update':
					if (parts.length === 3) {
						const courseId: string = parts[1]
						const count: number = Number.parseInt(parts[2], 10)
						const target = ccas.value.find(
							(course: CourseWithSelection) =>
								course.id === courseId,
						)
						if (target !== undefined) {
							target.current_students = Number.isNaN(count)
								? 0
								: count
						}
					}
					break

				case 'notify':
					if (parts.length > 1) {
						showInfoToast(parts.slice(1).join(','))
					}
					break

				default:
					console.warn('Unknown WebSocket event type:', eventType)
					break
			}
		} catch (err) {
			console.error('Failed to parse WebSocket message:', err)
		}
	}

	socket.onerror = (error: Event): void => {
		console.error('WebSocket error:', error)
	}

	socket.onclose = (): void => {
		console.log('WebSocket disconnected')
		if (ws !== socket) return

		ws = null

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

onMounted(async (): Promise<void> => {
	try {
		userInfo.value = await fetchJson<Student>('/student/api/user_info', {
			credentials: 'include',
			redirect: 'manual',
		})
		await Promise.all([loadCourses(), loadGrades(), loadPeriods()])
		startWebSocket()
	} catch (err) {
		errorMessage.value =
			err instanceof Error ? err.message : 'Failed to load data.'
	} finally {
		initialLoadComplete.value = true
	}
})

const toggleCCA = async (id: string): Promise<void> => {
	const course = ccas.value.find((c: CourseWithSelection) => c.id === id)
	if (course === undefined || updatingCcaId.value !== null) return

	if (course.selected) {
		pendingAction.value = { type: 'unselect', course }
		confirmModal.value?.showModal()
		return
	}

	const existingSelection = ccas.value.find(
		(c) => c.period === course.period && c.selected,
	)
	if (existingSelection !== undefined) {
		pendingAction.value = {
			type: 'replace',
			course,
			existing: existingSelection,
		}
		confirmModal.value?.showModal()
		return
	}

	updatingCcaId.value = id
	errorMessage.value = null
	try {
		await requestSelectionUpdate('PUT', course.id)
	} finally {
		updatingCcaId.value = null
	}
}

const confirmAction = async (): Promise<void> => {
	if (pendingAction.value === null) return
	const action = pendingAction.value

	const needsConfirmation =
		(action.type === 'unselect' &&
			action.course.membership === 'invite_only') ||
		(action.type === 'replace' &&
			action.existing?.membership === 'invite_only')
	if (needsConfirmation && confirmInput.value !== 'I am really sure') {
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
		if (action.type === 'unselect') {
			await requestSelectionUpdate('DELETE', action.course.id)
		} else {
			const existingSelection = action.existing
			if (existingSelection === undefined) {
				console.error(
					'Missing existing selection while confirming replace action',
				)
				return
			}
			const removed = await requestSelectionUpdate(
				'DELETE',
				existingSelection.id,
			)
			if (removed) {
				await requestSelectionUpdate('PUT', action.course.id)
			}
		}
	} finally {
		updatingCcaId.value = null
		pendingAction.value = null
		confirmInput.value = ''
		showInputError.value = false
	}
}

const filteredCCAs = computed<CourseWithSelection[]>(() => {
	if (searchQuery.value.length === 0) return ccas.value

	const query = searchQuery.value.toLowerCase()
	let filtered = ccas.value

	if (searchScope.value === 'period' && currentPeriod.value !== '') {
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
}

const handleBeforeUnload = (): void => cleanup()

if (typeof window !== 'undefined') {
	window.addEventListener('beforeunload', handleBeforeUnload)
}

onBeforeUnmount((): void => {
	if (typeof window !== 'undefined') {
		window.removeEventListener('beforeunload', handleBeforeUnload)
	}
	cleanup()
})
</script>

<template>
	<div class="min-h-screen bg-white flex flex-col">
		<header class="bg-white/80 backdrop-blur-sm sticky top-0 z-50">
			<div
				class="flex justify-between items-center px-8 py-5 border-b border-gray-200"
			>
				<h1 class="text-xl font-light tracking-wide">CCA Selection</h1>
				<div v-if="userInfo" class="flex items-center gap-3 text-sm">
					<span class="text-gray-900 font-medium">{{
						userInfo.name
					}}</span>
					<span class="text-gray-400">·</span>
					<span class="text-gray-600">{{ userInfo.grade }}</span>
					<span class="text-gray-400">·</span>
					<span class="text-gray-600">ID: {{ userInfo.id }}</span>
				</div>
			</div>
			<div class="border-b border-gray-200 bg-white">
				<div
					class="flex flex-wrap justify-between items-center px-8 py-4 gap-4"
				>
					<div class="flex gap-12">
						<button
							@click="activeTab = 'Selection'"
							class="text-sm pb-2"
							:class="
								activeTab === 'Selection'
									? 'border-b-2 border-[#5bae31] text-[#5bae31]'
									: 'text-gray-500 hover:text-gray-900'
							"
						>
							Selection
						</button>
						<button
							@click="activeTab = 'Review'"
							class="text-sm pb-2"
							:class="
								activeTab === 'Review'
									? 'border-b-2 border-[#5bae31] text-[#5bae31]'
									: 'text-gray-500 hover:text-gray-900'
							"
						>
							Review
						</button>
					</div>
					<div class="flex gap-4 items-center">
						<label class="label">
							<input
								v-model="disableClientRestriction"
								type="checkbox"
								class="toggle toggle-sm checked:border-[#5bae31] checked:text-[#5bae31]"
							/>
							Disable Client Restriction
						</label>
						<select
							v-model="searchScope"
							class="text-xs border border-gray-300 rounded px-2 py-1.5"
						>
							<option value="global">Search globally</option>
							<option value="period" v-if="currentPeriod">
								Search in {{ currentPeriod }}
							</option>
						</select>
						<input
							v-model="searchQuery"
							type="text"
							placeholder="Search CCAs..."
							class="text-sm border border-gray-300 rounded px-3 py-1.5 w-20 sm:w-40"
						/>
					</div>
				</div>
			</div>
		</header>

		<div v-if="errorMessage" class="toast toast-top toast-center z-[60]">
			<div
				role="alert"
				class="alert alert-error !bg-red-100 !text-red-900"
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
				class="alert alert-info !bg-blue-100 !text-blue-900 !border-blue-100"
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
			ref="selectionPageRef"
			:ccas="filteredCCAs"
			:search-active="searchScope === 'global' && !!searchQuery"
			:user-grade="userInfo?.grade"
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
			:ccas="ccas"
			:user-grade="userInfo?.grade"
			:grades="grades"
			:periods="periods"
		/>
		<div v-else class="flex flex-1">
			<aside
				class="w-56 border-r border-gray-200 bg-white p-8 sticky top-[137px] self-start max-h-[calc(100vh-137px)] overflow-y-auto"
			>
				<div class="space-y-2">
					<div class="skeleton h-6 w-full"></div>
					<div class="skeleton h-6 w-full"></div>
					<div class="skeleton h-6 w-full"></div>
					<div class="skeleton h-6 w-full"></div>
				</div>
			</aside>
			<main class="flex-1 p-8 bg-gray-50/30">
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
			class="px-4 border-t border-gray-200 bg-white py-4 text-sm text-gray-600"
		>
			<p>
				Copyright © 2025
				<a href="https://runxiyu.org" style="color: #5bae31"
					>Runxi Yu</a
				>
				and Henry Yang.
			</p>

			<p>
				This program is Free Software: you can redistribute it and/or
				modify it under the terms of the
				<a
					href="https://www.gnu.org/licenses/agpl-3.0.en.html"
					style="color: #5bae31"
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
				<a style="color: #5bae31" href="https://sr.ht/~runxiyu/cca/"
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
					<button class="btn" @click="reloadPage">
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
					class="text-sm text-red-600 mb-4"
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
					class="text-sm text-red-600 mb-4"
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
						:class="{ 'text-red-600': showInputError }"
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
						<button class="btn" @click="cancelAction">
							Cancel
						</button>
						<button class="btn ml-2" @click.prevent="confirmAction">
							Confirm
						</button>
					</form>
				</div>
			</div>
		</dialog>
	</div>
</template>
