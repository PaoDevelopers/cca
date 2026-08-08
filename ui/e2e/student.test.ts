// The student's own path through the app, against the real server.
// What these reach that svelte-check and the Go tests cannot is the
// join between them: whether the JSON the handlers emit is the shape
// the components read, and whether a rule the database enforces is
// also presented before the user runs into it.

import assert from "node:assert/strict"
import { after, afterEach, before, describe, it } from "node:test"
import type { Page } from "playwright-core"
import { startHarness, subjects, type Harness } from "./harness.js"

describe("student", (): void => {
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

	// Every test gets its own page; the seeded data is shared, so the
	// enrolments each one makes are undone at the end.
	async function studentPage(): Promise<Page> {
		assert.ok(harness !== null)
		const context = await harness.contextFor("student", subjects.alice)
		const page = await context.newPage()
		await page.goto("/student/")
		return page
	}

	async function openAvailable(page: Page): Promise<void> {
		await page.getByRole("button", { name: /Available courses/ }).click()
		await page.locator("article.card").first().waitFor()
	}

	it("shows the signed-in student their own details", async (): Promise<void> => {
		const page = await studentPage()

		// Straight from the seeded row, through /student/api/user_info.
		await page.getByText("Alice Example").waitFor()
		const account = await page.locator("section").first().innerText()
		assert.match(account, /Year 9/)
		assert.match(account, /s1001/)
	})

	it("lists the catalogue with the fields the API sends", async (): Promise<void> => {
		const page = await studentPage()
		await openAvailable(page)

		const cards = await page.locator("article.card").allInnerTexts()
		assert.equal(cards.length, 3)

		const baking = cards.find((c): boolean => c.includes("Baking"))
		assert.ok(baking !== undefined, "Baking should be listed")
		// cost and term are free text on the course row; a serialisation
		// that dropped them would still render a plausible-looking card.
		assert.match(baking, /200 rmb/)
		assert.match(baking, /Season/)
	})

	it("starts with independent availability filters", async (): Promise<void> => {
		assert.ok(harness !== null)
		const stack = harness

		// One course for each default: Basketball is invite-only,
		// Baking is incompatible with Alice's legal sex, and Chess is
		// full. Full courses remain visible while the other two do not.
		stack.exec("UPDATE courses SET invite_only = TRUE WHERE id = 'BB'")
		stack.exec("UPDATE courses SET max_students = 0 WHERE id = 'CH'")
		stack.exec(
			"INSERT INTO course_allowed_legal_sexes (course_id, legal_sex) VALUES ('BK', 'M')",
		)

		try {
			const page = await studentPage()
			await openAvailable(page)

			const hideFull = page.getByRole("checkbox", { name: "Hide full" })
			const hideInviteOnly = page.getByRole("checkbox", {
				name: "Hide invite-only",
			})
			const hideIncompatible = page.getByRole("checkbox", {
				name: "Hide incompatible",
			})
			const hideConflicting = page.getByRole("checkbox", {
				name: "Hide conflicting",
			})

			assert.equal(await hideFull.isChecked(), false)
			assert.equal(await hideInviteOnly.isChecked(), true)
			assert.equal(await hideIncompatible.isChecked(), true)
			assert.equal(await hideConflicting.isChecked(), false)

			const cards = page.locator("article.card")
			assert.equal(await cards.count(), 1)
			await cards.getByText("Chess").waitFor()

			await hideFull.check()
			await page.getByText("No courses match your search.").waitFor()

			await hideFull.uncheck()
			await hideInviteOnly.uncheck()
			await page.getByRole("heading", { name: "Basketball" }).waitFor()
			assert.equal(await cards.count(), 2)

			await hideIncompatible.uncheck()
			await page.getByRole("heading", { name: "Baking" }).waitFor()
			assert.equal(await cards.count(), 3)
		} finally {
			stack.exec("UPDATE courses SET invite_only = FALSE WHERE id = 'BB'")
			stack.exec("UPDATE courses SET max_students = 10 WHERE id = 'CH'")
			stack.exec(
				"DELETE FROM course_allowed_legal_sexes WHERE course_id = 'BK'",
			)
		}
	})

	it("enrolls, and the seat count reflects it", async (): Promise<void> => {
		const page = await studentPage()
		await openAvailable(page)

		const chess = page.locator("article.card", { hasText: "Chess" })
		await chess
			.getByRole("button", { name: /Enroll\b.*\bin Chess/ })
			.click()

		// Moves out of the catalogue and into the student's own list.
		await page.getByRole("button", { name: /Your courses \(1\)/ }).waitFor()
		await page.getByRole("button", { name: /Your courses/ }).click()
		await page.getByText("Chess").first().waitFor()

		await page.getByRole("button", { name: /Available courses/ }).click()
		await openAvailable(page)
		const remaining = await page.locator("article.card").allInnerTexts()
		assert.equal(remaining.length, 2)

		// Put the fixture back for the other tests.
		await page.getByRole("button", { name: /Your courses/ }).click()
		await page.getByRole("button", { name: /Drop\b.*\bChess/ }).click()
		await page
			.getByRole("button", { name: /Confirm dropping Chess/ })
			.click()
		await page.getByRole("button", { name: /Your courses \(0\)/ }).waitFor()
	})

	// A student working by keyboard, which is the case that had nothing
	// at all: enrolling disabled the button and then removed the card
	// from Available entirely, so the focus fell to <body> and the next
	// Tab landed in the footer, past the whole catalogue. Nothing was
	// announced either — the only live region on the page was the error
	// popup, so a screen reader said nothing about a write that had
	// just succeeded.
	it("keeps the keyboard somewhere useful, and says what happened", async (): Promise<void> => {
		const page = await studentPage()
		await openAvailable(page)

		const acted = "[data-course-action]"
		const listed = await page.$$eval(acted, (nodes): string[] =>
			nodes.map(
				(n): string => (n as HTMLElement).dataset["courseAction"] ?? "",
			),
		)
		assert.ok(listed.length >= 2, "need a list to be moved around in")

		const first = page.locator(acted).first()
		await first.focus()
		await first.click()

		// The card this acted on is gone from Available. The focus goes
		// to whatever took its position, not to nowhere and not back to
		// the top of the document.
		await page.waitForFunction(
			(): boolean =>
				document.activeElement instanceof HTMLElement &&
				document.activeElement.dataset["courseAction"] !== undefined,
			undefined,
			{ timeout: 5000 },
		)
		assert.equal(
			await page.evaluate(
				(): string =>
					(document.activeElement as HTMLElement).dataset[
						"courseAction"
					] ?? "",
			),
			listed[1],
			"the focus should be on the course that took the acted-on one's place",
		)

		// And a polite region says what happened, because nothing else
		// on the page does.
		assert.match(
			await page.evaluate(
				(): string =>
					document.querySelector('[aria-live="polite"]')
						?.textContent ?? "",
			),
			/Enrolled in /,
			"a successful enrolment was not announced",
		)

		// Arming a confirmation replaces the button that was pressed;
		// the focus follows to the button that must be pressed next
		// rather than being dropped between them.
		await page.getByRole("button", { name: /Your courses/ }).click()
		const drop = page.locator(acted).first()
		await drop.focus()
		await drop.click()
		assert.match(
			await page.evaluate(
				(): string =>
					document.activeElement?.getAttribute("aria-label") ?? "",
			),
			/^Confirm dropping /,
			"arming the confirmation dropped the focus",
		)

		// Dropping the last one leaves no course to move to. The
		// heading names the list and is where a reader starts again.
		await page.locator(acted).first().click()
		await page.waitForFunction(
			(): boolean => document.activeElement?.tagName === "H2",
			undefined,
			{ timeout: 5000 },
		)

		await page.getByRole("button", { name: /Your courses \(0\)/ }).waitFor()
	})

	// Basketball and Baking both meet in MON1. The database would
	// refuse the second one; the point of this test is that the student
	// is told before they try, and offered the swap instead — and that
	// the reason shown is the server's own, from
	// /student/api/eligibility, rather than a clash the browser worked
	// out for itself.
	it("marks a clashing course rather than letting it be chosen", async (): Promise<void> => {
		const page = await studentPage()
		await openAvailable(page)
		const hideConflicting = page.getByRole("checkbox", {
			name: "Hide conflicting",
		})
		assert.equal(await hideConflicting.isChecked(), false)

		const basketball = page.locator("article.card", {
			hasText: "Basketball",
		})
		await basketball
			.getByRole("button", { name: /Enroll\b.*\bin Basketball/ })
			.click()
		await page.getByRole("button", { name: /Your courses \(1\)/ }).waitFor()

		const baking = page.locator("article.card", { hasText: "Baking" })
		// The wording is the database's: enrollment_violations builds
		// it, and it travels through the eligibility read untouched.
		await baking.getByText(/clashes with BB in MON1/).waitFor()

		// Conflicts are visible by default and can be hidden without
		// changing the demographic-compatibility filter.
		await hideConflicting.check()
		await baking.waitFor({ state: "hidden" })
		await hideConflicting.uncheck()
		await baking.getByText(/clashes with BB in MON1/).waitFor()

		// Enrolling is not offered; swapping is.
		assert.equal(
			await baking
				.getByRole("button", { name: /^Enroll\b.*\bin Baking/ })
				.count(),
			0,
			"a clashing course must not offer a plain enroll",
		)
		await baking
			.getByRole("button", {
				name: /Swap in\b.*\bBaking clashes with/,
			})
			.waitFor()

		await page.getByRole("button", { name: /Your courses/ }).click()
		await page.getByRole("button", { name: /Drop\b.*\bBasketball/ }).click()
		await page
			.getByRole("button", { name: /Confirm dropping Basketball/ })
			.click()
		await page.getByRole("button", { name: /Your courses \(0\)/ }).waitFor()
	})

	// A student at their budget cap was never offered Swap, because
	// the offer required every violation to be a clash and the cap
	// added a budget one alongside it. But the swap gives back exactly
	// the periods that put them over: the server accepts it.
	//
	// Withholding it is the worse failure of the two — the student is
	// told a course is out of reach when it is not, and nothing on the
	// screen suggests otherwise.
	it("offers a swap to a student who is at their budget cap", async (): Promise<void> => {
		assert.ok(harness !== null)
		const stack = harness

		// One budgeted period, which Basketball fills. Restored in the
		// finally below: the fixture is shared, and a failure here
		// must not become a failure in the next test.
		stack.exec("UPDATE grades SET max_budgeted_periods = 1 WHERE id = 'Y9'")

		try {
			const page = await studentPage()
			await openAvailable(page)

			const basketball = page.locator("article.card", {
				hasText: "Basketball",
			})
			await basketball
				.getByRole("button", { name: /Enroll\b.*\bin Basketball/ })
				.click()
			await page
				.getByRole("button", { name: /Your courses \(1\)/ })
				.waitFor()

			// Baking clashes with Basketball *and* would put Alice over
			// the cap. Both go away if Basketball does.
			const baking = page.locator("article.card", { hasText: "Baking" })
			await baking
				.getByRole("button", {
					name: /Swap in\b.*\bBaking clashes with/,
				})
				.click()
			await baking
				.getByRole("button", { name: /Confirm swapping into Baking/ })
				.click()

			// And it went through: Basketball is gone, Baking is held.
			await page
				.getByRole("button", { name: /Your courses \(1\)/ })
				.waitFor()
			await page.getByRole("button", { name: /Your courses/ }).click()
			await page
				.getByRole("button", { name: /Drop\b.*\bBaking/ })
				.waitFor()

			await page.getByRole("button", { name: /Drop\b.*\bBaking/ }).click()
			await page
				.getByRole("button", { name: /Confirm dropping Baking/ })
				.click()
			await page
				.getByRole("button", { name: /Your courses \(0\)/ })
				.waitFor()
		} finally {
			stack.exec(
				"UPDATE grades SET max_budgeted_periods = 4 WHERE id = 'Y9'",
			)
			stack.exec("DELETE FROM enrollments WHERE student_id = 's1001'")
		}
	})

	// The other direction. A swap has to drop what it clashes with,
	// and a placement an administrator fixed is not the student's to
	// drop — the server answers YKG03 and nothing is swapped. Offering
	// the button spends a click to be told no.
	it("does not offer a swap that would have to drop a fixed placement", async (): Promise<void> => {
		assert.ok(harness !== null)
		const stack = harness

		stack.exec(
			`INSERT INTO enrollments
				(student_id, course_id, student_droppable, counts_toward_budget)
			VALUES ('s1001', 'BB', FALSE, TRUE)`,
		)

		try {
			const page = await studentPage()
			await openAvailable(page)

			const baking = page.locator("article.card", { hasText: "Baking" })
			await baking.getByText(/clashes with BB in MON1/).waitFor()

			assert.equal(
				await baking
					.getByRole("button", {
						name: /Swap in\b.*\bBaking clashes with/,
					})
					.count(),
				0,
				"a swap was offered that would have to drop a fixed placement",
			)
		} finally {
			stack.exec("DELETE FROM enrollments WHERE student_id = 's1001'")
		}
	})

	// A student has no accept dialog: a refused write is simply
	// refused, so the violation payload is the only place the reasons
	// ever reach them. They used to be shown the server's summary —
	// "1 unaccepted violation(s)" — which says nothing they can act on.
	it("says what was wrong when a write is refused, not how many things were", async (): Promise<void> => {
		assert.ok(harness !== null)
		const stack = harness

		// Bob takes the second and last seat, from outside this page,
		// so Alice's browser holds a stale count and her enroll is
		// refused by the server rather than hidden by the client.
		stack.exec(
			`INSERT INTO enrollments
				(student_id, course_id, student_droppable, counts_toward_budget)
			VALUES ('s1002', 'BB', TRUE, TRUE)`,
		)

		try {
			const page = await studentPage()
			await openAvailable(page)

			stack.exec("UPDATE courses SET max_students = 1 WHERE id = 'BB'")

			const basketball = page.locator("article.card", {
				hasText: "Basketball",
			})
			await basketball
				.getByRole("button", { name: /Enroll\b.*\bin Basketball/ })
				.click()

			// The database's own words, from enrollment_violations.
			await page.getByText(/BB is full/).waitFor()
		} finally {
			stack.exec("UPDATE courses SET max_students = 2 WHERE id = 'BB'")
			stack.exec("DELETE FROM enrollments WHERE student_id = 's1002'")
		}
	})
})
