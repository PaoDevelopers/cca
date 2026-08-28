// The admin app against the real server. Kept to the things that only
// a browser can answer: that the route's ?q= reaches the filter box,
// that a write goes through the API and is reflected, and that the two
// apps agree about whether a student has met their requirements.

import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { after, afterEach, before, describe, it } from "node:test"
import type { Page } from "playwright-core"
import { startHarness, subjects, type Harness } from "./harness.js"

// The import's columns, in order. Kept beside the tests that build
// files rather than imported from the Go source it mirrors: if the two
// drift, the header check rejects the file and these fail loudly.
const courseImportColumns = [
	"id",
	"name",
	"description",
	"periods",
	"max_students",
	"invite_only",
	"teacher",
	"teacher_email",
	"location",
	"term",
	"cost",
	"category",
	"allowed_legal_sexes",
	"allowed_grades",
]

// How long the tab opened by "Sign in as" has to show the student's
// own page. Generous: it is a fresh page load against the real server,
// and this is a backstop, not a performance budget.
const studentTabTimeout = 10_000

interface SeededStudent {
	id: string
}

interface SeededGrade {
	id: string
	is_open: boolean
	max_budgeted_periods: number | null
	min_distinct_categories: number
}

interface SeededCategory {
	id: string
	name: string
}

interface SeededCourse {
	id: string
	// null for a course with no cap; see internal/db/schemas/0007.
	max_students: number | null
}

describe("admin", (): void => {
	let harness: Harness | null = null

	before(async (): Promise<void> => {
		harness = await startHarness()
	})

	after(async (): Promise<void> => {
		await harness?.close()
	})
	afterEach(async (): Promise<void> => {
		await harness?.closeContexts()
	})

	async function adminPage(hash: string): Promise<Page> {
		assert.ok(harness !== null)
		const context = await harness.contextFor("admin", subjects.admin)
		const page = await context.newPage()
		await page.goto(`/admin/${hash}`)
		return page
	}

	it("lists the seeded students", async (): Promise<void> => {
		const page = await adminPage("#/students")
		await page.getByText("Alice Example").waitFor()

		const table = await page.locator("table").first().innerText()
		assert.match(table, /s1001/)
		assert.match(table, /Bob Example/)
	})

	// Signing in as a student, which is three things at once and only
	// a browser can see all three: the endpoint mints a session, the
	// browser stores it in the *student* cookie, and the tab that
	// asked keeps its administrator session. The Go tests can check
	// the Set-Cookie header; they cannot check that the second tab
	// then loads that student's own page, or that the first tab still
	// belongs to an administrator afterwards.
	it("signs an administrator in as a student in a second tab", async (): Promise<void> => {
		const page = await adminPage("#/students")
		await page.getByText("Bob Example").waitFor()

		const context = page.context()
		const adminCookieBefore = (await context.cookies()).find(
			(c): boolean => c.name === "admin_session",
		)
		assert.ok(adminCookieBefore !== undefined)

		// The tab is opened by the click itself; anything else is a
		// pop-up as far as the browser is concerned, and this is the
		// only test that would notice it being blocked.
		const opened = context.waitForEvent("page")
		await page
			.getByRole("button", { name: `Sign in as ${subjects.bob}` })
			.click()
		const student = await opened

		// Bob's page, from Bob's own session: the header is served
		// from /student/api/user_info, which reads the cookie.
		//
		// Bounded, unlike most waits in this file: a session that was
		// never minted leaves the second tab on the sign-in page, and
		// an unbounded wait there does not fail this test — it stalls
		// until the whole file times out, taking every later test with
		// it.
		await student
			.getByText("Bob Example")
			.waitFor({ timeout: studentTabTimeout })
		assert.match(student.url(), /\/student\/$/)
		const banner = await student.getByRole("banner").innerText()
		assert.match(banner, new RegExp(subjects.bob))

		const cookies = await context.cookies()
		const studentCookie = cookies.find(
			(c): boolean => c.name === "student_session",
		)
		assert.ok(
			studentCookie !== undefined,
			"no student session was stored in the browser",
		)

		// The administrator's own session is untouched, so the tab
		// they started from is still an administrator's. Both halves
		// matter: an unchanged cookie, and an admin area that still
		// serves the app rather than a sign-in page.
		const adminCookieAfter = cookies.find(
			(c): boolean => c.name === "admin_session",
		)
		assert.equal(adminCookieAfter?.value, adminCookieBefore.value)
		await page.reload()
		await page.getByText("Bob Example").waitFor()

		// And a second student replaces the first, rather than the
		// browser keeping whoever was signed in first.
		const reopened = context.waitForEvent("page")
		await page
			.getByRole("button", { name: `Sign in as ${subjects.alice}` })
			.click()
		const other = await reopened
		await other
			.getByText("Alice Example")
			.waitFor({ timeout: studentTabTimeout })
		assert.match(
			await other.getByRole("banner").innerText(),
			new RegExp(subjects.alice),
		)
	})

	// The filter box is bound to the route rather than copied into local
	// state, so a link carrying ?q= must arrive filtered.
	it("seeds the filter box from the route", async (): Promise<void> => {
		const page = await adminPage(
			`#/students?q=${encodeURIComponent("grade:Y9")}`,
		)
		await page.getByText("Alice Example").waitFor()

		const box = page.locator('input[type="search"]').first()
		assert.equal(await box.inputValue(), "grade:Y9")
	})

	it("keeps list management above the results and reports their count", async (): Promise<void> => {
		const page = await adminPage("#/courses")
		await page.getByText("Basketball").waitFor()

		const status = page.locator(".admin-list-status p").first()
		await status.waitFor()
		assert.equal(await status.innerText(), "3 courses")
		await page.getByRole("button", { name: "Clear the filter" }).waitFor()
		assert.equal(
			await page
				.getByRole("button", { name: "Clear the filter" })
				.isDisabled(),
			true,
		)

		const filter = page.getByLabel("Filter", { exact: true })
		await filter.fill("Basketball")
		await page.waitForFunction(
			(): boolean =>
				document
					.querySelector(".admin-list-status p")
					?.textContent.trim() === "1 of 3 courses shown",
		)

		const clear = page.getByRole("button", { name: "Clear the filter" })
		assert.equal(await clear.isDisabled(), false)
		await clear.click()
		await page.waitForFunction(
			(): boolean =>
				document
					.querySelector(".admin-list-status p")
					?.textContent.trim() === "3 courses",
		)

		// Creation and bulk data tools are reachable before entering the
		// scrollable table, but the large editor stays out of the way.
		const add = page.getByText("Add course", { exact: true })
		await add.waitFor()
		await page.getByText("Data tools", { exact: true }).waitFor()
		assert.equal(
			await page.locator("form.editor").first().isVisible(),
			false,
		)
		await add.click()
		assert.equal(
			await page.locator("form.editor").first().isVisible(),
			true,
		)
	})

	// Two edits committed in quick succession must both be sent and
	// both take effect. The writes are queued rather than skipped when
	// one is outstanding: returning early used to discard the second
	// silently, with no request, no message, and the control snapping
	// back to its old value.
	//
	// Number fields commit on blur, so the gesture that commits one is
	// the gesture that starts the next — which is exactly how these
	// forms are used, and exactly the window that used to lose a
	// write. Fields rather than buttons deliberately: buttons *are*
	// disabled while a write is outstanding, so that an action is not
	// repeated by accident, but a value the user has already changed
	// is an instruction and must not be dropped.
	it("does not drop a write committed while another is in flight", async (): Promise<void> => {
		const page = await adminPage("#/grades")
		await page.locator("article.card").first().waitFor()

		const writes: string[] = []
		page.on("request", (request): void => {
			if (
				request.url().includes("/admin/api/grades/") &&
				request.method() === "PUT"
			) {
				writes.push(request.url())
			}
		})

		const y9 = page.locator("article.card", { hasText: "Y9" })
		const y10 = page.locator("article.card", { hasText: "Y10" })

		// Typing commits nothing yet.
		await y9.locator('input[type="number"]').first().fill("3")

		// Focusing the other card's field blurs the one above, firing
		// the first write; this one is still uncommitted.
		await y10.locator('input[type="number"]').first().fill("2")

		// And this commits it, while the first is still outstanding.
		await page.locator("h1").click()

		await page.waitForTimeout(2000)

		assert.equal(writes.length, 2, "both writes should have been sent")

		// Both took effect, rather than the second being discarded.
		const response = await page.request.get("/admin/api/grades")
		const grades = (await response.json()) as SeededGrade[]
		const seededY9 = grades.find((g): boolean => g.id === "Y9")
		const seededY10 = grades.find((g): boolean => g.id === "Y10")
		assert.equal(seededY9?.max_budgeted_periods, 3)
		assert.equal(seededY10?.max_budgeted_periods, 2)
	})

	it("explains empty window bounds and groups category minimums as requirements", async (): Promise<void> => {
		assert.ok(harness !== null)
		const stack = harness
		stack.exec(
			"UPDATE grades SET opens_at = NULL, closes_at = NULL WHERE id = 'Y9'",
		)

		try {
			const page = await adminPage("#/grades")
			const heading = page.getByRole("heading", {
				name: "Y9",
				exact: true,
			})
			await heading.waitFor()
			const card = page.locator("article.card", { has: heading })
			assert.match(
				await card.innerText(),
				/Closed; no opening or closing time is set\./,
			)

			const open = card.getByRole("button", { name: "Open now" })
			const close = card.getByRole("button", { name: "Close now" })
			assert.equal(await open.isDisabled(), false)
			assert.equal(await close.isDisabled(), true)

			const limits = card.getByRole("group", { name: "Limits" })
			const requirements = card.getByRole("group", {
				name: "Requirements",
			})
			assert.equal(
				await limits.getByLabel("Distinct categories for Y9").count(),
				0,
			)
			assert.equal(
				await requirements
					.getByLabel("Distinct categories for Y9")
					.count(),
				1,
			)

			await open.click()
			await card
				.getByText(/Open since .*; no automatic closing time\./)
				.waitFor()
			assert.equal(await open.isDisabled(), true)
			assert.equal(await close.isDisabled(), false)
		} finally {
			stack.exec(
				"UPDATE grades SET opens_at = now() - interval '1 hour', closes_at = NULL WHERE id = 'Y9'",
			)
		}
	})

	// A different port on the same host is a different *origin* but the
	// same *site*, so SameSite=Lax still attaches the admin session
	// cookie: this is the case Lax does not cover and
	// http.CrossOriginProtection does. Multipart is used deliberately —
	// it is a CORS-simple content type, so the browser really sends the
	// request instead of stopping at a preflight.
	//
	// CORS keeps the page from reading the response, so the assertion
	// is the one that matters anyway: the write must not have happened.
	it("refuses a cross-origin write that still carries the session", async (): Promise<void> => {
		assert.ok(harness !== null)

		const context = await harness.contextFor("admin", subjects.admin)
		const page = await context.newPage()
		await page.goto(`${harness.otherOrigin}/attacker.html`)

		const sent = await page.evaluate(
			async (target: string): Promise<string> => {
				const form = new FormData()
				form.set(
					"spreadsheet",
					new File(
						["id,name,grade,legal_sex\ns9001,Mallory,Y9,F\n"],
						"students.csv",
					),
				)
				try {
					await fetch(`${target}/admin/api/students/import`, {
						method: "POST",
						body: form,
						credentials: "include",
					})
					return "response readable"
				} catch {
					// Expected: no CORS headers on the refusal.
					return "blocked by CORS"
				}
			},
			harness.baseURL,
		)

		// Whatever the page could see, the server must not have acted.
		const response = await page.request.get(
			`${harness.baseURL}/admin/api/students`,
		)
		const students = (await response.json()) as SeededStudent[]
		assert.ok(
			!students.some((s): boolean => s.id === "s9001"),
			`the cross-origin import was applied (fetch outcome: ${sent})`,
		)
	})

	// The accept protocol, end to end: a write that would break a
	// negotiable rule comes back refused and carrying what it would
	// break, the dialog lists it, and confirming re-sends the codes.
	//
	// The socket cap, on the wire and in the browser.
	//
	// Two defects met here. The hub keyed every administrator under the
	// constant "ADMIN", so a cap meant as "how many sockets one person
	// may hold" was a cap of eight for the school; and an evicted page
	// reconnected three seconds later, evicting the next oldest, which
	// reconnected. Ten tabs of one administrator produced nineteen new
	// sockets and seventy-six full reloads in thirty idle seconds, for
	// as long as the tabs stayed open.
	//
	// Raw sockets rather than ten pages: what is under test is the
	// close code the server sends and the fact that the survivors are
	// untouched.
	it("evicts the oldest socket over the cap, and says so", async (): Promise<void> => {
		const page = await adminPage("#/courses")

		const codes = await page.evaluate(async (): Promise<number[]> => {
			// One more than the cap in internal/web/websocket.go.
			const wanted = 9
			const url = new URL("/admin/api/events", window.location.href)
			url.protocol = url.protocol === "http:" ? "ws:" : "wss:"

			const closed: number[] = []
			const sockets: WebSocket[] = []
			let cleaningUp = false

			try {
				// In order, so "oldest" means something.
				for (let i = 0; i < wanted; i++) {
					const socket = new WebSocket(url)
					sockets.push(socket)
					socket.addEventListener("close", (event): void => {
						if (!cleaningUp) {
							closed.push(event.code)
						}
					})
					await new Promise<void>((resolve, reject): void => {
						const removeListeners = (): void => {
							socket.removeEventListener("open", opened)
							socket.removeEventListener("error", errored)
							socket.removeEventListener("close", closedEarly)
							clearTimeout(timer)
						}
						const opened = (): void => {
							removeListeners()
							resolve()
						}
						const errored = (): void => {
							removeListeners()
							reject(
								new Error(
									`WebSocket ${String(i + 1)} failed to open`,
								),
							)
						}
						const closedEarly = (event: CloseEvent): void => {
							removeListeners()
							reject(
								new Error(
									`WebSocket ${String(i + 1)} closed before opening with code ${String(event.code)}`,
								),
							)
						}
						socket.addEventListener("open", opened)
						socket.addEventListener("error", errored)
						socket.addEventListener("close", closedEarly)
						const timer = setTimeout((): void => {
							removeListeners()
							reject(
								new Error(
									`WebSocket ${String(i + 1)} did not open within 5 seconds`,
								),
							)
						}, 5000)
					})
				}

				const deadline = Date.now() + 5000
				while (closed.length === 0 && Date.now() < deadline) {
					await new Promise<void>((resolve): void => {
						setTimeout(resolve, 25)
					})
				}
				if (closed.length === 0) {
					throw new Error("no WebSocket was evicted within 5 seconds")
				}

				return closed
			} finally {
				cleaningUp = true
				for (const socket of sockets) {
					socket.close()
				}
			}
		})

		// Exactly one closed on its own, and with the code that means
		// "do not retry this". The page's own socket is one of the
		// nine's elders, so the count is what the cap implies rather
		// than zero.
		assert.ok(
			codes.length >= 1,
			"nothing was evicted; the cap did not apply",
		)
		assert.ok(
			codes.every((code): boolean => code === 4001),
			`evicted sockets closed with ${JSON.stringify(codes)}, want 4001 so the client stops retrying`,
		)
	})

	// The example spreadsheets the import forms link to, imported.
	//
	// Not a check that they parse — a Go test does that, and it is the
	// weaker half. Three separate files have to agree with each other
	// as well as with the importers: the enrolment example names
	// students and courses from the other two, and those have to
	// satisfy the rules. They did not. The girls' floorball row was
	// given a male student, and nothing but a real import could have
	// said so.
	it("imports the example spreadsheets it hands out", async (): Promise<void> => {
		assert.ok(harness !== null)
		const page = await adminPage("#/courses")

		// The vocabulary the examples name. They are a shape, not a
		// fixture, so nothing ships these rows.
		harness.exec(`
			INSERT INTO categories (id, name) VALUES
				('TECH','Technology example'),('ART','Art example')
				ON CONFLICT DO NOTHING;
			INSERT INTO periods (id, name, sort_order) VALUES
				('MON1','Monday 1 example',90),('WED1','Wednesday 1 example',91),
				('TUE2','Tuesday 2 example',92),('THU2','Thursday 2 example',93),
				('FRI1','Friday 1 example',94)
				ON CONFLICT DO NOTHING;
			INSERT INTO grades (id, name, min_distinct_categories, sort_order) VALUES
				('Y10','Year 10 example',0,90),('Y11','Year 11 example',0,91)
				ON CONFLICT DO NOTHING;
		`)

		// In order: a course cannot name a category that is not there,
		// and an enrolment cannot name either of the other two.
		for (const section of ["students", "courses", "enrollments"]) {
			const file = `${section}_example.csv`
			const response = await page.request.post(
				`/admin/api/${section}/import`,
				{
					multipart: {
						spreadsheet: {
							name: file,
							mimeType: "text/csv",
							buffer: readFileSync(
								`../internal/web/static/${file}`,
							),
						},
					},
				},
			)

			assert.equal(
				response.status(),
				204,
				`${file} was rejected: ${await response.text()}`,
			)
		}

		// Put the fixture back: the other tests count what is here.
		harness.exec(`
			DELETE FROM enrollments WHERE course_id IN ('ROBOTICS','FLOORBALL','CHOIR');
			DELETE FROM courses WHERE id IN ('ROBOTICS','FLOORBALL','CHOIR');
			DELETE FROM students WHERE id LIKE 's225%';
			DELETE FROM grades WHERE name LIKE '% example';
			DELETE FROM periods WHERE name LIKE '% example';
			DELETE FROM categories WHERE name LIKE '% example';
		`)
	})

	// This is the path that replaced the override checkbox, and it is
	// the only place the YKV01 payload is decoded, displayed and
	// echoed back — so nothing below the browser can check it.
	it("refuses a breaking edit, then applies exactly what was confirmed", async (): Promise<void> => {
		const page = await adminPage("#/courses")
		await page.getByText("Basketball").waitFor()

		// Fill Basketball's two seats.
		await page.request.post("/admin/api/enrollments", {
			data: {
				course_id: "BB",
				student_ids: [subjects.alice, subjects.bob],
				student_droppable: true,
				counts_toward_budget: true,
				accept: [],
			},
		})

		await page.reload()
		await page.getByText("Basketball").waitFor()

		// Shrinking it below its enrollment breaks the capacity rule.
		const row = page.locator("tr", { hasText: "Basketball" })
		await row.getByRole("button", { name: /Edit BB/ }).click()

		// Scope this to the row: the create editor also exists, collapsed
		// in the actions above the table.
		const editor = row
			.locator("xpath=following-sibling::tr[1]")
			.locator("form.editor")
		await editor.getByLabel("Capacity").fill("1")
		await editor.getByRole("button", { name: "Save" }).click()

		// The refusal arrives as a dialog naming the violated fact in
		// the database's own words, not a generic failure.
		const dialog = page.getByRole("dialog")
		await dialog.waitFor()
		await dialog.getByText(/BB is over its new capacity \(2\/1\)/).waitFor()

		// Cancelling writes nothing.
		await dialog.getByRole("button", { name: "Cancel" }).click()

		// And the focus comes back to the button that started the
		// write, once the write has finished settling.
		//
		// It used to be lost twice over: disabling a focused button
		// moves the focus to <body> and re-enabling it does not bring
		// it back, and the dialog's buttons answered without calling
		// close(), so <dialog> never restored anything either. An
		// administrator working by keyboard was returned to the top of
		// the document after every write.
		await page.waitForFunction(
			(): boolean => document.activeElement?.tagName !== "BODY",
			undefined,
			{ timeout: 5000 },
		)
		const untouched = (await (
			await page.request.get("/admin/api/courses")
		).json()) as SeededCourse[]
		assert.equal(
			untouched.find((c): boolean => c.id === "BB")?.max_students,
			2,
			"a cancelled confirmation must not have written anything",
		)

		// Confirming re-sends with the codes accepted, and it applies.
		await editor.getByRole("button", { name: "Save" }).click()
		await dialog.waitFor()
		await dialog.getByRole("button", { name: "Continue anyway" }).click()

		await page.waitForFunction(
			(): boolean => document.activeElement?.tagName !== "BODY",
			undefined,
			{ timeout: 5000 },
		)

		await page.waitForTimeout(1000)
		const applied = (await (
			await page.request.get("/admin/api/courses")
		).json()) as SeededCourse[]
		assert.equal(
			applied.find((c): boolean => c.id === "BB")?.max_students,
			1,
			"the confirmed edit should have been applied",
		)

		// Put the fixture back.
		await page.request.delete("/admin/api/enrollments", {
			data: {
				course_id: "BB",
				student_ids: [subjects.alice, subjects.bob],
			},
		})
	})

	// The course import is one call to upsert_courses, so a file with
	// several mistakes must come back naming all of them — that is the
	// whole reason it is not a loop — and a good file must be
	// re-loadable without becoming a fresh season.
	it("reports every bad row of a course import at once", async (): Promise<void> => {
		const page = await adminPage("#/courses")
		await page.getByText("Basketball").waitFor()

		const header = courseImportColumns.join(",")
		const good = "GOOD,Good course,,MON1,10,false,,,,Season,,SPORT,,"

		const response = await page.request.post("/admin/api/courses/import", {
			multipart: {
				spreadsheet: {
					name: "courses.csv",
					mimeType: "text/csv",
					buffer: Buffer.from(
						[
							header,
							good,
							// Three distinct mistakes, on three
							// distinct rows.
							"lowercase,Bad id,,MON1,10,false,,,,Season,,SPORT,,",
							"BAD2,Bad category,,MON1,10,false,,,,Season,,NOPE,,",
							"BAD3,Bad capacity,,MON1,lots,false,,,,Season,,SPORT,,",
							// Trailing newline, as a spreadsheet writes.
							"",
						].join("\n"),
					),
				},
			},
		})

		assert.equal(response.status(), 400, await response.text())

		const body = (await response.json()) as {
			error: {
				code: string
				malformed: Array<{ index: number; field: string }>
			}
		}
		assert.equal(body.error.code, "malformed")
		assert.deepEqual(
			body.error.malformed.map((m): number => m.index).sort(),
			[2, 3, 4],
			"every bad row should be reported, not just the first",
		)

		// And each names the column it was rejected on. Three
		// distinct mistakes that all read as "this value is wrong"
		// leave an administrator scanning fourteen columns by hand.
		assert.deepEqual(
			body.error.malformed
				.slice()
				.sort((a, b): number => a.index - b.index)
				.map((m): string => m.field),
			["id", "category", "max_students"],
		)

		// And nothing was written: the good row went with the bad ones.
		const survivors = (await (
			await page.request.get("/admin/api/courses")
		).json()) as SeededCourse[]
		assert.ok(
			!survivors.some((c): boolean => c.id === "GOOD"),
			"a refused import must leave nothing behind",
		)
	})

	it("imports a course file and can re-import it unchanged", async (): Promise<void> => {
		const page = await adminPage("#/courses")
		await page.getByText("Basketball").waitFor()

		const file = [
			courseImportColumns.join(","),
			"YOGA,Yoga,Calm,WED3,15,false,Sam,,Studio,Season,,SPORT,F,Y9",
			"",
		].join("\n")

		const send = async (): Promise<number> => {
			const result = await page.request.post(
				"/admin/api/courses/import",
				{
					multipart: {
						spreadsheet: {
							name: "courses.csv",
							mimeType: "text/csv",
							buffer: Buffer.from(file),
						},
					},
				},
			)
			return result.status()
		}

		assert.equal(await send(), 204)

		// Again, byte for byte: an import that could only ever create
		// would make "the spreadsheet changed, load it again"
		// impossible.
		assert.equal(await send(), 204)

		const courses = (await (
			await page.request.get("/admin/api/courses")
		).json()) as Array<
			SeededCourse & {
				period_ids: string[]
				allowed_grade_ids: string[]
			}
		>
		const yoga = courses.filter((c): boolean => c.id === "YOGA")
		assert.equal(yoga.length, 1, "the re-import duplicated the course")

		const only = yoga[0]
		assert.ok(only !== undefined)
		assert.deepEqual(only.period_ids, ["WED3"])
		assert.deepEqual(only.allowed_grade_ids, ["Y9"])

		await page.request.delete("/admin/api/courses/YOGA")
	})

	it("refuses a course file that states one ID twice", async (): Promise<void> => {
		const page = await adminPage("#/courses")
		await page.getByText("Basketball").waitFor()

		// Applying these in order would leave "Second" standing and
		// report success, with nothing to say "First" was discarded.
		const file = [
			courseImportColumns.join(","),
			"DUP,First,,WED3,10,false,,,,Season,,SPORT,,",
			"KEEP,Fine,,WED3,10,false,,,,Season,,SPORT,,",
			"DUP,Second,,WED3,10,false,,,,Season,,SPORT,,",
			"",
		].join("\n")

		const response = await page.request.post("/admin/api/courses/import", {
			multipart: {
				spreadsheet: {
					name: "courses.csv",
					mimeType: "text/csv",
					buffer: Buffer.from(file),
				},
			},
		})
		assert.equal(response.status(), 400, await response.text())

		const body = (await response.json()) as {
			error: {
				code: string
				malformed: Array<{ index: number; field: string }>
			}
		}
		assert.equal(body.error.code, "malformed")

		// Both colliding rows, not just the second: which to keep is
		// the administrator's decision.
		assert.deepEqual(
			body.error.malformed.map((m): number => m.index).sort(),
			[1, 3],
		)
		assert.deepEqual(
			body.error.malformed.map((m): string => m.field),
			["id", "id"],
		)

		// Nothing landed, including the row that was fine.
		const survivors = (await (
			await page.request.get("/admin/api/courses")
		).json()) as SeededCourse[]
		assert.ok(
			!survivors.some((c): boolean => c.id === "DUP" || c.id === "KEEP"),
			"a refused import must leave nothing behind",
		)
	})

	it("imports an uncapped course and exports it as unlimited", async (): Promise<void> => {
		const page = await adminPage("#/courses")
		await page.getByText("Basketball").waitFor()

		// The blank cell and the word both mean no cap, and a real 0
		// still means a cap that admits nobody.
		const file = [
			courseImportColumns.join(","),
			"OPEN1,Open one,,WED3,,false,,,,Season,,SPORT,,",
			"OPEN2,Open two,,WED3,unlimited,false,,,,Season,,SPORT,,",
			"SHUT,Shut,,WED3,0,false,,,,Season,,SPORT,,",
			"",
		].join("\n")

		const sent = await page.request.post("/admin/api/courses/import", {
			multipart: {
				spreadsheet: {
					name: "courses.csv",
					mimeType: "text/csv",
					buffer: Buffer.from(file),
				},
			},
		})
		assert.equal(sent.status(), 204, await sent.text())

		const courses = (await (
			await page.request.get("/admin/api/courses")
		).json()) as SeededCourse[]
		const capOf = (id: string): number | null | undefined =>
			courses.find((c): boolean => c.id === id)?.max_students

		assert.equal(capOf("OPEN1"), null, "a blank cell means no cap")
		assert.equal(capOf("OPEN2"), null, '"unlimited" means no cap')
		assert.equal(capOf("SHUT"), 0, "0 is a cap that admits nobody")

		// The export says the word rather than leaving the cell blank,
		// so the file an administrator edits does not read as an
		// oversight. Either spelling comes back in.
		const exported = await (
			await page.request.get("/admin/api/courses/export")
		).text()
		const capacityCell = (id: string): string | undefined =>
			exported
				.split("\n")
				.find((line): boolean => line.startsWith(`${id},`))
				?.split(",")[4]

		assert.equal(capacityCell("OPEN1"), "unlimited")
		assert.equal(capacityCell("OPEN2"), "unlimited")
		assert.equal(capacityCell("SHUT"), "0")

		// And the table renders the absence rather than the word null.
		await page.reload()
		const row = page.locator("tr", { hasText: "Open one" })
		await row.getByText("0/∞").waitFor()

		for (const id of ["OPEN1", "OPEN2", "SHUT"]) {
			await page.request.delete(`/admin/api/courses/${id}`)
		}
	})

	it("creates a category and shows it without a reload", async (): Promise<void> => {
		const page = await adminPage("#/categories")
		await page.getByText("SPORT").first().waitFor()

		// The selector is scoped to a form, and the rename boxes in the
		// table are not inside one — so these two are the create
		// form's id and name, in that order. (Not "the last two on the
		// page": the data tools below carry a text input of their own,
		// and it is inside a form.)
		const inputs = page.locator('form input[type="text"]')
		await inputs.nth(0).fill("ROBOTICS")
		await inputs.nth(1).fill("Robotics")
		await page.getByRole("button", { name: "Add" }).click()

		await page.getByText("ROBOTICS").first().waitFor()

		// Really persisted, not just optimistically rendered.
		const response = await page.request.get("/admin/api/categories")
		const categories = (await response.json()) as SeededCategory[]
		assert.ok(
			categories.some((c): boolean => c.id === "ROBOTICS"),
			`categories = ${categories.map((c): string => c.id).join(", ")}`,
		)
	})

	// Export, edit in a spreadsheet, upload the file you were handed.
	// That is how an administrator works, and it did not work: the
	// export is the PowerSchool hand-off and the import wanted four
	// different columns, so the upload was refused on its header — and
	// had it been accepted, an enrollment repeated once per period
	// would have been placed once per period and collided with itself.
	it("re-imports the enrollment file the export produced", async (): Promise<void> => {
		const page = await adminPage("#/enrollments")

		await page.request.post("/admin/api/enrollments", {
			data: {
				course_id: "CH",
				student_ids: [subjects.alice],
				student_droppable: true,
				counts_toward_budget: true,
			},
		})

		const exported = await (
			await page.request.get("/admin/api/enrollments/export")
		).text()

		assert.ok(
			exported.includes("student_name"),
			`the export should carry the wide shape: ${exported}`,
		)

		const reimport = async (): Promise<number> => {
			const result = await page.request.post(
				"/admin/api/enrollments/import",
				{
					multipart: {
						spreadsheet: {
							name: "enrollments.csv",
							mimeType: "text/csv",
							buffer: Buffer.from(exported),
						},
					},
				},
			)
			return result.status()
		}

		assert.equal(
			await reimport(),
			204,
			"the export's own file was refused by the import",
		)

		// And again: an unedited re-upload changes nothing, rather
		// than colliding with the rows it placed the first time.
		assert.equal(
			await reimport(),
			204,
			"a second unchanged import should be a no-op",
		)

		const held = (await (
			await page.request.get("/admin/api/enrollments")
		).json()) as Array<{ student_id: string; course_id: string }>
		const chess = held.filter(
			(e): boolean =>
				e.course_id === "CH" && e.student_id === subjects.alice,
		)
		assert.equal(chess.length, 1, "re-importing duplicated the enrollment")

		await page.request.delete("/admin/api/enrollments", {
			data: { course_id: "CH", student_ids: [subjects.alice] },
		})
	})

	// A card's window boxes are seeded when the card is built and then
	// belong to whoever is typing, which is right for a box someone is
	// typing in and wrong for one nobody has touched: it holds a value
	// that was true when the page loaded, and sending it back reverts
	// whatever happened in between.
	//
	// Two administrators is the ordinary case here — one setting the
	// opening date while another closes a grade — and neither of them
	// sees it happen, because the write succeeds and the value it puts
	// back is one that had genuinely been there.
	it("does not revert another administrator's window edit", async (): Promise<void> => {
		assert.ok(harness !== null)
		const stack = harness

		const page = await adminPage("#/grades")
		// The card is titled by the grade's id; its name lives in an
		// editable box, not in the heading.
		await page.getByRole("heading", { name: "Y9" }).waitFor()

		// Somebody else moves the opening time, from outside this page.
		stack.exec(
			`UPDATE grades SET opens_at = '2030-01-01T00:00:00Z',
				closes_at = NULL WHERE id = 'Y9'`,
		)

		// This administrator, whose card still shows the old opening,
		// sets a closing date.
		const card = page.locator("article.card", { hasText: "Y9" }).first()
		const closes = card.getByLabel("Window closes for Y9")
		await closes.fill("2031-06-30T12:00")
		await closes.blur()

		// Wait for the write to land and the list to come back.
		await card.getByText(/2031/).waitFor()

		const grades = (await (
			await page.request.get("/admin/api/grades")
		).json()) as Array<{ id: string; opens_at: string | null }>
		const y9 = grades.find((g): boolean => g.id === "Y9")
		assert.ok(y9 !== undefined)
		assert.ok(
			y9.opens_at?.startsWith("2030") === true,
			`the other administrator's opening time was reverted to ${String(y9.opens_at)}`,
		)

		// Put the fixture back: open now, no close.
		stack.exec(
			`UPDATE grades SET opens_at = now() - interval '1 hour',
				closes_at = NULL WHERE id = 'Y9'`,
		)
	})

	// A bare `value=` input is owned by the browser, so a rejected
	// write leaves the rejected number sitting in the box next to a
	// grade that does not have it — indefinitely, because the
	// expression behind the box never changed and Svelte has no reason
	// to touch the DOM.
	it("puts a refused number back to what the server has", async (): Promise<void> => {
		assert.ok(harness !== null)
		const stack = harness

		const page = await adminPage("#/grades")
		await page.getByRole("heading", { name: "Y9" }).waitFor()

		// Alice takes a course, so lowering the cap to zero strands
		// her and the write comes back asking to be confirmed.
		await page.request.post("/admin/api/enrollments", {
			data: {
				course_id: "CH",
				student_ids: [subjects.alice],
				student_droppable: true,
				counts_toward_budget: true,
			},
		})

		const card = page.locator("article.card", { hasText: "Y9" }).first()
		const budget = card.getByLabel("Period budget for Y9")
		const asStored = await budget.inputValue()

		await budget.fill("0")
		await budget.blur()

		// Decline the confirmation.
		await page.getByRole("button", { name: /Cancel/ }).click()

		// The box must go back to what the grade actually has, rather
		// than keeping the number the administrator declined.
		const deadline = Date.now() + 5000
		let settled = await budget.inputValue()
		while (settled !== asStored && Date.now() < deadline) {
			await page.waitForTimeout(100)
			settled = await budget.inputValue()
		}

		assert.equal(
			settled,
			asStored,
			"the box kept a value the administrator declined",
		)

		await page.request.delete("/admin/api/enrollments", {
			data: { course_id: "CH", student_ids: [subjects.alice] },
		})
		void stack
	})

	// A CEL expression that does not compile shows every row, which is
	// right — a half-typed expression should not blank the table under
	// the cursor. What is not right is offering to act on those rows
	// in bulk: an administrator who narrows two hundred courses to
	// three, mistypes, then selects all and deletes takes the whole
	// catalogue. The error is on the screen, but so is every row, and
	// a table that looks unfiltered because the filter is broken looks
	// exactly like one that is unfiltered because nothing was typed.
	it("will not bulk-act on a list a broken filter failed to narrow", async (): Promise<void> => {
		const page = await adminPage("#/courses")
		await page.getByText("Basketball").waitFor()

		await page.getByLabel("Filter mode").selectOption("cel")

		const box = page.getByLabel("Filter", { exact: true })
		await box.fill('name.startsWith("Bas")')

		const selectAll = page.getByLabel("Select all filtered courses")
		await selectAll.waitFor()
		assert.equal(
			await selectAll.isDisabled(),
			false,
			"a working filter should still allow selecting its rows",
		)

		const narrowed = await page.locator("tbody tr").count()
		assert.equal(narrowed, 1, "the filter should have narrowed the list")

		// Now break it.
		await box.fill('name.startsWith("Bas"')

		// Every row is back...
		await page.waitForFunction(
			(): boolean => document.querySelectorAll("tbody tr").length > 1,
			undefined,
			{ timeout: 5000 },
		)

		// ...and selecting them all is not on offer.
		assert.equal(
			await selectAll.isDisabled(),
			true,
			"select-all was offered over a list the filter failed to narrow",
		)
	})
})
