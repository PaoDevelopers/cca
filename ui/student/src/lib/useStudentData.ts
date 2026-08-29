// The student view's data layer: what is loaded, what keeps it fresh,
// and the three writes.
//
// Split out of the component because it is the half that has nothing to
// do with markup — reads, the event socket, the coalescers and the
// focus restoration are a unit, and the component below is easier to
// read when it is only the shape of the page.
//
// Note what is not here: nothing that judges a rule. Whether a course
// may be taken is asked of the server through /student/api/eligibility,
// which answers for the whole catalogue in one call. The browser
// arranges and displays; it does not decide.
import {
	useCallback,
	useEffect,
	useLayoutEffect,
	useMemo,
	useRef,
	useState,
} from "react"
import { flushSync } from "react-dom"
import { isFull } from "@common/capacity"
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
import { coalesce } from "./coalesce"
import {
	type CourseFilter,
	conflictingEnrollments,
	filterCourses,
	swappable,
	violationsFor,
} from "./enrollment"

interface StudentData {
	user: StudentInfo | null
	grade: Grade | null
	periods: Period[]
	categories: Category[]
	// Categories that some course actually uses, for the filter rail.
	courseCategories: Category[]
	// The whole catalogue after the filter, enrolled courses included.
	catalogue: Course[]
	selected: Course[]
	loading: boolean
	unauthenticated: boolean
	updating: boolean
	// Whether the student's own enrollment window is open.
	windowOpen: boolean
	error: string | null
	dismissError: () => void
	// What just happened, for the page's live region.
	announcement: string
	filter: CourseFilter
	setFilter: (filter: CourseFilter) => void
	enrollmentOf: (course: Course) => Enrollment | null
	violationsOf: (course: Course) => Violation[]
	canSwap: (course: Course) => boolean
	enroll: (course: Course) => void
	drop: (course: Course) => void
	swap: (course: Course) => void
	// Forget where the focus was headed; the student changed page.
	forgetFocus: () => void
}

// Every action button carries its course id, so focus restoration is a
// question about ids rather than about nodes, and survives the whole
// list being re-rendered.
function actionButtons(): HTMLElement[] {
	return [...document.querySelectorAll("[data-course-action]")].filter(
		(node): node is HTMLElement => node instanceof HTMLElement,
	)
}

export function useStudentData(): StudentData {
	const [user, setUser] = useState<StudentInfo | null>(null)
	const [courses, setCourses] = useState<Course[]>([])
	const [periods, setPeriods] = useState<Period[]>([])
	const [categories, setCategories] = useState<Category[]>([])
	const [grades, setGrades] = useState<Grade[]>([])
	const [enrollments, setEnrollments] = useState<Enrollment[]>([])
	const [eligibility, setEligibility] = useState<Eligibility>({})
	const [loading, setLoading] = useState(true)
	const [error, setError] = useState<string | null>(null)
	const [updating, setUpdating] = useState<string | null>(null)
	const [unauthenticated, setUnauthenticated] = useState(false)

	const [filter, setFilter] = useState<CourseFilter>({
		search: "",
		categories: [],
		periods: [],
		hideFull: false,
		hideInviteOnly: true,
		hideIncompatible: true,
		hideConflicting: false,
	})

	// What just happened, for a screen reader.
	//
	// Nothing else says it. The card changes — a new button, the word
	// "Selected", the count moving — and every one of those changes is
	// silent: the only live region on the page was the error popup, so a
	// student who could not see the screen had no way of learning
	// whether their enrolment had gone through.
	const [announcement, setAnnouncement] = useState("")

	// An expired session switches to the signed-out view; anything else
	// pops an error.
	const report = useCallback((err: unknown, fallback: string): void => {
		if (err instanceof AuthError) {
			setUnauthenticated(true)
		} else {
			setError(errorMessage(err, fallback))
		}
	}, [])

	// Each read applied on its own, and one failure reported rather than
	// allowed to discard the rest.
	//
	// This was a Promise.all with every assignment after the await, so a
	// single rejection threw away six good responses and applied
	// nothing. That is worst exactly where it is most likely: this
	// function is also the reconnect repair, so it runs when everyone
	// reconnects at once, which is when a read is most likely to be
	// refused.
	const load = useCallback(async (): Promise<void> => {
		const results = await Promise.allSettled([
			fetchUser().then(setUser),
			fetchCourses().then(setCourses),
			fetchPeriods().then(setPeriods),
			fetchCategories().then(setCategories),
			fetchGrades().then(setGrades),
			fetchEnrollments().then(setEnrollments),
			fetchEligibility().then(setEligibility),
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

		setLoading(false)
	}, [report])

	const pull = useCallback(
		<T>(loader: () => Promise<T>, set: (value: T) => void): void => {
			loader()
				.then(set)
				.catch((err: unknown): void => {
					report(err, "Refresh failed")
				})
		},
		[report],
	)

	// Anything that moves a rule's inputs moves the eligibility map with
	// it, so it is re-read alongside — and coalesced, because a
	// student's own write asks for it twice: once on the write path and
	// once when the server's frame comes back to this session.
	const eligibilityPull = useMemo(
		() =>
			coalesce((): void => {
				pull(fetchEligibility, setEligibility)
			}),
		[pull],
	)

	const userPull = useMemo(
		() =>
			coalesce((): void => {
				pull(fetchUser, setUser)
			}),
		[pull],
	)

	useEffect(
		() => (): void => {
			eligibilityPull.cancel()
			userPull.cancel()
		},
		[eligibilityPull, userPull],
	)

	useEffect((): void => {
		void load()
		// Deliberately once: the socket's onreconnect re-runs it, and a
		// changed `load` identity would otherwise reload the world.
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [])

	useEffect(
		(): (() => void) =>
			connectEvents("/student/api/events", {
				oninvalidate: (resource): void => {
					switch (resource) {
						case "courses":
							pull(fetchCourses, setCourses)
							eligibilityPull.trigger()
							break
						case "categories":
							pull(fetchCategories, setCategories)
							break
						case "periods":
							pull(fetchPeriods, setPeriods)
							eligibilityPull.trigger()
							break
						case "grades":
							// Includes the window opening or closing: the
							// server wakes at each boundary so an open
							// page repaints then rather than at its next
							// action.
							pull(fetchGrades, setGrades)
							userPull.trigger()
							break
						case "enrollments":
							pull(fetchEnrollments, setEnrollments)
							userPull.trigger()
							eligibilityPull.trigger()
							break
						case "students":
							userPull.trigger()
							eligibilityPull.trigger()
							break
					}
				},
				oncoursecount: (courseID, currentStudents): void => {
					// Fullness is a rule, and the rules are the server's:
					// "capacity" is one of the violations the eligibility
					// map carries. Re-read it only on the crossing, not
					// on every count — under a busy window these arrive
					// ten a second.
					setCourses((previous): Course[] => {
						const before = previous.find(
							(c): boolean => c.id === courseID,
						)
						const wasFull = before !== undefined && isFull(before)
						const nowFull =
							before !== undefined &&
							isFull({
								current_students: currentStudents,
								max_students: before.max_students,
							})
						if (wasFull !== nowFull) {
							eligibilityPull.trigger()
						}
						return previous.map((c): Course =>
							c.id === courseID
								? { ...c, current_students: currentStudents }
								: c,
						)
					})
				},
				// Whatever changed while the socket was down was never
				// delivered, and there is no way to find out what it was.
				onreconnect: (): void => {
					void load()
				},
				ongiveup: (reason: string): void => {
					setError(reason)
				},
			}),
		[eligibilityPull, userPull, pull, load],
	)

	const enrollmentByCourse = useMemo(
		() =>
			new Map(
				enrollments.map((s): [string, Enrollment] => [s.course_id, s]),
			),
		[enrollments],
	)

	const selectedCourses = useMemo(
		() => courses.filter((c): boolean => enrollmentByCourse.has(c.id)),
		[courses, enrollmentByCourse],
	)

	const gradeInfo = useMemo((): Grade | null => {
		if (user === null) {
			return null
		}
		const id = user.grade_id
		return grades.find((g): boolean => g.id === id) ?? null
	}, [user, grades])

	// The gate on every student write. Derived by the server from the
	// window's two bounds and read from here, never recomputed.
	const windowOpen = gradeInfo?.is_open ?? false

	const violations = useCallback(
		(course: Course): Violation[] => violationsFor(eligibility, course),
		[eligibility],
	)

	// The catalogue is the whole catalogue, enrolled courses included.
	// Taking one used to remove it, which meant the card a student had
	// just acted on vanished under their cursor and the only way to see
	// what they held was to change tab. It stays, marked as theirs, with
	// Drop where Enroll was.
	const catalogue = useMemo(
		() =>
			filterCourses(courses, filter, violations, (course): boolean =>
				enrollmentByCourse.has(course.id),
			),
		[courses, filter, violations, enrollmentByCourse],
	)

	// Distinct categories across the visible catalogue, for the facet.
	const courseCategories = useMemo(
		() =>
			categories.filter((category): boolean =>
				courses.some((c): boolean => c.category_id === category.id),
			),
		[categories, courses],
	)

	// Where the focus goes after a write.
	//
	// Not "back to the button that was pressed": a successful enrolment
	// moves the course out of Available entirely, so that button no
	// longer exists. The useful place is where the gap is — the course
	// that took its position, or the one before it if it was last.
	//
	// Held in a ref and applied from a layout effect rather than kept in
	// state. That is the whole of the fix for a race the Svelte version
	// had: it restored focus from an ordinary effect, after paint, so
	// the intermediate frame — where the acted-on button is still
	// mounted and takes the focus itself — was observable, and a test
	// that sampled the focus right after the click saw the wrong button
	// about two runs in three. A layout effect runs before the browser
	// paints, so no intermediate focus is ever shown or observed.
	const refocus = useRef<{ course: string; before: string[] } | null>(null)

	useLayoutEffect((): void => {
		const intent = refocus.current
		// Only when the focus is on <body>: if the student has since put
		// it somewhere themselves, that is theirs and this does not take
		// it.
		if (intent === null || document.activeElement !== document.body) {
			return
		}

		const after = actionButtons()
		const survivor = after.find(
			(node): boolean => node.dataset["courseAction"] === intent.course,
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
		const index = intent.before.indexOf(intent.course)
		if (index >= 0 && after.length > 0) {
			after[Math.min(index, after.length - 1)]?.focus()
			return
		}

		// Nothing left to move to — the last course was just dropped, or
		// a filter no longer matches anything. The heading names the
		// list and is where a reader would start again.
		//
		// A descendant selector, not a child one: the heading shares a
		// row with the view toggle, so it is a div deeper than it was.
		// As a child selector this silently matched nothing and the
		// focus stayed on <body>.
		document.querySelector<HTMLElement>("section h2[tabindex]")?.focus()
	})

	// The three student writes share everything but the call itself:
	// each answers with the resulting enrollment set, and each moves
	// what the student may do next, so the eligibility map and their own
	// standing are re-read afterwards.
	const write = useCallback(
		async (
			course: Course,
			action: () => Promise<Enrollment[]>,
			fallback: string,
			succeeded: string,
		): Promise<void> => {
			if (updating !== null) {
				return
			}

			const before = actionButtons().map(
				(node): string => node.dataset["courseAction"] ?? "",
			)

			// Let go of the pressed button before the request goes out.
			//
			// Disabling it is not enough: Chrome leaves a disabled
			// element as document.activeElement, so the focus sits on a
			// control that cannot be operated and is about to be
			// unmounted. Focus stranded on a disabled control is a wart
			// in its own right — a screen reader announces a button
			// nobody can press — and it is also what made the restore
			// unobservable, since for the width of the request the
			// focused element was still the acted-on course.
			//
			// Blurring hands it to <body>, which is where the layout
			// effect below expects to find it, so the restore runs once
			// and lands on the course that took this one's place.
			const pressed = document.activeElement
			if (
				pressed instanceof HTMLElement &&
				pressed.dataset["courseAction"] === course.id
			) {
				pressed.blur()
			}

			flushSync((): void => {
				setUpdating(course.id)
			})
			try {
				// The enrollment list comes straight back, so it is
				// never stale. What it implies — eligibility and
				// standing — is re-read through the coalescers.
				setEnrollments(await action())
				eligibilityPull.trigger()
				userPull.trigger()
				setAnnouncement(`${succeeded} ${course.name}.`)
			} catch (err) {
				// Refusals are announced by the error popup, which is
				// already a live region and is assertive because a
				// refusal is not something to mention in passing.
				report(err, fallback)
			} finally {
				setUpdating(null)
				refocus.current = { course: course.id, before }
			}
		},
		[updating, eligibilityPull, userPull, report],
	)

	const enroll = useCallback(
		(course: Course): void => {
			void write(
				course,
				() => enrollInCourse(course.id),
				"Enrollment failed",
				"Enrolled in",
			)
		},
		[write],
	)

	const drop = useCallback(
		(course: Course): void => {
			void write(
				course,
				() => dropCourse(course.id),
				"Drop failed",
				"Dropped",
			)
		},
		[write],
	)

	// One operation, not a drop followed by an enroll: the new course is
	// judged with the replaced ones disregarded, which is the only way
	// to move between two courses that clash.
	const swap = useCallback(
		(course: Course): void => {
			const replacing = conflictingEnrollments(eligibility, course)
			void write(
				course,
				() => swapIntoCourse(course.id, replacing),
				"Swap failed",
				"Swapped into",
			)
		},
		[eligibility, write],
	)
	// Hoisted rather than written inline in the object below: a hook
	// called from inside a return literal is legal only for as long as
	// nothing returns early above it, and neither the compiler nor the
	// hook lint rule can see the difference.
	const dismissError = useCallback((): void => {
		setError(null)
	}, [])

	const forgetFocus = useCallback((): void => {
		refocus.current = null
	}, [])

	const enrollmentOf = useCallback(
		(course: Course): Enrollment | null =>
			enrollmentByCourse.get(course.id) ?? null,
		[enrollmentByCourse],
	)

	const canSwap = useCallback(
		(course: Course): boolean =>
			swappable(eligibility, course, enrollments),
		[eligibility, enrollments],
	)

	return {
		user,
		grade: gradeInfo,
		periods,
		categories,
		courseCategories,
		catalogue,
		selected: selectedCourses,
		loading,
		unauthenticated,
		updating: updating !== null,
		windowOpen,
		error,
		dismissError,
		announcement,
		filter,
		setFilter,
		enrollmentOf,
		violationsOf: violations,
		canSwap,
		enroll,
		drop,
		swap,
		forgetFocus,
	}
}
