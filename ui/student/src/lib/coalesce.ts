// Collapsing refetches that arrive together.
//
// A student's own write causes two of everything. The write path
// refetches what it changed — deliberately, because a dropped socket
// must not leave the page showing what the user just changed away
// from — and then the server's invalidation frame arrives at the same
// session and asks for the same thing again. Both are correct in
// isolation; together they double the load on the eligibility read,
// which is the most expensive one in the system and is busiest at
// exactly the moment everyone is acting at once.
//
// A short delay makes the pair one fetch. It costs nothing visible:
// the enrollment list itself comes back in the write's own response,
// so what is delayed is only the derived view of it.
//
// The model is already eventually consistent — the whole invalidation
// scheme assumes a client may be briefly behind — so waiting a moment
// longer changes nothing about what is guaranteed.
//
// Two more things are bounded here, both learned from a live window
// that took the database down.
//
// At most one request is in flight. A trigger that arrives while one
// is running does not start a second: it marks the answer stale, and
// exactly one more request follows when the first returns — the first
// may have been answered before whatever the trigger announced, so it
// cannot simply be reused. Without this, every trigger during a slow
// response added another request, so the slower the server got the
// more each page asked of it, until the 30-second timeout.
//
// A trigger may ask to be spread. A frame the hub broadcasts reaches
// every open page in the same instant, and a fixed delay moves the
// whole herd without dispersing it; a random delay over a couple of
// seconds turns a spike of every page at once into a plateau. The
// student's own writes do not spread — they are the one refetch
// somebody is watching for — and the earliest deadline asked for
// always wins, so a spread trigger never postpones a prompt one.

const settleDelay = 150

export interface Coalescer {
	// Asks for a refresh, after the settle delay plus a random share of
	// `spread` milliseconds. Several calls close together produce one.
	trigger: (spread?: number) => void
	// Stops any pending refresh; for component teardown.
	cancel: () => void
}

export function coalesce(load: () => Promise<void>): Coalescer {
	let timer: number | null = null
	let due = 0
	let running = false
	// The delay for the one request that follows the running one, or
	// null when nothing arrived while it ran.
	let followUp: number | null = null

	function schedule(delay: number): void {
		const at = Date.now() + delay
		if (timer !== null) {
			if (due <= at) {
				return
			}
			window.clearTimeout(timer)
		}
		due = at
		timer = window.setTimeout(fire, delay)
	}

	function fire(): void {
		timer = null
		running = true
		load()
			.catch((): void => {
				// The loader reports its own failures; a rejection
				// here must not strand the follow-up below.
			})
			.finally((): void => {
				running = false
				if (followUp !== null) {
					const delay = followUp
					followUp = null
					schedule(delay)
				}
			})
	}

	return {
		trigger: (spread = 0): void => {
			const delay = settleDelay + Math.random() * spread
			if (running) {
				followUp = followUp === null ? delay : Math.min(followUp, delay)
				return
			}
			schedule(delay)
		},
		cancel: (): void => {
			if (timer !== null) {
				window.clearTimeout(timer)
				timer = null
			}
			followUp = null
		},
	}
}
