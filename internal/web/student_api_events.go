package web

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
)

// Every accept option is left at its default; notably, origin checking
// stays on.
//
//nolint:gochecknoglobals
//exhaustruct:ignore
var upgraderOpts = &websocket.AcceptOptions{}

func (app *Server) handleStuAPIEvents(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPIEvents", slog.String("student_id", sui.ID))

	if r.Method != http.MethodGet {
		app.apiMethodNotAllowed(r, w)

		return
	}

	conn, err := websocket.Accept(w, r, upgraderOpts)
	if err != nil {
		app.logError(r, logMsgStudentEventsUpgradeError, slog.Any("error", err))

		return
	}

	client := newWSClient(app.wsHub, conn, sui.ID)
	app.wsHub.register(client)

	// Bounded, like every other frame write. Unbounded, a peer whose
	// receive window never opens parks this handler goroutine
	// forever, holding a hijacked connection that shutdown will not
	// reclaim.
	helloCtx, cancelHello := context.WithTimeout(context.WithoutCancel(r.Context()), wsWriteTimeout)
	err = conn.Write(helloCtx, websocket.MessageText, []byte("hello"))

	cancelHello()

	if err != nil {
		app.logError(r, logMsgStudentEventsHelloError, slog.Any("error", err))
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

	app.logInfo(r, logMsgStudentEventsEstablished, slog.String("student_id", sui.ID))
}
