<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted, ref} from 'vue'
import SelectionPage from './pages/SelectionPage.vue'
import ReviewPage from './pages/ReviewPage.vue'
import type {Choice, Course, Student} from './types'

interface CourseWithSelection extends Course {
    selected: boolean
}

type SelectionResponse = Pick<Choice, 'course_id' | 'period' | 'selection_type'>

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
const grades = ref<any[]>([])
const periods = ref<string[]>([])
const disableClientRestriction = ref(false)
let eventSource: EventSource | null = null
const confirmModal = ref<HTMLDialogElement | null>(null)
const pendingAction = ref<{ type: 'unselect' | 'replace', course: CourseWithSelection, existing?: CourseWithSelection } | null>(null)
const confirmInput = ref('')
const showInputError = ref(false)

const extractErrorMessage = async (res: Response) => {
    const text = await res.text()
    if (!text) {
        return `Request failed with status ${res.status}`
    }
    try {
        const data = JSON.parse(text)
        if (typeof data === 'string') return data
        if (data && typeof data === 'object') {
            if (typeof (data as { message?: unknown }).message === 'string') {
                return (data as { message: string }).message
            }
            if (typeof (data as { error?: unknown }).error === 'string') {
                return (data as { error: string }).error
            }
            const fallback = JSON.stringify(data)
            return fallback === '{}' ? `Request failed with status ${res.status}` : fallback
        }
        return String(data)
    } catch {
        return text.trim() === 'null' ? `Request failed with status ${res.status}` : text
    }
}

const fetchJson = async <T>(input: RequestInfo, init?: RequestInit) => {
    const res = await fetch(input, init)
    if (res.type === 'opaqueredirect' || (res.status >= 300 && res.status < 400)) {
        if (typeof window !== 'undefined') {
            window.location.href = '/'
        }
        throw new Error('Redirecting to root')
    }
    if (!res.ok) {
        throw new Error(await extractErrorMessage(res))
    }
    return await res.json() as Promise<T>
}

const extractCourseId = (value: unknown) => {
    if (value && typeof value === 'object') {
        const courseId = (value as SelectionResponse).course_id
        if (typeof courseId === 'string') return courseId
        const fallback = (value as { courseID?: unknown }).courseID
        if (typeof fallback === 'string') return fallback
    }
    return typeof value === 'string' ? value : null
}

const applySelections = (selections: SelectionResponse[] | null | undefined) => {
    const list = Array.isArray(selections) ? selections : []
    const selectedIds = new Set<string>()
    list.forEach(selection => {
        const courseId = extractCourseId(selection)
        if (courseId) {
            selectedIds.add(courseId)
        }
    })
    ccas.value = ccas.value.map(course => ({
        ...course,
        selected: selectedIds.has(course.id)
    }))
}

const requestSelectionUpdate = async (method: 'PUT' | 'DELETE', courseId: string) => {
    try {
        const res = await fetch('/student/api/my_selections', {
            method,
            credentials: 'include',
            redirect: 'manual',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(courseId)
        })
        if (res.type === 'opaqueredirect' || (res.status >= 300 && res.status < 400)) {
            if (typeof window !== 'undefined') {
                window.location.href = '/'
            }
            return false
        }
        if (!res.ok) {
            const errMsg = await extractErrorMessage(res)
            console.error('Selection update failed:', errMsg)
            errorMessage.value = errMsg
            if (errorTimeout) clearTimeout(errorTimeout)
            errorTimeout = setTimeout(() => errorMessage.value = null, 5000)
            return false
        }
        const selections = await res.json() as SelectionResponse[] | null
        applySelections(selections)
        errorMessage.value = null
        return true
    } catch (err) {
        const errMsg = err instanceof Error ? err.message : 'Unable to update selections.'
        console.error('Selection update error:', err)
        errorMessage.value = errMsg
        if (errorTimeout) clearTimeout(errorTimeout)
        errorTimeout = setTimeout(() => errorMessage.value = null, 5000)
        return false
    }
}

const loadCourses = async () => {
    const [coursesData, selectionsData] = await Promise.all([
        fetchJson<Course[]>('/student/api/courses', {credentials: 'include', redirect: 'manual'}),
        fetchJson<SelectionResponse[] | null>('/student/api/my_selections', {credentials: 'include', redirect: 'manual'})
    ])
    ccas.value = coursesData.map((course: Course) => ({
        ...course,
        current_students: course.current_students ?? 0,
        selected: false
    }))
    applySelections(selectionsData)
}

const loadGrades = async () => {
    const gradesRes = await fetch('/student/api/grades', {credentials: 'include', redirect: 'manual'})
    if (gradesRes.type === 'opaqueredirect' || (gradesRes.status >= 300 && gradesRes.status < 400)) {
        if (typeof window !== 'undefined') {
            window.location.href = '/'
        }
        return
    }
    grades.value = await gradesRes.json()
}

const loadPeriods = async () => {
    const periodsRes = await fetch('/student/api/periods', {credentials: 'include', redirect: 'manual'})
    if (periodsRes.type === 'opaqueredirect' || (periodsRes.status >= 300 && periodsRes.status < 400)) {
        if (typeof window !== 'undefined') {
            window.location.href = '/'
        }
        return
    }
    periods.value = await periodsRes.json()
}

const showInfoToast = (message: string) => {
    infoMessage.value = message
    if (infoTimeout) clearTimeout(infoTimeout)
    infoTimeout = window.setTimeout(() => {
        infoMessage.value = null
        infoTimeout = null
    }, 5000)
}

const closeEventSource = () => {
    if (eventSource) {
        eventSource.close()
        eventSource = null
    }
}

const startEventStream = () => {
    closeEventSource()
    const source = new EventSource('/student/api/events')
    eventSource = source

    source.addEventListener('invalidate_periods', async () => {
        try {
            await Promise.all([loadCourses(), loadPeriods()])
        } catch (err) {
            console.error('Failed to refresh periods:', err)
        }
    })
    source.addEventListener('invalidate_courses', async () => {
        try {
            await loadCourses()
        } catch (err) {
            console.error('Failed to refresh courses:', err)
        }
    })
    source.addEventListener('invalidate_categories', async () => {
        try {
            await loadCourses()
        } catch (err) {
            console.error('Failed to refresh categories:', err)
        }
    })
    source.addEventListener('invalidate_grades', async () => {
        try {
            await loadGrades()
        } catch (err) {
            console.error('Failed to refresh grades:', err)
        }
    })
    source.addEventListener('invalidate_selections', async () => {
        try {
            await Promise.all([loadCourses(), loadGrades(), loadPeriods()])
        } catch (err) {
            console.error('Failed to refresh selections:', err)
        }
    })
    source.addEventListener('course_count_update', (event) => {
        try {
            const data = JSON.parse((event as MessageEvent<string>).data)
            if (!data || typeof data.c !== 'string') {
                return
            }
            const count = typeof data.n === 'number' ? data.n : Number.parseInt(data.n, 10)
            const target = ccas.value.find((course: CourseWithSelection) => course.id === data.c)
            if (target) {
                target.current_students = Number.isNaN(count) ? 0 : count
            }
        } catch (err) {
            console.error('Failed to process course_count_update event:', err)
        }
    })
    source.addEventListener('notify', (event) => {
        const data = (event as MessageEvent<string>).data
        if (data) {
            showInfoToast(data)
        }
    })
    source.onerror = () => {
        source.close()
        if (eventSource === source) {
            eventSource = null
        }
    }
}

onMounted(async () => {
    try {
        userInfo.value = await fetchJson<Student>('/student/api/user_info', {credentials: 'include', redirect: 'manual'})
        await Promise.all([loadCourses(), loadGrades(), loadPeriods()])
        startEventStream()
    } catch (err) {
        errorMessage.value = err instanceof Error ? err.message : 'Failed to load data.'
    }
})

const toggleCCA = async (id: string) => {
    const course = ccas.value.find((c: CourseWithSelection) => c.id === id)
    if (!course || updatingCcaId.value) return

    if (course.selected) {
        pendingAction.value = { type: 'unselect', course }
        confirmModal.value?.showModal()
        return
    }

    const existingSelection = ccas.value.find(c => c.period === course.period && c.selected)
    if (existingSelection) {
        pendingAction.value = { type: 'replace', course, existing: existingSelection }
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

const confirmAction = async () => {
    if (!pendingAction.value) return

    const needsConfirmation = (pendingAction.value.type === 'unselect' && pendingAction.value.course.membership === 'invite_only') ||
        (pendingAction.value.type === 'replace' && pendingAction.value.existing?.membership === 'invite_only')
    if (needsConfirmation && confirmInput.value !== 'I am really sure') {
        showInputError.value = true
        setTimeout(() => showInputError.value = false, 1500)
        return
    }

    confirmModal.value?.close()
    updatingCcaId.value = pendingAction.value.course.id
    errorMessage.value = null

    try {
        if (pendingAction.value.type === 'unselect') {
            await requestSelectionUpdate('DELETE', pendingAction.value.course.id)
        } else {
            const removed = await requestSelectionUpdate('DELETE', pendingAction.value.existing!.id)
            if (removed) {
                await requestSelectionUpdate('PUT', pendingAction.value.course.id)
            }
        }
    } finally {
        updatingCcaId.value = null
        pendingAction.value = null
        confirmInput.value = ''
        showInputError.value = false
    }
}

const filteredCCAs = computed(() => {
    if (!searchQuery.value) return ccas.value

    const query = searchQuery.value.toLowerCase()
    let filtered = ccas.value

    if (searchScope.value === 'period' && currentPeriod.value) {
        filtered = filtered.filter(c => c.period === currentPeriod.value)
    }

    return filtered.filter(c =>
        c.name.toLowerCase().includes(query) ||
        c.id.toLowerCase().includes(query) ||
        c.description.toLowerCase().includes(query) ||
        c.teacher.toLowerCase().includes(query) ||
        c.location.toLowerCase().includes(query)
    )
})

const cleanup = () => {
    closeEventSource()
    if (infoTimeout) {
        clearTimeout(infoTimeout)
        infoTimeout = null
    }
    if (errorTimeout) {
        clearTimeout(errorTimeout)
        errorTimeout = null
    }
}

const handleBeforeUnload = () => cleanup()

if (typeof window !== 'undefined') {
    window.addEventListener('beforeunload', handleBeforeUnload)
}

onBeforeUnmount(() => {
    if (typeof window !== 'undefined') {
        window.removeEventListener('beforeunload', handleBeforeUnload)
    }
    cleanup()
})
</script>

<style scoped>
@keyframes flash {
    0%, 100% { color: inherit; }
    50% { color: #dc2626; }
}
</style>

<template>
    <div class="min-h-screen bg-white flex flex-col">
        <header class="bg-white/80 backdrop-blur-sm sticky top-0 z-50">
            <div class="flex justify-between items-center px-8 py-5 border-b border-gray-200">
                <h1 class="text-xl font-light tracking-wide">CCA Selection</h1>
                <div v-if="userInfo" class="flex items-center gap-3 text-sm">
                    <span class="text-gray-900 font-medium">{{ userInfo.name }}</span>
                    <span class="text-gray-400">·</span>
                    <span class="text-gray-600">{{ userInfo.grade }}</span>
                    <span class="text-gray-400">·</span>
                    <span class="text-gray-600">ID: {{ userInfo.id }}</span>
                </div>
            </div>
            <div class="border-b border-gray-200 bg-white">
                <div class="flex flex-wrap justify-between items-center px-8 py-4 gap-4">
                    <div class="flex gap-12">
                        <button
                            @click="activeTab = 'Selection'"
                            class="text-sm pb-2"
                            :class="activeTab === 'Selection' ? 'border-b-2 border-[#5bae31] text-[#5bae31]' : 'text-gray-500 hover:text-gray-900'"
                        >
                            Selection
                        </button>
                        <button
                            @click="activeTab = 'Review'"
                            class="text-sm pb-2"
                            :class="activeTab === 'Review' ? 'border-b-2 border-[#5bae31] text-[#5bae31]' : 'text-gray-500 hover:text-gray-900'"
                        >
                            Review
                        </button>
                    </div>
                    <div class="flex gap-4 items-center">
                        <label class="label">
                            <input v-model="disableClientRestriction" type="checkbox" class="toggle toggle-sm checked:border-[#5bae31]  checked:text-[#5bae31]"/>
                            Disable Client Restriction
                        </label>
                        <select v-model="searchScope" class="text-xs border border-gray-300 rounded px-2 py-1.5">
                            <option value="global">Search globally</option>
                            <option value="period" v-if="currentPeriod">Search in {{ currentPeriod }}</option>
                        </select>
                        <input v-model="searchQuery" type="text" placeholder="Search CCAs..."
                               class="text-sm border border-gray-300 rounded px-3 py-1.5 w-20 sm:w-40"/>
                    </div>
                </div>
            </div>
        </header>

        <div v-if="errorMessage" class="toast toast-top toast-center z-[60]">
            <div role="alert" class="alert alert-error !bg-red-100 !text-red-900">
                <svg xmlns="http://www.w3.org/2000/svg" class="h-6 w-6 shrink-0 stroke-current" fill="none" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <span>{{ errorMessage }}</span>
            </div>
        </div>

        <div v-if="infoMessage" class="toast toast-top toast-center z-[60]">
            <div role="alert" class="alert alert-info !bg-blue-100 !text-blue-900 !border-blue-100">
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="h-6 w-6 shrink-0 stroke-current">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                </svg>
                <span>{{ infoMessage }}</span>
            </div>
        </div>


        <SelectionPage v-if="activeTab === 'Selection'" ref="selectionPageRef" :ccas="filteredCCAs"
                       :search-active="searchScope === 'global' && !!searchQuery" :user-grade="userInfo?.grade"
                       :grades="grades" :periods="periods" :initial-period="currentPeriod" :initial-view-mode="viewMode" :disable-client-restriction="disableClientRestriction" :updating-cca-id="updatingCcaId" @toggle="toggleCCA" @period-change="currentPeriod = $event" @view-mode-change="viewMode = $event"/>
        <ReviewPage v-else :ccas="ccas" :user-grade="userInfo?.grade" :grades="grades" :periods="periods"/>

        <footer class="border-t border-gray-200 bg-white py-4 text-center text-sm text-gray-600">
            Copyright © 2025 <a href="https://runxiyu.org" style="color: #5bae31;">Runxi Yu</a> and Henry Yang. This program is Free Software: you can redistribute it and/or modify it under the terms of the <a href="https://www.gnu.org/licenses/agpl-3.0.en.html" style="color: #5bae31;">GNU Affero General Public License as published by the Free Software Foundation, version 3</a> only. This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Affero General Public License for more details. The source code is available <a style="color: #5bae31;" href="https://sr.ht/~runxiyu/cca/">on SourceHut</a>.
        </footer>

        <dialog ref="confirmModal" class="modal">
            <div class="modal-box">
                <h3 class="font-bold text-lg">Confirm Action</h3>
                <p class="py-4" v-if="pendingAction?.type === 'unselect'">
                    Do you really want to unselect <strong>{{ pendingAction.course.name }}</strong>?
                </p>
                <p class="text-sm text-red-600 mb-4" v-if="pendingAction?.type === 'unselect' && pendingAction.course.membership === 'invite_only'">
                    This is an invite-only CCA. You cannot select it back after unselecting.
                </p>
                <p class="py-4" v-else-if="pendingAction?.type === 'replace'">
                    Do you really want to unselect <strong>{{ pendingAction.existing?.name }}</strong> and select <strong>{{ pendingAction.course.name }}</strong>?
                </p>
                <p class="text-sm text-red-600 mb-4" v-if="pendingAction?.type === 'replace' && pendingAction.existing?.membership === 'invite_only'">
                    This is an invite-only CCA. You cannot select it back after unselecting.
                </p>
                <div v-if="(pendingAction?.type === 'unselect' && pendingAction.course.membership === 'invite_only') || (pendingAction?.type === 'replace' && pendingAction.existing?.membership === 'invite_only')" class="mb-4">
                    <label class="block text-sm mb-2" :class="{ 'text-red-600': showInputError }" :style="showInputError ? 'animation: flash 0.2s 7' : ''">Type "I am really sure" to confirm:</label>
                    <input v-model="confirmInput" type="text" class="input input-bordered w-full" :class="{ 'input-error': showInputError }" />
                </div>
                <div class="modal-action">
                    <form method="dialog">
                        <button class="btn" @click="pendingAction = null; confirmInput = ''; showInputError = false">Cancel</button>
                        <button class="btn ml-2" @click.prevent="confirmAction">Confirm</button>
                    </form>
                </div>
            </div>
        </dialog>
    </div>
</template>
