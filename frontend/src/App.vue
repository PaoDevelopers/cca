<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'
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
const isUpdatingSelection = ref(false)
const selectionPageRef = ref<{ loadPeriods: () => Promise<void> } | null>(null)
const grades = ref<any[]>([])
const periods = ref<string[]>([])
let eventSource: EventSource | null = null

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
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(courseId)
        })
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
        fetchJson<Course[]>('/student/api/courses', {credentials: 'include'}),
        fetchJson<SelectionResponse[] | null>('/student/api/my_selections', {credentials: 'include'})
    ])
    ccas.value = coursesData.map((course: Course) => ({
        ...course,
        current_students: course.current_students ?? 0,
        selected: false
    }))
    applySelections(selectionsData)
}

const loadGrades = async () => {
    const gradesRes = await fetch('/student/api/grades', {credentials: 'include'})
    grades.value = await gradesRes.json()
}

const loadPeriods = async () => {
    const periodsRes = await fetch('/student/api/periods', {credentials: 'include'})
    periods.value = await periodsRes.json()
}

onMounted(async () => {
    try {
        userInfo.value = await fetchJson<Student>('/student/api/user_info', {credentials: 'include'})
        await Promise.all([loadCourses(), loadGrades(), loadPeriods()])

        eventSource = new EventSource('/student/api/events')
        eventSource.addEventListener('invalidate_periods', async () => {
            await loadCourses()
            await loadPeriods()
        })
        eventSource.addEventListener('invalidate_courses', async () => {
            await loadCourses()
        })
        eventSource.addEventListener('invalidate_categories', async () => {
            await loadCourses()
        })
        eventSource.addEventListener('invalidate_grades', loadGrades)
        eventSource.onerror = () => {
            eventSource?.close()
            eventSource = null
        }
    } catch (err) {
        errorMessage.value = err instanceof Error ? err.message : 'Failed to load data.'
    }
})

const toggleCCA = async (id: string) => {
    const course = ccas.value.find((c: CourseWithSelection) => c.id === id)
    if (!course || isUpdatingSelection.value) return

    isUpdatingSelection.value = true
    errorMessage.value = null

    try {
        if (course.selected) {
            await requestSelectionUpdate('DELETE', course.id)
            return
        }

        const existingSelection = ccas.value.find(c => c.period === course.period && c.selected)
        if (existingSelection) {
            const removed = await requestSelectionUpdate('DELETE', existingSelection.id)
            if (!removed) {
                return
            }
        }

        await requestSelectionUpdate('PUT', course.id)
    } finally {
        isUpdatingSelection.value = false
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
    if (eventSource) {
        eventSource.close()
        eventSource = null
    }
}

if (typeof window !== 'undefined') {
    window.addEventListener('beforeunload', cleanup)
}
</script>

<template>
    <div class="min-h-screen bg-white flex flex-col">
        <header class="border-b border-gray-200 bg-white/80 backdrop-blur-sm sticky top-0 z-50">
            <div class="flex justify-between items-center px-8 py-5">
                <h1 class="text-xl font-light tracking-wide">CCA Selection</h1>
                <div v-if="userInfo" class="flex items-center gap-3 text-sm">
                    <span class="text-gray-900 font-medium">{{ userInfo.name }}</span>
                    <span class="text-gray-400">·</span>
                    <span class="text-gray-600">{{ userInfo.grade }}</span>
                    <span class="text-gray-400">·</span>
                    <span class="text-gray-600">ID: {{ userInfo.id }}</span>
                </div>
            </div>
        </header>

        <Transition name="fade">
            <div v-if="errorMessage" class="toast toast-top toast-center z-[60]">
                <div role="alert" class="alert alert-error">
                    <svg xmlns="http://www.w3.org/2000/svg" class="h-6 w-6 shrink-0 stroke-current" fill="none" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                    <span>{{ errorMessage }}</span>
                </div>
            </div>
        </Transition>

        <div class="border-b border-gray-200 bg-white">
            <div class="flex justify-between items-center px-8 py-4">
                <div class="flex gap-12">
                    <button
                        @click="activeTab = 'Selection'"
                        class="text-sm pb-2 transition-colors"
                        :class="activeTab === 'Selection' ? 'border-b-2 border-[#5bae31] text-[#5bae31]' : 'text-gray-500 hover:text-gray-900'"
                    >
                        Selection
                    </button>
                    <button
                        @click="activeTab = 'Review'"
                        class="text-sm pb-2 transition-colors"
                        :class="activeTab === 'Review' ? 'border-b-2 border-[#5bae31] text-[#5bae31]' : 'text-gray-500 hover:text-gray-900'"
                    >
                        Review
                    </button>
                </div>
                <div class="flex gap-2 items-center">
                    <select v-model="searchScope" class="text-xs border border-gray-300 rounded px-2 py-1.5">
                        <option value="global">Search globally</option>
                        <option value="period" v-if="currentPeriod">Search in {{ currentPeriod }}</option>
                    </select>
                    <input v-model="searchQuery" type="text" placeholder="Search CCAs..."
                           class="text-sm border border-gray-300 rounded px-3 py-1.5 w-64"/>
                </div>
            </div>
        </div>

        <SelectionPage v-if="activeTab === 'Selection'" ref="selectionPageRef" :ccas="filteredCCAs"
                       :search-active="searchScope === 'global' && !!searchQuery" :user-grade="userInfo?.grade"
                       :grades="grades" :periods="periods" :initial-period="currentPeriod" :initial-view-mode="viewMode" @toggle="toggleCCA" @period-change="currentPeriod = $event" @view-mode-change="viewMode = $event"/>
        <ReviewPage v-else :ccas="ccas" :user-grade="userInfo?.grade" :grades="grades"/>

        <footer class="border-t border-gray-200 bg-white py-4 text-center text-sm text-gray-600">
            Written by Runxi Yu and Henry Yang
        </footer>
    </div>
</template>
