package web //nolint:testpackage

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/PaoDevelopers/cca/internal/db"
)

// The hub's design rests on claims its comments make but nothing
// checked: that a coalesced event is delivered once, that a client
// behind on counts is brought up to the latest value rather than
// replayed through every intermediate one, and that none of this
// corrupts under concurrent writers. These pin those claims.
//
// A nil *websocket.Conn is fine here: every test drives the hub and
// the client's queue directly, and never reaches sendPump, which is
// the only thing that touches the connection.

func testHub() *WebSocketHub {
	return NewWebSocketHub(db.New(fakeDBTX{err: nil}))
}

// setCount mimics one refresher publish without running Run, which
// would need a live database.
func (h *WebSocketHub) setCountForTest(id string, count int64) {
	h.countsMu.Lock()
	defer h.countsMu.Unlock()

	h.version++
	h.counts[id] = countEntry{count: count, version: h.version}
}

func countFrame(id string, count int64) WSMessage {
	return WSMessage("course_count_update," + id + "," + strconv.FormatInt(count, 10))
}

func TestEnqueueCoalescesDuplicateEvents(t *testing.T) {
	t.Parallel()

	client := newWSClient(testHub(), nil, "s1")

	client.enqueue("invalidate_courses")
	client.enqueue("invalidate_courses")
	client.enqueue("invalidate_grades")
	client.enqueue("invalidate_courses")

	got := client.takePending()
	want := []WSMessage{"invalidate_courses", "invalidate_grades"}

	if !slices.Equal(got, want) {
		t.Errorf("pending = %v, want %v", got, want)
	}

	if rest := client.takePending(); len(rest) != 0 {
		t.Errorf("queue not drained: %v", rest)
	}
}

// A fresh client's cursor is zero, which is behind every count, so its
// first send carries the whole snapshot.
func TestFirstSendCarriesEveryCount(t *testing.T) {
	t.Parallel()

	hub := testHub()
	hub.setCountForTest("BB", 3)
	hub.setCountForTest("CH", 7)

	var cursor uint64

	frames := hub.countFramesSince(&cursor)
	slices.Sort(frames)

	want := []WSMessage{countFrame("BB", 3), countFrame("CH", 7)}
	if !slices.Equal(frames, want) {
		t.Errorf("first send = %v, want %v", frames, want)
	}

	if again := hub.countFramesSince(&cursor); len(again) != 0 {
		t.Errorf("second send with no changes = %v, want nothing", again)
	}
}

// The claim in the Client doc comment: a slow reader "only ever misses
// values that a newer value for the same course supersedes". Three
// updates to one course while the client is idle must collapse to one
// frame carrying the last value — not three frames, and not the first.
func TestSlowClientGetsOnlyTheLatestValue(t *testing.T) {
	t.Parallel()

	hub := testHub()
	hub.setCountForTest("BB", 1)
	// A second, untouched course, so an implementation that ignored the
	// cursor and resent everything would be caught here too.
	hub.setCountForTest("CH", 9)

	var cursor uint64

	hub.countFramesSince(&cursor) // client is up to date

	hub.setCountForTest("BB", 2)
	hub.setCountForTest("BB", 3)
	hub.setCountForTest("BB", 4)

	frames := hub.countFramesSince(&cursor)

	want := []WSMessage{countFrame("BB", 4)}
	if !slices.Equal(frames, want) {
		t.Errorf("frames = %v, want %v (superseded values must not be replayed)", frames, want)
	}
}

// A course that did not change must not be resent just because a
// different one did.
func TestOnlyChangedCoursesAreResent(t *testing.T) {
	t.Parallel()

	hub := testHub()
	hub.setCountForTest("BB", 1)
	hub.setCountForTest("CH", 1)

	var cursor uint64

	hub.countFramesSince(&cursor)
	hub.setCountForTest("CH", 2)

	frames := hub.countFramesSince(&cursor)

	want := []WSMessage{countFrame("CH", 2)}
	if !slices.Equal(frames, want) {
		t.Errorf("frames = %v, want %v", frames, want)
	}
}

// Whichever pump notices the failure first calls remove, and both may.
// The second call must not re-close done.
func TestRemoveIsIdempotent(t *testing.T) {
	t.Parallel()

	hub := testHub()
	client := newWSClient(hub, nil, "s42")

	hub.register(client)
	hub.remove(client)
	hub.remove(client) // must not panic on a second close of done

	select {
	case <-client.done:
	default:
		t.Error("done was not closed by remove")
	}

	hub.mu.RLock()
	defer hub.mu.RUnlock()

	if _, ok := hub.clients["s42"]; ok {
		t.Error("client set for the student was left behind after removal")
	}
}

// One student's several sessions all get their own events; another
// student's sessions get none of them.
func TestBroadcastToStudentsIsTargeted(t *testing.T) {
	t.Parallel()

	hub := testHub()
	first := newWSClient(hub, nil, "s1")
	second := newWSClient(hub, nil, "s1") // same student, second tab
	other := newWSClient(hub, nil, "s2")

	for _, c := range []*Client{first, second, other} {
		hub.register(c)
	}

	hub.BroadcastToStudents([]string{"s1"}, "invalidate_enrollments")

	for i, c := range []*Client{first, second} {
		if got := c.takePending(); !slices.Equal(got, []WSMessage{"invalidate_enrollments"}) {
			t.Errorf("session %d of the target student got %v, want the event", i, got)
		}
	}

	if got := other.takePending(); len(got) != 0 {
		t.Errorf("a different student received %v, want nothing", got)
	}
}

func TestBroadcastReachesEveryStudent(t *testing.T) {
	t.Parallel()

	hub := testHub()
	first := newWSClient(hub, nil, "s1")
	other := newWSClient(hub, nil, "s2")

	hub.register(first)
	hub.register(other)
	hub.Broadcast("invalidate_courses")

	for _, c := range []*Client{first, other} {
		if got := c.takePending(); !slices.Equal(got, []WSMessage{"invalidate_courses"}) {
			t.Errorf("student %s got %v, want the event", c.studentID, got)
		}
	}
}

// Marks are a set: repeats collapse, and the empty id (which a caller
// can produce from a missing course) is dropped rather than queued.
func TestMarkCoursesDirtyCoalescesAndIgnoresEmpty(t *testing.T) {
	t.Parallel()

	hub := testHub()
	hub.MarkCoursesDirty("BB", "BB", "", "CH")

	hub.dirtyMu.Lock()
	defer hub.dirtyMu.Unlock()

	if len(hub.dirty) != 2 {
		t.Errorf("dirty = %v, want exactly BB and CH", hub.dirty)
	}

	if _, ok := hub.dirty[""]; ok {
		t.Error("the empty course id was marked dirty")
	}
}

// MarkCoursesDirty is called from request handlers and documented as
// never blocking, including when the refresher is not running and
// nothing drains dirtyWake.
func TestMarkCoursesDirtyDoesNotBlockWithoutARefresher(t *testing.T) {
	t.Parallel()

	hub := testHub()
	done := make(chan struct{})

	go func() {
		defer close(done)

		for i := range 100 {
			hub.MarkCoursesDirty("C" + strconv.Itoa(i))
		}
	}()

	select {
	case <-done:
	case <-t.Context().Done():
		t.Fatal("MarkCoursesDirty blocked with an undrained wake channel")
	}
}

// Run with -race: registration, removal, broadcasts and count publishes
// all touch shared maps behind different locks.
func TestHubSurvivesConcurrentUse(t *testing.T) {
	t.Parallel()

	hub := testHub()

	var wg sync.WaitGroup

	for worker := range 8 {
		wg.Go(func() {
			for i := range 50 {
				client := newWSClient(hub, nil, "s"+strconv.Itoa(worker))
				hub.register(client)
				hub.Broadcast("invalidate_courses")
				hub.BroadcastToStudents([]string{"s" + strconv.Itoa(worker)}, "invalidate_enrollments")
				hub.setCountForTest("C"+strconv.Itoa(i%5), int64(i))
				hub.MarkCoursesDirty("C" + strconv.Itoa(i%5))

				var cursor uint64

				hub.countFramesSince(&cursor)
				client.takePending()
				hub.remove(client)
			}
		})
	}

	wg.Wait()

	hub.mu.RLock()
	defer hub.mu.RUnlock()

	if len(hub.clients) != 0 {
		t.Errorf("clients left registered after every client was removed: %v", hub.clients)
	}
}

// Shutdown closes every socket. A client left registered would be a
// goroutine still holding a connection the process is about to drop.
func TestCloseAllDisconnectsEveryClient(t *testing.T) {
	t.Parallel()

	hub := testHub()

	clients := make([]*Client, 0, 6)

	for student := range 3 {
		for range 2 {
			client := newWSClient(hub, nil, "s"+strconv.Itoa(student))
			hub.register(client)
			clients = append(clients, client)
		}
	}

	hub.CloseAll()

	for i, client := range clients {
		select {
		case <-client.done:
		default:
			t.Errorf("client %d was not closed", i)
		}
	}

	hub.mu.RLock()
	defer hub.mu.RUnlock()

	if len(hub.clients) != 0 {
		t.Errorf("clients still registered after CloseAll: %v", hub.clients)
	}
}

// A course that no longer exists must leave the mirror. Two things go
// wrong otherwise: the map grows across every season of course churn,
// and — worse — an id created again later inherits the old count, so
// the snapshot a newly connected client receives overwrites the
// correct value it had just fetched over HTTP with a stale one.
func TestDeletedCoursesLeaveTheCountsMirror(t *testing.T) {
	t.Parallel()

	hub := testHub()
	hub.setCountForTest("GONE", 20)
	hub.setCountForTest("STAYS", 5)

	// What a refresh does when the query answers for only one of the
	// two courses it asked about.
	hub.reconcileCounts(
		[]string{"GONE", "STAYS"},
		[]db.GetCourseCountsByIDsRow{{ID: "STAYS", CurrentStudents: 7}},
	)

	hub.countsMu.RLock()
	defer hub.countsMu.RUnlock()

	if _, still := hub.counts["GONE"]; still {
		t.Error("a course that no longer exists was left in the mirror")
	}

	if entry, ok := hub.counts["STAYS"]; !ok || entry.count != 7 {
		t.Errorf("the surviving course reads %+v, want 7", entry)
	}
}

// Every connected client must be told the same thing.
//
// A school opening its window holds around 1200 of these at once, and
// the fan-out has to be fair: a client that is behind is brought up to
// the current value, not left short and not replayed through every
// intermediate one. Measured against a real fleet of 1200 sockets,
// each received exactly the same number of frames; this is that
// property at a size a test can hold.
func TestEveryClientIsToldTheSameCounts(t *testing.T) {
	t.Parallel()

	hub := testHub()

	const fleet = 200

	clients := make([]*Client, fleet)
	cursors := make([]uint64, fleet)

	for i := range clients {
		clients[i] = newWSClient(hub, nil, "s"+strconv.Itoa(i))
		hub.register(clients[i])
	}

	// Some clients drain immediately, some lag behind, and some
	// connect late — the three states a real fleet is always in.
	hub.setCountForTest("A", 1)

	for i := range clients {
		if i%3 == 0 {
			hub.countFramesSince(&cursors[i])
		}
	}

	hub.setCountForTest("A", 2)
	hub.setCountForTest("B", 7)

	// Now everyone catches up.
	seen := make([]map[string]string, fleet)

	for i := range clients {
		seen[i] = make(map[string]string)
		for _, frame := range hub.countFramesSince(&cursors[i]) {
			parts := strings.Split(string(frame), ",")
			if len(parts) != 3 {
				t.Fatalf("client %d got a malformed frame %q", i, frame)
			}

			// A client must never be handed the same course twice in
			// one catch-up: that is the replay the versioning exists
			// to prevent.
			if _, twice := seen[i][parts[1]]; twice {
				t.Fatalf("client %d was sent %s twice in one cycle", i, parts[1])
			}

			seen[i][parts[1]] = parts[2]
		}
	}

	// Whatever each client had seen before, they now agree on the
	// current value of everything they were told about.
	for i := range clients {
		for course, value := range seen[i] {
			want := map[string]string{"A": "2", "B": "7"}[course]
			if value != want {
				t.Fatalf("client %d was told %s=%s, want %s", i, course, value, want)
			}
		}

		// Everyone hears about B, which changed after every cursor
		// was set; only the clients that had already drained miss A's
		// first value, and they get its second.
		if _, ok := seen[i]["B"]; !ok {
			t.Fatalf("client %d was never told about B", i)
		}

		if _, ok := seen[i]["A"]; !ok {
			t.Fatalf("client %d was never told about A", i)
		}
	}
}

// The hub holds a fleet without leaking it: every client that
// registers must be removable, and removal must leave nothing behind.
// Against a real server, 4800 connect/disconnect cycles left the file
// descriptor and thread counts exactly where they started.
func TestTheHubReleasesAWholeFleet(t *testing.T) {
	t.Parallel()

	hub := testHub()

	const fleet = 500

	clients := make([]*Client, fleet)
	for i := range clients {
		clients[i] = newWSClient(hub, nil, "s"+strconv.Itoa(i%50))
		hub.register(clients[i])
	}

	hub.mu.RLock()
	students := len(hub.clients)
	hub.mu.RUnlock()

	if students != 50 {
		t.Fatalf("hub holds %d students, want 50", students)
	}

	for _, c := range clients {
		hub.remove(c)
	}

	hub.mu.RLock()
	defer hub.mu.RUnlock()

	// Not merely empty sets: no entry at all, or the map grows by one
	// per student who ever connected and never shrinks.
	if len(hub.clients) != 0 {
		t.Fatalf("hub still holds %d student entries after removing everyone", len(hub.clients))
	}
}

// A session cookie is a bearer token with no server-side record, so
// one identity is however many sockets hold a copy of it. The cap
// evicts the oldest rather than refusing the newest, because the way
// this is reached in practice is a reconnect loop, where the newest
// socket is the live one.
func TestOneIdentityCannotHoldUnboundedSockets(t *testing.T) {
	t.Parallel()

	hub := testHub()

	opened := make([]*Client, 0, wsMaxPerSubject*3)
	for range cap(opened) {
		client := newWSClient(hub, nil, "s1")
		hub.register(client)
		opened = append(opened, client)
	}

	hub.mu.RLock()
	held := len(hub.clients["s1"])
	hub.mu.RUnlock()

	if held != wsMaxPerSubject {
		t.Errorf("the hub holds %d sockets for one identity, want the cap of %d", held, wsMaxPerSubject)
	}

	// The survivors are the newest, and every evicted one was told.
	for i, client := range opened {
		evicted := i < len(opened)-wsMaxPerSubject

		select {
		case <-client.done:
			if !evicted {
				t.Errorf("socket %d was evicted; it is among the newest %d", i, wsMaxPerSubject)
			}
		default:
			if evicted {
				t.Errorf("socket %d is past the cap but was never closed", i)
			}
		}
	}

	// A different identity is unaffected by the first one's flood.
	other := newWSClient(hub, nil, "s2")
	hub.register(other)

	hub.mu.RLock()
	defer hub.mu.RUnlock()

	if got := len(hub.clients["s2"]); got != 1 {
		t.Errorf("the second student holds %d sockets, want 1", got)
	}
}

// MarkAllDirty is the repair for a change nobody named.
//
// Clearing every enrollment sets every count to zero at once, and
// nothing marks those courses dirty, because only a course that was
// written is published. The mirror cannot notice on its own, so every
// connected browser keeps showing the old seats — indefinitely, since
// the counts a reconnecting client receives come from the same stale
// mirror.
func TestMarkAllDirtyMarksEveryCourseTheMirrorHolds(t *testing.T) {
	t.Parallel()

	hub := testHub()
	for _, id := range []string{"BB", "BK", "CH"} {
		hub.setCountForTest(id, 3)
	}

	hub.MarkAllDirty()

	hub.dirtyMu.Lock()
	defer hub.dirtyMu.Unlock()

	for _, id := range []string{"BB", "BK", "CH"} {
		if _, marked := hub.dirty[id]; !marked {
			t.Errorf("course %s was not marked dirty", id)
		}
	}

	if len(hub.dirty) != 3 {
		t.Errorf("marked %d courses, want 3", len(hub.dirty))
	}

	// And the refresher was woken, or the marks sit there until the
	// next unrelated write happens along.
	select {
	case <-hub.dirtyWake:
	default:
		t.Error("the refresher was not woken")
	}
}

// A hub that has published nothing has nothing to re-read, and must
// not wedge on that.
func TestMarkAllDirtyOnAnEmptyMirrorIsHarmless(t *testing.T) {
	t.Parallel()

	hub := testHub()
	hub.MarkAllDirty()

	hub.dirtyMu.Lock()
	defer hub.dirtyMu.Unlock()

	if len(hub.dirty) != 0 {
		t.Errorf("an empty mirror marked %d courses", len(hub.dirty))
	}
}

// Run's loop, without a database that can answer: the point is that
// cancelling the context ends it rather than leaving the goroutine
// spinning on a wake channel nobody will close.
func TestRunStopsWhenItsContextIsCancelled(t *testing.T) {
	t.Parallel()

	hub := testHub()
	ctx, cancel := context.WithCancel(t.Context())

	stopped := make(chan struct{})

	go func() {
		hub.Run(ctx)
		close(stopped)
	}()

	// Give it a mark to chew on first, so the cancellation is racing a
	// working loop rather than an idle one.
	hub.MarkCoursesDirty("BB")
	cancel()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}

// The cap counts one identity's sockets, and administrators are
// separate identities.
//
// They used to share the hub key "ADMIN", which made wsMaxPerSubject a
// cap of eight for every administrator in the school at once. The
// ninth tab anywhere evicted the oldest; that page reconnected three
// seconds later and evicted the next; and every administrator's page
// sat in a permanent loop, each reconnect reloading every resource it
// had ever displayed. Three administrators with three tabs each is
// enough to start it, and only closing tabs stops it.
func TestTheSocketCapIsPerAdministratorAndNotPerSchool(t *testing.T) {
	t.Parallel()

	app := testServer(nil)

	// Well past the cap in total, and well within it for each person.
	admins := []string{"first.admin", "second.admin", "third.admin"}
	opened := make(map[string][]*Client, len(admins))

	for range wsMaxPerSubject - 1 {
		for _, admin := range admins {
			client := newAdminWSClient(app.wsHub, nil, wsAdminKey(admin))
			app.wsHub.register(client)
			opened[admin] = append(opened[admin], client)
		}
	}

	for _, admin := range admins {
		for i, client := range opened[admin] {
			select {
			case <-client.done:
				t.Errorf("%s socket %d was evicted; %s holds %d sockets and the cap is %d",
					admin, i, admin, len(opened[admin]), wsMaxPerSubject)
			default:
			}
		}
	}

	// And the group broadcast still reaches all of them, which is what
	// the shared key was doing before.
	app.wsHub.BroadcastToStudentsAndAdmins(nil, WSMessage("invalidate_enrollments"))

	for _, admin := range admins {
		for i, client := range opened[admin] {
			if got := client.takePending(); !slices.Contains(got, WSMessage("invalidate_enrollments")) {
				t.Errorf("%s socket %d got %v, want the enrollment invalidation", admin, i, got)
			}
		}
	}
}

// The other half: one administrator opening tabs without limit is
// still capped, because the reason for the cap has not gone away.
func TestOneAdministratorIsStillCapped(t *testing.T) {
	t.Parallel()

	app := testServer(nil)

	opened := make([]*Client, 0, wsMaxPerSubject*2)

	for range wsMaxPerSubject * 2 {
		client := newAdminWSClient(app.wsHub, nil, wsAdminKey("busy.admin"))
		app.wsHub.register(client)
		opened = append(opened, client)
	}

	app.wsHub.mu.RLock()
	held := len(app.wsHub.clients[wsAdminKey("busy.admin")])
	admins := len(app.wsHub.admins)
	app.wsHub.mu.RUnlock()

	if held != wsMaxPerSubject {
		t.Errorf("the hub holds %d sockets for one administrator, want %d", held, wsMaxPerSubject)
	}

	// The admin set has to shrink with the eviction too, or a closed
	// socket keeps receiving broadcasts for as long as the process
	// lives.
	if admins != wsMaxPerSubject {
		t.Errorf("the admin set holds %d clients, want %d; eviction is leaking", admins, wsMaxPerSubject)
	}

	// And the evicted ones are told why, with the code the client does
	// not retry. Without it the eviction does not settle: the loser
	// reconnects, evicts the next oldest, and the churn lasts as long
	// as the tabs are open. Measured before this: nineteen new sockets
	// and seventy-six full reloads in thirty idle seconds.
	for i, client := range opened[:len(opened)-wsMaxPerSubject] {
		select {
		case <-client.done:
		default:
			t.Errorf("socket %d is among the oldest and was not evicted", i)

			continue
		}

		if got := client.closeStatus.Load(); got != int32(wsStatusTooManySockets) {
			t.Errorf("socket %d closes with status %d, want %d; the client will retry it",
				i, got, wsStatusTooManySockets)
		}
	}

	// The survivors close normally, so a page that is merely being
	// navigated away from does reconnect.
	for i, client := range opened[len(opened)-wsMaxPerSubject:] {
		if got := client.closeStatus.Load(); got != 0 {
			t.Errorf("surviving socket %d carries close status %d, want none", i, got)
		}
	}
}

// The socket when nothing is happening, which is most of the time and
// was covered by nothing.
//
// Every test passed a nil connection and never started the pumps, so
// the ping cycle — the only thing that detects a peer that vanished
// without closing, and now also the only thing a browser can use to
// tell a quiet socket from a dead one — ran in production and nowhere
// else.
//
// In a synctest bubble, because the interval is forty-five seconds.

type fakeConn struct {
	mu      sync.Mutex
	writes  []string
	pings   int
	closed  bool
	code    websocket.StatusCode
	reason  string
	blocked chan struct{}
}

func newFakeConn() *fakeConn {
	//exhaustruct:ignore
	return &fakeConn{blocked: make(chan struct{})}
}

// Read blocks until the connection is closed, which is what a real one
// does while the peer is quiet.
func (f *fakeConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	select {
	case <-ctx.Done():
		return 0, nil, fmt.Errorf("fake read: %w", ctx.Err())
	case <-f.blocked:
		return 0, nil, net.ErrClosed
	}
}

func (f *fakeConn) Write(_ context.Context, _ websocket.MessageType, p []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.writes = append(f.writes, string(p))

	return nil
}

func (f *fakeConn) Ping(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.pings++

	return nil
}

func (f *fakeConn) Close(code websocket.StatusCode, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.closed {
		f.closed = true
		f.code = code
		f.reason = reason
		close(f.blocked)
	}

	return nil
}

func (f *fakeConn) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.writes)
}

func TestAnIdleSocketPingsAndSaysSoInBandToo(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		app := testServer(nil)
		conn := newFakeConn()
		client := newWSClient(app.wsHub, conn, "s1")
		app.wsHub.register(client)

		go client.sendPump(t.Context())

		// A bubble will not let its goroutines outlive it, which is
		// the right rule: a pump still running is a test that did not
		// finish.
		defer func() {
			client.once.Do(func() { close(client.done) })
			synctest.Wait()
		}()

		// Two intervals of the bubble's clock, which costs nothing.
		time.Sleep(2*wsPingInterval + time.Second)
		synctest.Wait()

		conn.mu.Lock()
		pings := conn.pings
		conn.mu.Unlock()

		// The protocol ping is what reaps a peer that went away
		// without closing: a write only happens when there is traffic,
		// and there may be none for hours.
		if pings < 2 {
			t.Errorf("an idle socket sent %d pings in two intervals, want 2", pings)
		}

		// And the heartbeat is the half of it a browser can see. A
		// protocol ping is answered by the browser itself and is
		// invisible to JavaScript, so without this a page has no way
		// to tell a quiet socket from a black-holed one — and one that
		// cannot tell shows a stale answer forever.
		beats := 0

		for _, frame := range conn.sent() {
			if frame == string(wsHeartbeat) {
				beats++
			}
		}

		if beats < 2 {
			t.Errorf("an idle socket sent %d heartbeats in two intervals, want 2", beats)
		}
	})
}

// The close code an evicted socket carries, on the way out through the
// pump rather than only in the field the pump reads.
func TestAnEvictedSocketClosesWithTheCodeTheClientWillNotRetry(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		app := testServer(nil)

		conns := make([]*fakeConn, 0, wsMaxPerSubject+1)
		clients := make([]*Client, 0, wsMaxPerSubject+1)

		for range wsMaxPerSubject + 1 {
			conn := newFakeConn()
			client := newWSClient(app.wsHub, conn, "s1")
			app.wsHub.register(client)

			go client.sendPump(t.Context())

			conns = append(conns, conn)
			clients = append(clients, client)
		}

		defer func() {
			for _, client := range clients {
				client.once.Do(func() { close(client.done) })
			}

			synctest.Wait()
		}()

		synctest.Wait()

		oldest := conns[0]
		oldest.mu.Lock()
		closed, code := oldest.closed, oldest.code
		oldest.mu.Unlock()

		if !closed {
			t.Fatal("the oldest socket was not closed")
		}

		if code != wsStatusTooManySockets {
			t.Errorf("closed with %d, want %d; the client retries anything else",
				code, wsStatusTooManySockets)
		}

		// The survivors are untouched.
		for i, conn := range conns[1:] {
			conn.mu.Lock()
			gone := conn.closed
			conn.mu.Unlock()

			if gone {
				t.Errorf("socket %d was closed; it is among the newest %d", i+1, wsMaxPerSubject)
			}
		}
	})
}

// flakyCourseDB fails the catalogue read a fixed number of times and
// then answers, so a test can watch what the hub does about it.
type flakyCourseDB struct {
	mu        sync.Mutex
	calls     int
	failUntil int
}

func (d *flakyCourseDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (d *flakyCourseDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return errRow{scanErr: nil}
}

func (d *flakyCourseDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.calls++
	if d.calls <= d.failUntil {
		return nil, errSeedRead
	}

	return emptyRows{}, nil
}

func (d *flakyCourseDB) reads() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.calls
}

var errSeedRead = errors.New("flakyCourseDB: the catalogue read failed")

// A hiccup while the counts mirror is being filled must not disable
// seat counts for the life of the process.
//
// It did. The seed ran once and fell through with an empty map, and
// nothing ever re-seeded: one ERROR line at boot, then silence, every
// probe green, and a course entered the mirror only if somebody
// happened to write it. Two seconds of a VACUUM FULL or a migration
// lock at the wrong moment was enough — and a boot under the
// reconnect rush is exactly when the database is slowest. The inner
// refresh loop always retried; only the seed did not.
func TestTheCountsMirrorKeepsAskingUntilItIsFilled(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		fake := &flakyCourseDB{failUntil: 3} //exhaustruct:ignore
		hub := NewWebSocketHub(db.New(fake))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go hub.Run(ctx)

		synctest.Wait()

		if n := fake.reads(); n != 1 {
			t.Fatalf("asked %d times before waiting, want 1", n)
		}

		// Long enough for every failure and the read that succeeds.
		time.Sleep(4 * countsSeedRetry)
		synctest.Wait()

		if n := fake.reads(); n < 4 {
			t.Fatalf("asked %d times across four retry intervals; the "+
				"seed gave up and the mirror is blind for good", n)
		}

		// And having succeeded, it stops asking: the retry is for
		// getting started, not a poll.
		settled := fake.reads()

		time.Sleep(10 * countsSeedRetry)
		synctest.Wait()

		if n := fake.reads(); n != settled {
			t.Errorf("kept reading the catalogue after the mirror was "+
				"filled: %d then %d", settled, n)
		}
	})
}
