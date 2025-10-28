<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import SelectionPage from './pages/SelectionPage.vue'
import ReviewPage from './pages/ReviewPage.vue'
import type { Course, Student, Choice } from './types'

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
const errorMessage = ref<string | null>(null)
const isUpdatingSelection = ref(false)

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
  return res.json() as Promise<T>
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
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(courseId)
    })
    if (!res.ok) {
      throw new Error(await extractErrorMessage(res))
    }
    const selections = await res.json() as SelectionResponse[] | null
    applySelections(selections)
    errorMessage.value = null
    return true
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : 'Unable to update selections.'
    return false
  }
}

onMounted(async () => {
  try {
    const [coursesData, userData, selectionsData] = await Promise.all([
      fetchJson<Course[]>('/student/api/courses', { credentials: 'include' }),
      fetchJson<Student>('/student/api/user_info', { credentials: 'include' }),
      fetchJson<SelectionResponse[] | null>('/student/api/my_selections', { credentials: 'include' })
    ])
    ccas.value = coursesData.map((course: Course) => ({
      ...course,
      current_students: course.current_students ?? 0,
      selected: false
    }))
    userInfo.value = userData
    applySelections(selectionsData)
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

    <div v-if="errorMessage" class="px-8 mt-4">
      <div class="border border-red-200 bg-red-50 text-red-700 rounded-lg px-4 py-3">
        <p class="font-semibold">We hit a problem</p>
        <p class="text-sm mt-1">{{ errorMessage }}</p>
      </div>
    </div>

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
          <input v-model="searchQuery" type="text" placeholder="Search CCAs..." class="text-sm border border-gray-300 rounded px-3 py-1.5 w-64" />
        </div>
      </div>
    </div>

    <SelectionPage v-if="activeTab === 'Selection'" :ccas="filteredCCAs" :search-active="searchScope === 'global' && !!searchQuery" :user-grade="userInfo?.grade" @toggle="toggleCCA" @period-change="currentPeriod = $event" />
    <ReviewPage v-else :ccas="ccas" />
  </div>
</template>
