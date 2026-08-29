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
// A short trailing delay makes the pair one fetch. It costs nothing
// visible: the enrollment list itself comes back in the write's own
// response, so what is delayed is only the derived view of it.
//
// The model is already eventually consistent — the whole invalidation
// scheme assumes a client may be briefly behind — so waiting a moment
// longer changes nothing about what is guaranteed.

const settleDelay = 150

interface Coalescer {
	// Asks for a refresh. Several calls close together produce one.
	trigger: () => void
	// Stops any pending refresh; for component teardown.
	cancel: () => void
}

export function coalesce(load: () => void): Coalescer {
	let timer: number | null = null

	return {
		trigger: (): void => {
			if (timer !== null) {
				window.clearTimeout(timer)
			}
			timer = window.setTimeout((): void => {
				timer = null
				load()
			}, settleDelay)
		},
		cancel: (): void => {
			if (timer !== null) {
				window.clearTimeout(timer)
				timer = null
			}
		},
	}
}
