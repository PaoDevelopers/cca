// Client for the /…/api/events WebSocket endpoints. The server pushes
// plain text frames: "hello" on connect, "invalidate_<resource>", and
// "course_count_update,<course id>,<count>".

export interface EventHandlers {
	oninvalidate?: (resource: string) => void
	oncoursecount?: (courseID: string, count: number) => void
	// Called when a socket opens after an earlier one had been
	// established — a reconnect, not the first connection.
	//
	// Every frame sent while the socket was down is gone. The hub does
	// keep a small pending set per client — it coalesces, it does not
	// buffer — but a client that disconnects is removed from the hub
	// outright, and that set goes with it. The counts snapshot a new
	// connection receives covers only the counts. An invalidation that
	// happened in the gap is simply never delivered, so a page whose
	// laptop slept through the enrollment window opening keeps showing
	// it closed, indefinitely, until the student does something that
	// happens to refetch.
	//
	// The gap is not rare and not short: a sleeping laptop, a NAT
	// timeout, a wifi handover between buildings. The remedy is the
	// same one the first connection uses — read everything — and it is
	// cheap enough that being liberal about it costs nothing.
	onreconnect?: () => void
	// Called when the server has closed this socket for a reason that
	// retrying cannot fix, so this page will not receive live updates
	// again until it is reloaded. Currently one reason: too many tabs
	// open for one account.
	//
	// Something has to say so. A page that quietly stops reconnecting
	// is a page showing a snapshot of some earlier moment with nothing
	// to indicate it, which during a selection window is worse than
	// the churn it replaced.
	ongiveup?: (reason: string) => void
}

// The server's code for "you have too many sockets open"; see
// wsStatusTooManySockets in internal/web/websocket.go. Retrying it is
// what made the eviction policy perpetual: the evicted tab reconnects,
// evicting the next oldest, which reconnects.
const statusTooManySockets = 4001

// Reconnect delays.
//
// Backoff, because a server that has just come back does not want
// twelve hundred browsers arriving at once, and a server that is still
// down does not want them arriving every three seconds for the whole
// outage — one page made 86 attempts over seven minutes of downtime.
//
// Jitter, because the delay was a constant and the disconnect is
// simultaneous: killing the server put four pages into a 30-millisecond
// window and kept them there, each firing seven API calls the moment
// they reconnected. A constant interval cannot disperse a herd it did
// not create.
const reconnectBaseDelay = 1000
const reconnectMaxDelay = 30000

// How long a socket may go without a frame before it is presumed dead.
//
// The server sends a heartbeat every 45 seconds (wsPingInterval in
// internal/web/websocket.go), so this is two missed ones and some
// slack. It exists because a socket does not always close when the
// path under it dies: a NAT rebind, a VPN drop or a laptop waking up
// leaves readyState at OPEN with no event ever firing, and a page in
// that state is a snapshot of some earlier moment that nothing will
// ever correct. One was measured showing "Enrollment is closed", with
// the wrong date under it, straight through the window opening.
//
// The protocol-level ping cannot serve here: the browser answers it
// itself and JavaScript never sees it.
const staleAfter = 110000
const watchdogInterval = 15000

// Reconnects until the returned cleanup function is called.
export function connectEvents(
	path: string,
	handlers: EventHandlers,
): () => void {
	const url = new URL(path, window.location.href)
	url.protocol = url.protocol === "http:" ? "ws:" : "wss:"

	let socket: WebSocket | null = null
	let timer: number | null = null
	let closed = false
	let attempt = 0
	let lastFrameAt = Date.now()
	// The first open is not a reconnect: whoever called this has just
	// loaded everything, and telling them to load it again is waste.
	let everOpened = false

	function open(): void {
		socket = new WebSocket(url)
		socket.onopen = (): void => {
			attempt = 0
			lastFrameAt = Date.now()
			if (everOpened) {
				handlers.onreconnect?.()
			}
			everOpened = true
		}
		socket.onmessage = (event): void => {
			// Any frame at all, whatever it says, is proof the path is
			// alive. The heartbeat says nothing else and falls through
			// the loop below unmatched.
			lastFrameAt = Date.now()

			// One frame carries whatever a server send cycle coalesced.
			for (const data of String(event.data).split("\n")) {
				if (data.startsWith("invalidate_")) {
					handlers.oninvalidate?.(data.slice("invalidate_".length))
				} else if (data.startsWith("course_count_update,")) {
					const rest = data.slice("course_count_update,".length)
					const cut = rest.lastIndexOf(",")
					const count = Number.parseInt(rest.slice(cut + 1), 10)
					if (cut > 0 && !Number.isNaN(count)) {
						handlers.oncoursecount?.(rest.slice(0, cut), count)
					}
				}
			}
		}
		socket.onclose = (event): void => {
			if (closed) {
				return
			}

			if (event.code === statusTooManySockets) {
				closed = true
				handlers.ongiveup?.(
					"Too many tabs are open for this account, so this one " +
						"is no longer receiving live updates. Close some " +
						"and reload this page.",
				)

				return
			}

			// Exponential up to the cap, then a full random spread over
			// the whole interval rather than a small wobble around it:
			// the point is to break the phase lock, and only full
			// jitter does that in one step.
			const ceiling = Math.min(
				reconnectMaxDelay,
				reconnectBaseDelay * 2 ** attempt,
			)
			attempt++
			timer = window.setTimeout(open, Math.random() * ceiling)
		}
	}

	// Force the socket shut so that onclose runs and the reconnect
	// path takes over. Closing it is the whole repair: everything that
	// follows — the backoff, the resync — already exists.
	function presumeDead(): void {
		if (socket !== null && socket.readyState === WebSocket.OPEN) {
			socket.close()
		}
	}

	const watchdog = window.setInterval((): void => {
		if (!closed && Date.now() - lastFrameAt > staleAfter) {
			presumeDead()
		}
	}, watchdogInterval)

	// Coming back from offline is the one case the browser will tell us
	// about, and it is worth taking: a socket that was open across a
	// sleep or a lost network is dead whatever readyState says, and
	// waiting out the watchdog would leave the page stale for another
	// minute and a half for no reason.
	const onOnline = (): void => {
		if (!closed) {
			presumeDead()
		}
	}

	window.addEventListener("online", onOnline)

	open()

	return (): void => {
		closed = true
		window.clearInterval(watchdog)
		window.removeEventListener("online", onOnline)
		if (timer !== null) {
			window.clearTimeout(timer)
		}
		socket?.close()
	}
}
