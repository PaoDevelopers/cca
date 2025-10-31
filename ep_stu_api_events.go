package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
)

var upgraderOpts = &websocket.AcceptOptions{}

func (app *App) handleStuAPIEvents(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPIEvents", slog.Int64("student_id", sui.ID))
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil)
		return
	}

	conn, err := websocket.Accept(w, r, upgraderOpts)
	if err != nil {
		app.logError(r, "failed to upgrade to websocket", slog.Any("error", err))
		return
	}

	client := &Client{
		conn:      conn,
		send:      make(chan WSMessage, 256),
		hub:       app.wsHub,
		studentID: sui.ID,
	}

	app.wsHub.register <- client

	if err := conn.Write(context.Background(), websocket.MessageText, []byte("hello")); err != nil {
		app.logError(r, "failed to send websocket hello", slog.Any("error", err))
		app.wsHub.unregister <- client
		_ = conn.Close(websocket.StatusInternalError, "")
		return
	}

	go client.writePump()
	go client.readPump()

	app.logInfo(r, "websocket connection established", slog.Int64("student_id", sui.ID))
}
