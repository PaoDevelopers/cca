// The websocket path, which nothing else covers end to end.
//
// internal/web/websocket_test.go checks the hub's bookkeeping in
// isolation and the browser tests above never involve a second party.
// This one enrolls as Bob and asserts that Alice's already-open page
// updates on its own — so it exercises the enrolment handler marking
// the course dirty, the refresher reading the new count, the hub's
// versioned fan-out, the frame format, and the client that parses it.

import assert from "node:assert/strict"
import { after, afterEach, before, describe, it } from "node:test"
import type { Page } from "playwright-core"
import { startHarness, subjects, type Harness } from "./harness.js"

describe("realtime", (): void => {
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

	it("updates an open page when another student takes a seat", async (): Promise<void> => {
		// before() throws if the stack could not be built, so by here it
		// is always present; this is for the type, not for control flow.
		assert.ok(harness !== null)

		const aliceContext = await harness.contextFor("student", subjects.alice)
		const alice = await aliceContext.newPage()
		await alice.goto("/student/")

		// The nav only renders once the page is signed in and loaded, so
		// a timeout here means the page was not the app at all. Say what
		// it was instead of leaving a bare locator timeout.
		try {
			await alice.getByRole("button", { name: /All courses/ }).click()
		} catch (err) {
			throw new Error(
				`${String(err)}\n--- alice's page ---\n${await harness.describe(alice)}`,
			)
		}

		const basketball = alice.locator("article.card", {
			hasText: "Basketball",
		})
		await basketball.getByText("0/2").waitFor()

		// Bob enrols in his own session. Alice never reloads.
		const bobContext = await harness.contextFor("student", subjects.bob)
		const bob = await bobContext.newPage()
		await bob.goto("/student/")
		await bob.getByRole("button", { name: /All courses/ }).click()
		await bob
			.locator("article.card", { hasText: "Basketball" })
			.getByRole("button", { name: /Enroll\b.*\bin Basketball/ })
			.click()
		await bob
			.getByRole("button", { name: /Your selections \(1\)/ })
			.waitFor()

		// Arrives over the socket as course_count_update, not by polling.
		await basketball.getByText("1/2").waitFor({ timeout: 10_000 })

		const seats = await basketball.innerText()
		assert.match(seats, /1\/2/)

		await bob.getByRole("button", { name: /Your selections/ }).click()
		await bob.getByRole("button", { name: /Drop\b.*\bBasketball/ }).click()
		await bob
			.getByRole("button", { name: /Confirm dropping Basketball/ })
			.click()

		// And back down again, so the fan-out is not a one-way effect.
		await basketball.getByText("0/2").waitFor({ timeout: 10_000 })

		await aliceContext.close()
		await bobContext.close()
	})

	// A student's own write reaches their other tabs as the new state
	// itself (a student_state frame), so no page of theirs reads it
	// back: not the tab that wrote, and not the others. Checked through
	// what the page shows — the enrollment and a clash that only the
	// eligibility map can report — and through the requests it made.
	it("brings the student's other tab up to date without it reading anything", async (): Promise<void> => {
		assert.ok(harness !== null)
		const stack = harness

		const context = await stack.contextFor("student", subjects.alice)

		// Each tab's socket is up before anything happens, or a frame
		// sent in between would be missed rather than tested.
		async function open(): Promise<Page> {
			const page = await context.newPage()
			const socket = page.waitForEvent("websocket")
			await page.goto("/student/")
			await (await socket).waitForEvent("framereceived")
			await page.getByRole("button", { name: /All courses/ }).click()
			return page
		}

		const first = await open()
		const second = await open()

		const reads: string[] = []
		for (const page of [first, second]) {
			page.on("request", (request): void => {
				const path = new URL(request.url()).pathname
				if (
					path === "/student/api/eligibility" ||
					path === "/student/api/user_info" ||
					path === "/student/api/my_enrollments"
				) {
					reads.push(`${request.method()} ${path}`)
				}
			})
		}

		try {
			// Baking meets in MON1, like Basketball.
			const baking = second.locator("article.card", { hasText: "Baking" })
			await baking.waitFor()
			assert.doesNotMatch(await baking.innerText(), /Clashes with/)

			await first
				.locator("article.card", { hasText: "Basketball" })
				.getByRole("button", { name: /Enroll\b.*\bin Basketball/ })
				.click()

			await second
				.getByRole("button", { name: /Your selections \(1\)/ })
				.waitFor({ timeout: 10_000 })
			await baking
				.getByText(/Clashes with Basketball/)
				.waitFor({ timeout: 10_000 })

			// Past the writing tab's fallback (stateGrace), which must
			// not have fired: the frame came.
			await first.waitForTimeout(2500)

			assert.deepEqual(reads, ["PUT /student/api/my_enrollments"])
		} finally {
			stack.exec("DELETE FROM enrollments WHERE student_id = 's1001'")
			await context.close()
		}
	})

	// Everything sent while the socket was down is gone: the hub keeps
	// no per-client queue, and the snapshot a reconnecting client gets
	// covers the counts and nothing else. So an invalidation that
	// happened in the gap is never delivered, and the page keeps
	// showing what it showed before the gap — indefinitely.
	//
	// The window opening is the case that matters. A student whose
	// laptop slept through it comes back to a page that still says
	// enrollment is closed, and nothing on it will ever say otherwise.
	it("reloads what it missed after the socket comes back", async (): Promise<void> => {
		assert.ok(harness !== null)
		const stack = harness

		// Start closed, so there is something to miss.
		stack.exec(
			`UPDATE grades SET opens_at = NULL, closes_at = NULL
			WHERE id = 'Y9'`,
		)

		const context = await stack.contextFor("student", subjects.alice)
		const page = await context.newPage()
		await page.goto("/student/")
		await page.getByRole("button", { name: /All courses/ }).click()
		await page
			.getByText(/Enrollment closed/)
			.first()
			.waitFor()

		// The window opens while nobody is listening for it.
		stack.exec(
			`UPDATE grades SET opens_at = now() - interval '1 hour',
				closes_at = NULL WHERE id = 'Y9'`,
		)

		// Take the network away and give it back, which is what a real
		// gap is: a laptop lid, a wifi handover, a NAT timeout. The
		// socket closes, the page notices, and reconnecting is the
		// only thing that can repair it.
		//
		// This used to be done by opening sockets until the per-identity
		// cap evicted this page's. That no longer works, and should
		// not: an evicted socket is now closed with a code the client
		// deliberately does not retry, because retrying it evicted the
		// next oldest and the churn never ended.
		await context.setOffline(true)
		await page.waitForFunction(
			(): boolean => !navigator.onLine,
			undefined,
			{ timeout: 5000 },
		)
		await context.setOffline(false)

		// Nothing invalidated the page's grades — that frame was sent
		// while it was offline, if at all. Only the reconnect resync
		// can bring it back.
		await page
			.getByRole("button", { name: /Enroll\b.*\bin Chess/ })
			.waitFor({ timeout: 20_000 })
	})
})
