package web

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
)

// The hub key for one administrator's connections.
//
// Per administrator, not shared. It used to be the constant "ADMIN"
// for everyone, which quietly turned wsMaxPerSubject — a cap on how
// many sockets *one identity* may hold — into a cap of eight for the
// whole school. The ninth admin tab anywhere evicted the oldest, that
// page reconnected three seconds later and evicted the next, and every
// administrator's page sat in a permanent reconnect loop, each
// reconnect reloading every resource it had ever shown. Three
// administrators with three tabs each is enough to start it.
//
// The prefix keeps it out of the student keyspace twice over: a
// localpart is lowercase by domain, and ':' is excluded from every
// identifier domain there is.
func wsAdminKey(username string) string {
	return "ADMIN:" + username
}

func (app *Server) apiEvents(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	conn, err := websocket.Accept(w, r, upgraderOpts)
	if err != nil {
		app.logError(r, logMsgAdminEventsUpgradeError, slog.Any("error", err), slog.String("admin_username", aui.Username))

		return
	}

	client := newAdminWSClient(app.wsHub, conn, wsAdminKey(aui.Username))
	app.wsHub.register(client)

	// Bounded, like every other frame write. Unbounded, a peer whose
	// receive window never opens parks this handler goroutine
	// forever, holding a hijacked connection that shutdown will not
	// reclaim.
	helloCtx, cancelHello := context.WithTimeout(context.WithoutCancel(r.Context()), wsWriteTimeout)
	err = conn.Write(helloCtx, websocket.MessageText, []byte("hello"))

	cancelHello()

	if err != nil {
		app.logError(r, logMsgAdminEventsHelloError, slog.Any("error", err), slog.String("admin_username", aui.Username))
		app.wsHub.remove(client)

		_ = conn.Close(websocket.StatusInternalError, "")

		return
	}

	// Detached from the request, which is about to return: the
	// connection is hijacked and the pumps must outlive it. Derived
	// from it, so the request's logging values come along.
	sessionCtx := context.WithoutCancel(r.Context())

	go client.sendPump(sessionCtx)
	go client.readPump(sessionCtx)

	app.logInfo(r, logMsgAdminEventsEstablished, slog.String("admin_username", aui.Username))
}
