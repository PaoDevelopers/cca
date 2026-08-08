package web

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/PaoDevelopers/cca/internal/db"
)

// WSMessage is a text frame sent to student WebSocket clients.
type WSMessage string

// wsWriteTimeout bounds a single frame write so a dead peer cannot
// park its sender goroutine past TCP's own patience.
const wsWriteTimeout = 30 * time.Second

// wsCountsReadTimeout bounds one refresh of the counts mirror. It is
// generous: the query is small and indexed, so anything approaching
// this is a stuck lock rather than slow work.
const wsCountsReadTimeout = 15 * time.Second

// countsSeedRetry is how long to wait before trying to fill the counts
// mirror again. Short, because until it is filled no client is told
// how many seats a course has left, and the whole point of the mirror
// is that they are told promptly.
const countsSeedRetry = 5 * time.Second

// wsPingInterval is how often an idle connection is probed. Nothing
// else detects a peer that went away without a FIN — a sleeping
// laptop, a NAT that dropped the mapping — because a write only
// happens when there is traffic, and there may be none for hours.
// Without it those entries accumulate in the hub, each holding two
// goroutines and a file descriptor, and every broadcast walks them.
const wsPingInterval = 45 * time.Second

// wsHeartbeat is the visible half of the liveness check; see the ping
// cycle in sendPump. Named rather than spelled inline because the
// client's watchdog interval is derived from wsPingInterval and the
// two have to be read together.
const wsHeartbeat = WSMessage("heartbeat")

// wsMinSendInterval is the cooldown after a send cycle. The first
// update after a quiet period still goes out immediately; under
// sustained churn this bounds fan-out CPU at high client counts.
const wsMinSendInterval = 100 * time.Millisecond

// wsMaxPerSubject bounds how many sockets one identity may hold.
//
// A session cookie is a bearer token with a three-day life and no
// server-side record, so "one student" is however many tabs, devices
// and scripted connections hold a copy of it. Each socket costs two
// goroutines, a file descriptor and a place in every broadcast's walk,
// and nothing in the protocol makes opening more of them expensive.
//
// A person legitimately has a handful: a laptop, a phone, a stale tab,
// and briefly two of each while a reconnect overlaps the socket it is
// replacing. Well above that, and far below what it takes to matter.
const wsMaxPerSubject = 8

// The close code an evicted socket is given.
//
// In the private range RFC 6455 sets aside for the application. It
// exists because the eviction is not a failure and must not be
// retried: a page that reconnects after being evicted evicts the next
// oldest, which reconnects, which evicts the next — so one person with
// nine tabs open put all nine into a permanent reconnect loop, each
// reconnect reloading every resource that tab had ever displayed. The
// comment on register used to call the policy self-clearing. It is
// self-clearing only if the loser stops asking.
const wsStatusTooManySockets websocket.StatusCode = 4001

// countEntry is one course's last known enrollee count, stamped with
// the hub-global version at which it was observed.
type countEntry struct {
	count   int64
	version uint64
}

// wsConn is the part of a websocket connection the pumps use.
//
// Four methods, named here rather than taken as *websocket.Conn, so
// that the pumps can be run against something in a test. They could
// not before: every test passed a nil connection and stopped short of
// starting them, which left the ping cycle, the heartbeat and the
// close codes — the whole of what a socket does when nothing is
// happening — covered by nothing.
type wsConn interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Write(ctx context.Context, typ websocket.MessageType, p []byte) error
	Ping(ctx context.Context) error
	Close(code websocket.StatusCode, reason string) error
}

// Client is one connected WebSocket. It carries no message queue:
// events coalesce in a pending set and counts are read from the hub's
// versioned state at send time, so a slow reader only ever misses
// values that a newer value for the same course supersedes.
type Client struct {
	hub       *WebSocketHub
	conn      wsConn
	studentID string
	// Whether this socket belongs to an administrator, for the group
	// broadcasts. Set at construction and never written again.
	admin bool //exhaustruct:optional

	mu      sync.Mutex  //exhaustruct:optional
	pending []WSMessage //exhaustruct:optional

	// cursor is the counts version this client has been brought up to;
	// only sendPump touches it. Zero means its first send delivers the
	// whole counts snapshot.
	cursor uint64 //exhaustruct:optional

	wake chan struct{}
	done chan struct{}
	once sync.Once //exhaustruct:optional

	// seq orders this identity's sockets by arrival, so that the cap
	// evicts the oldest. Assigned under the hub lock by register.
	seq uint64 //exhaustruct:optional

	// The status this socket should close with, when it is being
	// closed for a reason the client can act on. Zero means the
	// ordinary one. Atomic because it is set by whoever decided to
	// close the socket and read by the pump that does it.
	closeStatus atomic.Int32 //exhaustruct:optional
}

func newWSClient(hub *WebSocketHub, conn wsConn, studentID string) *Client {
	return &Client{
		hub:       hub,
		conn:      conn,
		studentID: studentID,
		wake:      make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
}

// newAdminWSClient is the same socket, marked so that the group
// broadcasts reach it. The key is still per administrator: being an
// administrator is what this records, not who.
func newAdminWSClient(hub *WebSocketHub, conn wsConn, key string) *Client {
	client := newWSClient(hub, conn, key)
	client.admin = true

	return client
}

// WebSocketHub distributes invalidation events and course enrollee
// counts. Counts are a versioned last-value map mirrored from the
// database by a single refresher goroutine (Run); mutation handlers
// only mark courses dirty, which never blocks.
type WebSocketHub struct {
	queries *db.Queries

	mu      sync.RWMutex //exhaustruct:optional
	clients map[string]map[*Client]struct{}
	// Every connected administrator, kept as its own set.
	//
	// Administrators used to share one hub key so that "tell every
	// administrator" could be spelt as a lookup in clients. That made
	// them one identity for the purposes of the per-identity socket
	// cap, which is not what they are. This is the other half of the
	// fix: they are keyed individually above, and addressed as a group
	// here.
	//
	// A set rather than a prefix scan over clients, because every
	// enrollment write addresses this and a scan would walk twelve
	// hundred student keys to find three administrators.
	admins map[*Client]struct{}

	countsMu sync.RWMutex //exhaustruct:optional
	counts   map[string]countEntry
	// Only ever rises, so a cursor of zero is behind every count.
	version uint64 //exhaustruct:optional

	// Monotonic, hub-wide, under mu. Only ever compared within one
	// identity's set, so wrapping would need 2^64 registrations.
	nextSeq uint64 //exhaustruct:optional

	dirtyMu   sync.Mutex //exhaustruct:optional
	dirty     map[string]struct{}
	dirtyWake chan struct{}
}

// NewWebSocketHub creates an empty hub; call Run to start the counts
// refresher.
func NewWebSocketHub(queries *db.Queries) *WebSocketHub {
	return &WebSocketHub{
		queries:   queries,
		clients:   make(map[string]map[*Client]struct{}),
		admins:    make(map[*Client]struct{}),
		counts:    make(map[string]countEntry),
		dirty:     make(map[string]struct{}),
		dirtyWake: make(chan struct{}, 1),
	}
}

// Run seeds the counts mirror, then until ctx is cancelled drains the
// dirty set, re-reads those counts, publishes the changed ones, and
// wakes every client. Cycles are serialized, so marks arriving during
// one coalesce into the next and publishes cannot reorder.
func (h *WebSocketHub) Run(ctx context.Context) {
	// Until the mirror is seeded there is nothing to publish, so this
	// keeps asking rather than proceeding blind.
	//
	// It used to try once and fall through with an empty map, and
	// nothing ever re-seeded it. A two-second hiccup at boot — a
	// VACUUM FULL, a migration lock, a slow first connection under the
	// reconnect rush — therefore disabled live seat counts for the
	// whole life of the process: one ERROR line, then silence, every
	// probe green, and students racing for seats that filled minutes
	// ago. A course re-entered the mirror only if somebody happened to
	// write it. The inner loop below always retried; only the seed did
	// not.
	for !h.seedCounts(ctx) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(countsSeedRetry):
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.dirtyWake:
		}

		h.dirtyMu.Lock()
		batch := h.dirty
		h.dirty = make(map[string]struct{})
		h.dirtyMu.Unlock()

		if len(batch) == 0 {
			continue
		}

		ids := make([]string, 0, len(batch))
		for id := range batch {
			ids = append(ids, id)
		}

		rows, err := h.queries.GetCourseCountsByIDs(h.readCtx(ctx), ids)
		if err != nil {
			slog.Error(logMsgWebsocketCountsRefreshError, slog.Any("error", err))
			// Put the marks back so the counts cannot silently stay
			// stale.
			h.MarkCoursesDirty(ids...)

			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}

			continue
		}

		changed := h.reconcileCounts(ids, rows)

		if changed {
			h.mu.RLock()

			for _, set := range h.clients {
				for client := range set {
					client.notify()
				}
			}

			h.mu.RUnlock()
		}
	}
}

// MarkCoursesDirty hands the given courses to the refresher. O(1) per
// course and never blocks, so it is safe on the request path.
func (h *WebSocketHub) MarkCoursesDirty(courseIDs ...string) {
	h.dirtyMu.Lock()
	for _, id := range courseIDs {
		if id != "" {
			h.dirty[id] = struct{}{}
		}
	}
	h.dirtyMu.Unlock()

	select {
	case h.dirtyWake <- struct{}{}:
	default:
	}
}

// Broadcast queues an event for every connected client. Duplicates
// coalesce until the client's sender drains them.
func (h *WebSocketHub) Broadcast(msg WSMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, set := range h.clients {
		for client := range set {
			client.enqueue(msg)
		}
	}
}

func (h *WebSocketHub) BroadcastToStudents(studentIDs []string, msg WSMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, id := range studentIDs {
		for client := range h.clients[id] {
			client.enqueue(msg)
		}
	}
}

// BroadcastToStudentsAndAdmins tells the named students and every
// administrator, which is what an enrollment write moves: the students
// whose own list changed, and whoever is watching the roster.
//
// One call rather than two, because it used to be one — every
// administrator shared a hub key, and reaching them was a matter of
// appending that key to the student list. Splitting it here is what
// let the key become per-administrator.
func (h *WebSocketHub) BroadcastToStudentsAndAdmins(studentIDs []string, msg WSMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, id := range studentIDs {
		for client := range h.clients[id] {
			client.enqueue(msg)
		}
	}

	for client := range h.admins {
		client.enqueue(msg)
	}
}

// MarkAllDirty re-reads every course the mirror holds.
//
// Used after a bulk reset, where the change is not to a course anybody
// named: clearing the enrollments sets every count to zero at once,
// and nothing marks those courses dirty, so the mirror — and every
// connected browser — keeps showing the old seats indefinitely. The
// hub cannot notice on its own; only a course that was written is
// published.
func (h *WebSocketHub) MarkAllDirty() {
	h.countsMu.RLock()
	ids := slices.Collect(maps.Keys(h.counts))
	h.countsMu.RUnlock()

	h.MarkCoursesDirty(ids...)
}

// CloseAll disconnects every client. Used at shutdown: the sockets
// would die with the process anyway, but closing them first means a
// browser reconnects on its own timer instead of waiting on a socket
// that will never speak again.
func (h *WebSocketHub) CloseAll() {
	// Snapshotted under the read lock, because remove takes the write
	// lock for each client.
	h.mu.RLock()

	var clients []*Client

	for _, set := range h.clients {
		for client := range set {
			clients = append(clients, client)
		}
	}

	h.mu.RUnlock()

	for _, client := range clients {
		h.remove(client)
	}
}

// seedCounts fills the mirror from the catalogue, reporting whether it
// managed to. A marked-dirty course arriving while this is retrying
// waits in the dirty set and is published once the mirror exists.
func (h *WebSocketHub) seedCounts(ctx context.Context) bool {
	rows, err := h.queries.GetCourses(h.readCtx(ctx))
	if err != nil {
		slog.Error(logMsgWebsocketCountsRefreshError, slog.Any("error", err))

		return false
	}

	h.countsMu.Lock()
	defer h.countsMu.Unlock()

	for _, row := range rows {
		h.version++
		h.counts[row.ID] = countEntry{
			count: row.CurrentStudents, version: h.version,
		}
	}

	return true
}

// reconcileCounts folds one refresh into the mirror and reports
// whether anything moved.
//
// ids is what was asked about; rows is what answered. A course in the
// first and not the second no longer exists, and its entry has to go:
// leaving it would grow the mirror across every season of course
// churn, and an id created again later would inherit the old count —
// so the snapshot a newly connected client receives would overwrite
// the correct value it had just fetched with a stale one.
func (h *WebSocketHub) reconcileCounts(ids []string, rows []db.GetCourseCountsByIDsRow) bool {
	present := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		present[row.ID] = struct{}{}
	}

	changed := false

	h.countsMu.Lock()
	defer h.countsMu.Unlock()

	for _, row := range rows {
		if e, ok := h.counts[row.ID]; ok && e.count == row.CurrentStudents {
			continue
		}

		h.version++
		h.counts[row.ID] = countEntry{count: row.CurrentStudents, version: h.version}
		changed = true
	}

	for _, id := range ids {
		if _, still := present[id]; !still {
			if _, had := h.counts[id]; had {
				delete(h.counts, id)

				changed = true
			}
		}
	}

	return changed
}

// isOrdinaryDisconnect reports whether a read failure is just a peer
// that left. A close frame says so politely; an EOF is the same thing
// without the courtesy, and is what a closed tab usually produces.
func isOrdinaryDisconnect(err error) bool {
	// Only these two are ordinary; every other close code, and the
	// -1 that means "no close frame at all", falls through to the
	// error checks below.
	status := websocket.CloseStatus(err)
	if status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
		return true
	}

	return errors.Is(err, io.EOF) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, context.Canceled)
}

// readCtx bounds one refresh. Without it a single query stuck on a
// lock freezes the only refresher goroutine for the process lifetime:
// marks keep accumulating in the dirty set and no client ever receives
// another count again, with nothing to notice or restart it.
func (h *WebSocketHub) readCtx(ctx context.Context) context.Context {
	// Not cancelled by the caller here — the parent already carries
	// shutdown, and the timeout is the part that matters.
	bounded, cancel := context.WithTimeout(ctx, wsCountsReadTimeout)
	context.AfterFunc(bounded, cancel)

	return bounded
}

// register adds a client and wakes it for its initial counts snapshot.
//
// Over wsMaxPerSubject sockets for one identity, the oldest is closed
// rather than the new one refused. A refusal punishes the wrong socket:
// the common way to exceed the cap is a reconnect loop, where the
// newest connection is the live one and the old ones are corpses the
// ping has not yet reaped.
//
// The evicted socket is closed with wsStatusTooManySockets, and the
// client does not retry that code. Without it the policy does not
// terminate: the loser reconnects, evicts the next oldest, and the
// churn is permanent for as long as the tabs are open.
func (h *WebSocketHub) register(client *Client) {
	h.mu.Lock()
	if _, ok := h.clients[client.studentID]; !ok {
		h.clients[client.studentID] = make(map[*Client]struct{})
	}

	h.nextSeq++
	client.seq = h.nextSeq
	set := h.clients[client.studentID]
	set[client] = struct{}{}

	if client.admin {
		h.admins[client] = struct{}{}
	}

	var evict []*Client

	for len(set) > wsMaxPerSubject {
		var oldest *Client

		for candidate := range set {
			if oldest == nil || candidate.seq < oldest.seq {
				oldest = candidate
			}
		}

		delete(set, oldest)
		delete(h.admins, oldest)

		evict = append(evict, oldest)
	}

	h.mu.Unlock()

	for _, stale := range evict {
		// Closing done is what sendPump selects on. readPump has no
		// select of its own: it is blocked in conn.Read and is
		// unblocked as a side effect of sendPump closing the
		// connection, which can take until the write deadline.
		//
		// The code is set before done is closed, because closing done
		// is what starts the teardown that reads it.
		stale.closeStatus.Store(int32(wsStatusTooManySockets))
		stale.once.Do(func() { close(stale.done) })
		slog.Info(logMsgWebsocketClientEvicted,
			slog.String("student_id", stale.studentID),
			slog.Int("limit", wsMaxPerSubject))
	}

	client.notify()
	slog.Info(logMsgWebsocketClientRegistered, slog.String("student_id", client.studentID))
}

// remove is idempotent; whichever pump fails first calls it.
func (h *WebSocketHub) remove(client *Client) {
	h.mu.Lock()
	if set, ok := h.clients[client.studentID]; ok {
		if _, exists := set[client]; exists {
			delete(set, client)
			delete(h.admins, client)

			if len(set) == 0 {
				delete(h.clients, client.studentID)
			}

			slog.Info(logMsgWebsocketClientUnregistered, slog.String("student_id", client.studentID))
		}
	}
	h.mu.Unlock()
	client.once.Do(func() { close(client.done) })
}

// countFramesSince renders one frame per course changed after cursor,
// and advances it.
func (h *WebSocketHub) countFramesSince(cursor *uint64) []WSMessage {
	h.countsMu.RLock()
	defer h.countsMu.RUnlock()

	if h.version <= *cursor {
		return nil
	}

	var frames []WSMessage

	for id, e := range h.counts {
		if e.version > *cursor {
			frames = append(frames, WSMessage("course_count_update,"+id+","+strconv.FormatInt(e.count, 10)))
		}
	}

	*cursor = h.version

	return frames
}

func (c *Client) enqueue(msg WSMessage) {
	c.mu.Lock()
	found := slices.Contains(c.pending, msg)

	if !found {
		c.pending = append(c.pending, msg)
	}
	c.mu.Unlock()
	c.notify()
}

func (c *Client) notify() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Client) takePending() []WSMessage {
	c.mu.Lock()
	msgs := c.pending
	c.pending = nil
	c.mu.Unlock()

	return msgs
}

// sendPump delivers pending events and count updates whenever woken,
// reading current values at send time. Its own write pace is the only
// throttle: a slow connection simply coalesces more per wake.
// The pumps take a context that outlives the request on purpose: the
// connection is hijacked, so tying them to r.Context() would tear them
// down the moment the handler returned. It is derived from the request
// rather than made from Background so that the request's logging
// values travel with it; only the cancellation is dropped.
// send writes one frame and reports whether the pump should carry on.
//
// One frame per cycle: newline-separated messages, so a coalesced
// batch costs one write.
func (c *Client) send(ctx context.Context, msgs []WSMessage) bool {
	var frame []byte

	for i, msg := range msgs {
		if i > 0 {
			frame = append(frame, '\n')
		}

		frame = append(frame, msg...)
	}

	writeCtx, cancel := context.WithTimeout(ctx, wsWriteTimeout)
	err := c.conn.Write(writeCtx, websocket.MessageText, frame)

	cancel()

	if err != nil {
		slog.Error(logMsgWebsocketWriteError, slog.Any("error", err))

		return false
	}

	return true
}

func (c *Client) sendPump(ctx context.Context) {
	defer func() {
		c.hub.remove(c)

		status := websocket.StatusNormalClosure
		reason := ""

		if code := c.closeStatus.Load(); code != 0 {
			status = websocket.StatusCode(code)
			reason = "too many connections for one account"
		}

		_ = c.conn.Close(status, reason)
	}()

	for {
		select {
		case <-c.done:
			return
		case <-c.wake:
		case <-time.After(wsPingInterval):
			// Nothing to say, so ask whether anyone is listening.
			// A peer that vanished without closing is detected here
			// and nowhere else.
			pingCtx, cancel := context.WithTimeout(ctx, wsWriteTimeout)
			err := c.conn.Ping(pingCtx)

			cancel()

			if err != nil {
				slog.Info(logMsgWebsocketPingFailed, slog.Any("error", err))

				return
			}

			// And the same question in the other direction, as
			// something the browser can see.
			//
			// A protocol ping is answered by the browser itself and is
			// invisible to JavaScript, so the page has no way to tell a
			// quiet socket from a dead one — and when the path is
			// black-holed rather than closed (a NAT rebind, a VPN drop,
			// a sleeping laptop) readyState stays OPEN and no event
			// ever fires. A page in that state showed "Enrollment is
			// closed" straight through the window opening, with the
			// wrong date under it, until somebody reloaded.
			//
			// This is the frame its watchdog counts from. The client
			// ignores the content.
			if !c.send(ctx, []WSMessage{wsHeartbeat}) {
				return
			}

			continue
		}

		msgs := c.takePending()

		msgs = append(msgs, c.hub.countFramesSince(&c.cursor)...)
		if len(msgs) == 0 {
			continue
		}

		if !c.send(ctx, msgs) {
			return
		}

		select {
		case <-c.done:
			return
		case <-time.After(wsMinSendInterval):
		}
	}
}

func (c *Client) readPump(ctx context.Context) {
	defer func() {
		c.hub.remove(c)

		status := websocket.StatusNormalClosure
		reason := ""

		if code := c.closeStatus.Load(); code != 0 {
			status = websocket.StatusCode(code)
			reason = "too many connections for one account"
		}

		_ = c.conn.Close(status, reason)
	}()

	for {
		// No deadline: this blocks for as long as the peer is quiet,
		// which is most of the time and is not a fault. What detects
		// a peer that has gone away silently is the ping in
		// sendPump, whose failure closes the connection and lands
		// here as a read error.
		_, _, err := c.conn.Read(ctx)
		if err != nil {
			// Every way a browser can leave ends up here: a close
			// frame, a bare EOF when the tab went away without one,
			// or the context ending at shutdown. None of them is a
			// fault, and logging them at error level buries the ones
			// that are.
			if isOrdinaryDisconnect(err) {
				slog.Info(logMsgWebsocketClientGone, slog.Any("error", err))
			} else {
				slog.Error(logMsgWebsocketReadError, slog.Any("error", err))
			}

			break
		}
	}
}
