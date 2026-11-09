// Brings up one disposable stack per test file: a scratch database, a
// real cca serving it, and a browser.
//
// One thing is faked, because the test cannot host it and it is not
// what is under test: the identity provider. cca runs with -dev-login,
// and each context signs in through that form, so the session cookie
// is set by the server's own encoder after the same allowlist and
// roster checks a real sign-in gets. Every request the browser makes
// is answered by the production handlers.
//
// Chromium is taken from PATH or the standard macOS application path
// and never downloaded, which is why the dependency is playwright-core
// rather than playwright.

import { execFileSync, spawn, type ChildProcess } from "node:child_process"
import { accessSync, constants } from "node:fs"
import { createServer, type Server } from "node:http"
import { createServer as createSocketServer } from "node:net"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"
import {
	chromium,
	type Browser,
	type BrowserContext,
	type Page,
} from "playwright-core"

const e2eDir = dirname(fileURLToPath(import.meta.url))

// The compiled tests run from ui/.e2e, beside the source directory.
const scriptDir = join(e2eDir, "..", "e2e")

export interface Harness {
	baseURL: string
	// A different origin (another port on this host), for testing that
	// cross-origin writes are refused.
	otherOrigin: string
	browser: Browser
	// What the page actually was, for when a locator times out and the
	// bare timeout says nothing about why nothing matched.
	describe: (page: Page) => Promise<string>
	// Runs SQL against the scratch database.
	//
	// For arranging the state a test is about, where doing it through
	// the interface would be a second test in front of the first: a
	// budget cap, a fixed placement, a closed window. What is under
	// test is always still driven through the browser.
	exec: (sql: string) => void
	// A context already signed in as this subject.
	contextFor: (
		role: "student" | "admin",
		subject: string,
	) => Promise<BrowserContext>
	// Closes every context opened since the previous call. Test files
	// call this after each test so pages, renderers and sockets do not
	// accumulate for the lifetime of the suite.
	closeContexts: () => Promise<void>
	close: () => Promise<void>
}

// The seeded subjects. Students are email localparts, as everywhere;
// the administrator is one too, and is on the allowlist the backend
// script writes into the config.
export const subjects = {
	alice: "s1001",
	bob: "s1002",
	admin: "e2e.admin",
} as const

// Chromium's usual names, in PATH order. Overridable for unusual
// installations, but never downloaded, and never skipped over: a
// missing browser is a broken environment, and a suite that quietly
// does not run is worse than one that fails.
function chromiumPath(): string {
	const override = process.env["CCA_E2E_CHROMIUM"]
	if (override !== undefined && override !== "") {
		try {
			accessSync(override, constants.X_OK)
		} catch {
			throw new Error(
				`CCA_E2E_CHROMIUM is set to ${override}, which is not executable`,
			)
		}
		return override
	}
	const names = ["chromium", "chromium-browser", "google-chrome"]
	for (const name of names) {
		const found = which(name)
		if (found !== null) {
			return found
		}
	}
	if (process.platform === "darwin") {
		const chrome =
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		try {
			accessSync(chrome, constants.X_OK)
			return chrome
		} catch {
			// Continue to the useful environment error below.
		}
	}

	throw new Error(
		`no browser in PATH: looked for ${names.join(", ")}. ` +
			"Install chromium, or point CCA_E2E_CHROMIUM at one; " +
			"these tests never download a browser.",
	)
}

function which(name: string): string | null {
	for (const dir of (process.env["PATH"] ?? "").split(":")) {
		if (dir === "") {
			continue
		}
		const candidate = join(dir, name)
		try {
			accessSync(candidate, constants.X_OK)
			return candidate
		} catch {
			continue
		}
	}
	return null
}

// A free TCP port, released immediately. The window between releasing
// and cca binding is small enough for a loopback test and avoids
// having to teach the server to report the port it chose.
function freePort(): Promise<number> {
	return new Promise<number>((resolve, reject): void => {
		const server = createSocketServer()
		server.on("error", reject)
		server.listen(0, "127.0.0.1", (): void => {
			const address = server.address()
			if (address === null || typeof address === "string") {
				server.close()
				reject(new Error("no port assigned"))
				return
			}
			const { port } = address
			server.close((): void => {
				resolve(port)
			})
		})
	})
}

// A second origin, on its own port: somewhere to host a page from a
// *different* origin than cca, which is what the cross-origin write
// test needs.
//
// Binds port 0 and reads back what it got, rather than going through
// freePort: this one holds its port for the whole run, so the port
// picked for cca afterwards cannot collide with it.
async function startOtherOrigin(): Promise<{
	origin: string
	server: Server
}> {
	const server = createServer((req, res): void => {
		// A blank page, so a test can run script in
		// this origin rather than cca's.
		res.writeHead(200, { "content-type": "text/html" })
		res.end("<!doctype html><title>other origin</title>")
	})
	const port = await new Promise<number>((resolve, reject): void => {
		server.on("error", reject)
		server.listen(0, "127.0.0.1", (): void => {
			const address = server.address()
			if (address === null || typeof address === "string") {
				reject(new Error("no port assigned to the other origin"))
				return
			}
			resolve(address.port)
		})
	})
	const origin = `http://127.0.0.1:${String(port)}`

	return { origin, server }
}

async function waitForServer(
	baseURL: string,
	backend: ChildProcess,
): Promise<void> {
	const deadline = Date.now() + 20_000
	for (;;) {
		if (backend.exitCode !== null) {
			throw new Error(
				`backend exited with code ${String(backend.exitCode)}`,
			)
		}
		try {
			const response = await fetch(`${baseURL}/`, {
				signal: AbortSignal.timeout(1000),
			})
			if (response.ok) {
				// A port cca failed to bind may be answering for
				// something else entirely — the other origin, or another
				// run's server. Anything but cca's landing page here
				// would surface much later as an inexplicable timeout
				// in whichever test looked at the page first.
				const body = await response.text()
				if (!body.includes("<title>CCA</title>")) {
					const head = JSON.stringify(body.slice(0, 200))
					throw new Error(
						`${baseURL} is answering, but not with cca's landing page: ${head}`,
					)
				}
				return
			}
		} catch (err) {
			if (err instanceof Error && err.message.includes("not with cca")) {
				throw err
			}
			// not up yet
		}
		if (Date.now() > deadline) {
			throw new Error(`cca did not start listening on ${baseURL}`)
		}
		await new Promise((resolve): unknown => setTimeout(resolve, 100))
	}
}

// Throws if the stack cannot be built: a missing browser or database
// is a broken environment, not a reason to pass quietly.
export async function startHarness(): Promise<Harness> {
	const executablePath = chromiumPath()

	const port = await freePort()
	const baseURL = `http://127.0.0.1:${String(port)}`
	const dbName = `cca_e2e_${String(process.pid)}`
	const other = await startOtherOrigin()

	// Keep the backend in this process group, so an interrupt from the
	// terminal reaches it too. Its trap stops cca and drops the database.
	const backend = spawn(join(scriptDir, "backend"), [], {
		env: {
			...process.env,
			CCA_E2E_DB: dbName,
			CCA_E2E_ADDR: `127.0.0.1:${String(port)}`,
		},
		stdio: ["ignore", "pipe", "pipe"],
	})

	// Bounded, so a chatty server cannot grow this without limit.
	let log = ""
	const appendLog = (text: string): void => {
		log = (log + text).slice(-20_000)
	}
	const record = (chunk: Buffer): void => {
		appendLog(chunk.toString())
	}
	backend.stdout.on("data", record)
	backend.stderr.on("data", record)

	let shutdownPromise: Promise<void> | null = null
	const shutdown = (): Promise<void> => {
		if (shutdownPromise !== null) {
			return shutdownPromise
		}

		shutdownPromise = (async (): Promise<void> => {
			const exited =
				backend.exitCode !== null || backend.signalCode !== null
					? Promise.resolve()
					: new Promise<void>((resolve): void => {
							const done = (): void => {
								backend.off("exit", done)
								clearTimeout(timer)
								resolve()
							}
							backend.once("exit", done)
							const timer = setTimeout(done, 5000)
							// Cover an exit between the check above and installing
							// the listener.
							if (
								backend.exitCode !== null ||
								backend.signalCode !== null
							) {
								done()
							}
						})

			if (backend.exitCode === null && backend.signalCode === null) {
				try {
					backend.kill("SIGTERM")
				} catch {
					// already gone
				}
			}
			await exited
			await new Promise<void>((resolve): void => {
				other.server.close((): void => {
					resolve()
				})
			})
		})()

		return shutdownPromise
	}

	let browser: Browser
	try {
		await waitForServer(baseURL, backend)
		browser = await chromium.launch({ executablePath })
	} catch (err) {
		await shutdown()
		throw new Error(`${String(err)}\nbackend log:\n${log}`)
	}

	let closing = false
	browser.on("disconnected", (): void => {
		if (!closing) {
			appendLog("\nChromium disconnected unexpectedly.\n")
		}
	})

	const contexts = new Set<BrowserContext>()
	const closeContexts = async (): Promise<void> => {
		const open = [...contexts]
		const results = await Promise.allSettled(
			open.map(async (context): Promise<void> => {
				await context.close()
			}),
		)
		const failures = results.filter(
			(result): result is PromiseRejectedResult =>
				result.status === "rejected",
		)
		if (failures.length > 0 && browser.isConnected()) {
			throw new AggregateError(
				failures.map((failure): string => String(failure.reason)),
				"failed to close E2E browser contexts",
			)
		}
	}

	const exec = (sql: string): void => {
		execFileSync(
			"psql",
			["-v", "ON_ERROR_STOP=1", "-q", "-d", dbName, "-c", sql],
			{
				stdio: ["ignore", "ignore", "pipe"],
			},
		)
	}

	return {
		baseURL,
		exec,
		otherOrigin: other.origin,
		browser,
		describe: async (page: Page): Promise<string> => {
			let body = "<unreadable>"
			try {
				body = (await page.locator("body").innerText()).slice(0, 400)
			} catch {
				// the page may be gone
			}
			return [
				`url: ${page.url()}`,
				`browser connected: ${String(browser.isConnected())}`,
				`body: ${JSON.stringify(body)}`,
				`server log (tail):\n${log.slice(-1500)}`,
			].join("\n")
		},
		contextFor: async (role, subject): Promise<BrowserContext> => {
			if (!browser.isConnected()) {
				throw new Error(
					`Chromium disconnected unexpectedly.\nserver log:\n${log.slice(-1500)}`,
				)
			}
			const context = await browser.newContext({ baseURL })
			contexts.add(context)
			context.on("close", (): void => {
				contexts.delete(context)
			})
			try {
				const response = await fetch(`${baseURL}/dev/login`, {
					method: "POST",
					body: new URLSearchParams({ role, email: subject }),
					redirect: "manual",
				})
				const name = `${role}_session`
				const value = response.headers
					.getSetCookie()
					.map((c) => c.split(";")[0] ?? "")
					.find((c) => c.startsWith(`${name}=`))
					?.slice(name.length + 1)
				if (value === undefined || value === "") {
					throw new Error(
						`dev login as ${role}:${subject} set no session (status ${String(response.status)})`,
					)
				}
				// Copied in rather than left to a cookie jar: the
				// server marks it Secure, and this origin is plain
				// http.
				await context.addCookies([{ name, value, url: baseURL }])
			} catch (err) {
				await context.close()
				throw err
			}
			return context
		},
		closeContexts,
		close: async (): Promise<void> => {
			closing = true
			try {
				try {
					await closeContexts()
				} finally {
					await browser.close()
				}
			} finally {
				await shutdown()
			}
		},
	}
}
