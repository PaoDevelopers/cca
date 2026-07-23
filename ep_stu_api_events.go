package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
)

var upgraderOpts = &websocket.AcceptOptions{}

func (app *App) handleStuAPIEvents(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil)
		return
	}

	conn, err := websocket.Accept(w, r, upgraderOpts)
	if err != nil {
		app.logError(r, logMsgStudentEventsUpgradeError, slog.Any("error", err))
		return
	}

	client := &Client{
		conn:      conn,
		send:      make(chan WSMessage, 256),
		hub:       app.wsHub,
		studentID: sui.ID,
	}

	app.wsHub.register <- client

	ctx, cancel := context.WithTimeout(r.Context(), websocketWriteTimeout)
	err = conn.Write(ctx, websocket.MessageText, []byte("hello"))
	cancel()
	if err != nil {
		app.logError(r, logMsgStudentEventsHelloError, slog.Any("error", err))
		client.unregister()
		conn.CloseNow()
		return
	}

	go client.writePump()
	go client.readPump()

	app.logDebug(r, logMsgStudentEventsEstablished, slog.Int64("student_id", sui.ID))
}
