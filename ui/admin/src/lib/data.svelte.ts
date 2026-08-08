// The admin app's shared data layer: one copy of every resource, one
// event WebSocket, and one place that knows how to reload a resource.
//
// Pages used to hold their own copies and open their own socket, which
// meant a route change reconnected and the pages that share a resource
// each fetched it separately. They declare what they need through
// want() instead, and read it from here.

import {
	fetchAdminCourses,
	fetchCategories,
	fetchEnrollments,
	fetchGrades,
	fetchPeriods,
	fetchStudentStatus,
	fetchStudents,
	type AdminStudent,
	type StudentStatus,
} from "@common/adminApi"
import { connectEvents } from "@common/events"
import { AuthError, batchErrorMessage, violationsOf } from "@common/http"
import type {
	Category,
	Course,
	Enrollment,
	Grade,
	Period,
	Violation,
} from "@common/types"
import { createContext, tick } from "svelte"
import { SvelteSet } from "svelte/reactivity"

const resourceNames = [
	"categories",
	"periods",
	"grades",
	"courses",
	"students",
	"enrollments",
] as const

export type Resource = (typeof resourceNames)[number]

// How long the focus restore keeps watching. Long enough to outlast
// the invalidation frame the write provokes, short enough that it is
// over before anybody could have moved the focus deliberately.
const focusRestoreFrames = 12

function nextFrame(): Promise<void> {
	return new Promise<void>((resolve): void => {
		requestAnimationFrame((): void => {
			resolve()
		})
	})
}

// The server names resources in its invalidation frames; anything we
// do not recognise is ignored rather than trusted.
function isResource(value: string): value is Resource {
	return (resourceNames as readonly string[]).includes(value)
}

export class AdminData {
	public categories = $state.raw<Category[]>([])
	public periods = $state.raw<Period[]>([])
	public grades = $state.raw<Grade[]>([])
	public courses = $state.raw<Course[]>([])
	public students = $state.raw<AdminStudent[]>([])
	public enrollments = $state.raw<Enrollment[]>([])

	// Advisory standing, read from the server rather than derived
	// here: requirement satisfaction is a rule, and the rules have one
	// definition. Loaded with "students", because that is the only
	// page that shows it.
	public studentStatus = $state.raw<StudentStatus[]>([])

	// Most callers want the IDs alone, in display order.
	public periodIDs = $derived(this.periods.map((p): string => p.id))
	public categoryIDs = $derived(this.categories.map((c): string => c.id))

	public error = $state<string | null>(null)

	// A write the server refused because it would break negotiable
	// rules, waiting for the administrator to confirm or abandon it.
	// One field and one dialog for every such write: the protocol is
	// the same wherever it surfaces, so it is presented in one place
	// rather than re-implemented per form.
	public pendingViolations = $state<PendingViolations | null>(null)
	// Whether any write is outstanding.
	//
	// Disable *buttons* with this, so an action is not repeated by
	// accident. Do not disable inputs, selects or textareas with it: a
	// value the user has changed is an instruction, and a control that
	// disables itself between the mouse going down and the click
	// resolving swallows that instruction without a request, an error,
	// or any other sign it happened. Committing one field by clicking
	// on another — which is how every one of these forms is used — hits
	// exactly that window. Serialising the writes below is what makes
	// leaving the editors live safe.
	public busy = $state(false)
	public unauthenticated = $state(false)

	// Writes run one at a time, in the order they were asked for, so
	// two edits in flight cannot interleave their read-modify-write of
	// the same resource. run() used to return early while another write
	// was outstanding, which silently discarded the second one.
	#queue: Promise<void> = Promise.resolve()
	#outstanding = 0

	// The control that started the current run of writes.
	//
	// Disabling a focused button moves the focus to <body>, and
	// re-enabling it does not bring the focus back — so an
	// administrator reordering ten grades by keyboard began again from
	// the top of the document ten times, and a screen reader announced
	// nothing at all about where they now were. Recorded when the first
	// write starts and put back when the last one finishes, in one
	// place rather than in every form.
	#focused: HTMLElement | null = null

	// Everything this tab holds a copy of, however it came to hold it.
	//
	// Not "everything a page asked for": a write names what it touched
	// and run() refreshes that, so a resource can arrive without any
	// page ever having wanted it. Saving a course refreshes
	// enrollments; saving a grade refreshes students. Those copies
	// were kept and then never invalidated again, because the
	// invalidation frame was tested against the wanted set and the
	// page-load path was tested against this one. Navigating to
	// Enrollments after saving a course therefore issued no request at
	// all and showed a table that had been wrong since the save.
	readonly #loaded = new SvelteSet<Resource>()

	// What the page currently on screen reads, which is a smaller set
	// and the one an invalidation frame should act on.
	//
	// Refreshing everything ever wanted meant an administrator who had
	// visited Students and Enrollments and then navigated to Courses
	// kept re-reading both, in full, at the hub's coalescing rate, for
	// data nothing was displaying: measured at ten enrollment reads and
	// ten status reads per second for ninety seconds while students
	// enrolled. The cost only ever ratchets up over a working day.
	//
	// What is not on screen is marked stale instead and re-read when a
	// page next asks for it — so the caching this all exists for is
	// kept, without the traffic.
	#live = new Set<Resource>()
	readonly #stale = new Set<Resource>()
	// A reload already running, so callers asking for the same resource
	// at once share it. The refetch after a write and the invalidation
	// frame that write provokes usually collapse this way, but not
	// always: the server coalesces frames over a short window, so a
	// refetch that finishes before the frame arrives means a second
	// request. That costs a read, never correctness.
	readonly #inFlight = new Map<Resource, Promise<void>>()

	// Asked for again while a read was already out.
	//
	// Sharing the read in flight is only sound when it can still
	// contain the answer. It was issued at some earlier instant, so a
	// change committed after that is not in it — and joining it marked
	// the resource loaded and current while holding a snapshot that
	// predates the change, with nothing left to correct it. The window
	// is exactly one read's duration, which is longest when reads are
	// slow, which is when the most is happening. Read again instead,
	// once the one in flight lands.
	readonly #refetch = new Set<Resource>()

	readonly #fetchers: Record<Resource, () => Promise<void>> = {
		categories: async (): Promise<void> => {
			this.categories = await fetchCategories()
		},
		periods: async (): Promise<void> => {
			this.periods = await fetchPeriods()
		},
		grades: async (): Promise<void> => {
			this.grades = await fetchGrades()
		},
		courses: async (): Promise<void> => {
			this.courses = await fetchAdminCourses()
		},
		students: async (): Promise<void> => {
			// Both in one go: the table shows a student and their
			// standing side by side, and two separate resources would
			// let the row disagree with itself.
			const [students, status] = await Promise.all([
				fetchStudents(),
				fetchStudentStatus(),
			])
			this.students = students
			this.studentStatus = status
		},
		enrollments: async (): Promise<void> => {
			this.enrollments = await fetchEnrollments()
			// Standing is a function of enrollments, so it goes stale
			// whenever they move.
			if (this.#loaded.has("students")) {
				this.studentStatus = await fetchStudentStatus()
			}
		},
	}

	// Declares what a page reads. Anything not loaded yet is fetched;
	// anything already here is reused as-is.
	// Declares what a page reads. Anything not loaded, or gone stale
	// while some other page was on screen, is fetched; anything already
	// here and still current is reused as-is.
	//
	// One call per page, naming everything it reads, so the argument
	// list is also the set that is on screen.
	public want(...resources: Resource[]): void {
		this.#live = new Set(resources)

		for (const resource of resources) {
			if (!this.#loaded.has(resource) || this.#stale.has(resource)) {
				this.#stale.delete(resource)
				void this.#load(resource, "Failed to load")
			}
		}
	}

	// Whether every named resource has arrived at least once. A page
	// shows its loading state until then.
	public ready(...resources: Resource[]): boolean {
		return resources.every((resource): boolean =>
			this.#loaded.has(resource),
		)
	}

	public async refresh(...resources: Resource[]): Promise<void> {
		await Promise.all(
			resources.map((resource): Promise<void> =>
				this.#load(resource, "Refresh failed"),
			),
		)
	}

	// Runs a write, then reloads what it touched. The reload is not
	// left to the invalidation frame alone: a dropped socket must not
	// leave the page showing what the user just changed away from.
	//
	// Reports whether the state actually moved. A control whose value
	// the browser owns — a bare `value=` on an input, with no binding
	// — keeps whatever was typed into it when the write is refused,
	// because the expression behind it never changed and Svelte has no
	// reason to touch the DOM. The box then shows a number the server
	// rejected, indefinitely, next to a grade that does not have it.
	public run(
		action: () => Promise<void>,
		...touched: Resource[]
	): Promise<boolean> {
		// Synchronous, so the controls disable on this click rather
		// than after the request has already been sent.
		if (this.#outstanding === 0) {
			this.#holdFocus()
		}

		this.#outstanding++
		this.busy = true

		const next = this.#queue.then(async (): Promise<boolean> => {
			this.error = null
			try {
				await action()
				await this.refresh(...touched)
				return true
			} catch (err) {
				this.report(err, "Request failed")
				return false
			} finally {
				this.#outstanding--
				if (this.#outstanding === 0) {
					this.busy = false
					void this.#returnFocus()
				}
			}
		})

		// The callback above swallows its own failures, so the chain
		// cannot be poisoned by one write and stall every later one.
		this.#queue = next.then((): void => undefined)

		return next
	}

	#holdFocus(): void {
		const active = document.activeElement
		this.#focused = active instanceof HTMLElement ? active : null
	}

	// After a tick, because the button is re-enabled by a state change
	// and focus cannot be given to a control that is still disabled.
	//
	// isConnected because a write often re-renders the form its button
	// lived in. A detached node accepts focus() and does nothing with
	// it, which is the same as not restoring but harder to notice.
	//
	// And repeatedly over the next few frames, because the write is not
	// the last thing that redraws the page: the server's invalidation
	// frame arrives over the websocket a moment later and redraws it
	// again, replacing the button and dropping the focus back to
	// <body>. Restoring once won the race about half the time.
	//
	// Only ever from <body>: once the administrator has put the focus
	// somewhere themselves, it is theirs.
	async #returnFocus(): Promise<void> {
		const target = this.#focused
		this.#focused = null
		if (target === null) {
			return
		}

		await tick()

		for (let frame = 0; frame < focusRestoreFrames; frame++) {
			if (document.activeElement === document.body) {
				// The control itself if it is still there. If it is not
				// — a save that closed its own editor, a delete that
				// removed its own row — then <main>, which at least
				// continues the tab order inside the page instead of
				// restarting it at the top of the document.
				const fallback = document.querySelector<HTMLElement>("main")
				const landing = target.isConnected ? target : fallback
				landing?.focus()
			}

			await nextFrame()
		}
	}

	// Runs a write that may come back with violations. The first
	// attempt accepts nothing: an administrator must be shown what
	// they are about to break before they agree to it. If the server
	// refuses, the codes it named are put to them, and only what they
	// confirm is sent back — accepting the whole set, not "override
	// everything", so a violation that appears between the two
	// attempts still stops the write.
	public async runAccepting(
		action: (accept: string[]) => Promise<void>,
		...touched: Resource[]
	): Promise<boolean> {
		// Abandoning the dialog is neither success nor failure: nothing
		// was written and there is nothing to report, but the caller
		// still needs to know the state did not move, because a box
		// showing the value that was not applied is a lie.
		let abandoned = false

		const ran = await this.run(
			async (): Promise<void> => {
				try {
					await action([])
				} catch (err) {
					const violations = violationsOf(err)
					if (violations.length === 0) {
						throw err
					}
					if (!(await this.#confirm(violations))) {
						abandoned = true
						return
					}
					await action(violations.map((v): string => v.code))
				}
			},
			...touched,
		)

		return ran && !abandoned
	}

	// Resolves when the administrator answers the dialog.
	//
	// The busy flag is dropped for the wait and taken back afterwards.
	// Nothing is in flight while a person is reading a list of rules
	// they are about to break, and "busy" is the app's word for a
	// request being outstanding — held across the dialog it disabled
	// every button in the app for as long as somebody took to decide,
	// including the button that opened the dialog. That last one
	// matters beyond appearances: <dialog> restores focus to whatever
	// opened it, and cannot restore focus to a disabled control, so
	// answering left the administrator on <body>.
	//
	// Nothing else can start a write in the meantime: the dialog is
	// modal, which is a stronger interlock than the flag was.
	#confirm(violations: Violation[]): Promise<boolean> {
		const held = this.#outstanding
		this.#outstanding = 0
		this.busy = false

		// #focused is deliberately left as it is: it still names the
		// button that started this write, and the finally in run() will
		// put the focus back there once the answer has been dealt with.
		// Restoring it here would only lose it again the moment the
		// dialog took the focus.
		return new Promise<boolean>((resolve): void => {
			this.pendingViolations = {
				violations,
				answer: (accepted: boolean): void => {
					this.pendingViolations = null
					this.#outstanding = held
					this.busy = held > 0
					resolve(accepted)
				},
			}
		})
	}

	// An expired session switches the whole app to the signed-out
	// view; anything else pops an error.
	public report(err: unknown, fallback: string): void {
		if (err instanceof AuthError) {
			this.unauthenticated = true
		} else {
			// batchErrorMessage lists every malformed element of a
			// rejected batch, so a spreadsheet is fixed in one pass.
			// Violations are not handled here: they are a question for
			// the caller to put to the user, not an error to display.
			this.error = batchErrorMessage(err, fallback)
		}
	}

	// Opens the one event socket. Returns its teardown, so the caller
	// can hand it straight to an effect.
	public connect(): () => void {
		return connectEvents("/admin/api/events", {
			oninvalidate: (resource): void => {
				if (!isResource(resource)) {
					return
				}

				// Reload only what is actually on screen. The comment
				// here used to say exactly this while the condition
				// tested every resource this tab had ever displayed,
				// which never shrinks.
				if (this.#live.has(resource)) {
					void this.refresh(resource)

					return
				}

				// Otherwise remember that the copy we hold is behind,
				// so returning to the page that reads it re-reads it
				// rather than showing what it had months ago.
				if (this.#loaded.has(resource)) {
					this.#stale.add(resource)
				}
			},
			oncoursecount: (courseID, currentStudents): void => {
				this.courses = this.courses.map((c): Course =>
					c.id === courseID
						? { ...c, current_students: currentStudents }
						: c,
				)
			},
			// Whatever changed while the socket was down was never
			// delivered, and there is no way to find out what it was.
			// Only what something is displaying, as everywhere else
			// here.
			// Everything wanted, not only what is on screen: the gap
			// may have moved anything, and marking the rest stale is
			// the same repair a page change would do.
			onreconnect: (): void => {
				void this.refresh(...this.#live)
				for (const resource of this.#loaded) {
					if (!this.#live.has(resource)) {
						this.#stale.add(resource)
					}
				}
			},
			// Through the error channel, because that is what it is: a
			// page that will not update again is showing an answer that
			// was true once, and the administrator has to be told
			// rather than left to notice.
			ongiveup: (reason: string): void => {
				this.error = reason
			},
		})
	}

	#load(resource: Resource, fallback: string): Promise<void> {
		const existing = this.#inFlight.get(resource)
		if (existing !== undefined) {
			this.#refetch.add(resource)
			return existing
		}
		const fetcher = this.#fetchers[resource]
		const pending = fetcher()
			.then((): void => {
				this.#loaded.add(resource)
			})
			.catch((err: unknown): void => {
				this.report(err, fallback)
			})
			.finally((): void => {
				this.#inFlight.delete(resource)
				if (this.#refetch.delete(resource)) {
					void this.#load(resource, fallback)
				}
			})
		this.#inFlight.set(resource, pending)
		return pending
	}
}

export interface PendingViolations {
	violations: Violation[]
	answer: (accepted: boolean) => void
}

export const [getAdminData, setAdminData] = createContext<AdminData>()
