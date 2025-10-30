package main

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (app *App) handleStuAPIEvents(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPIEvents", slog.Int64("student_id", sui.ID))
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
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

	go client.writePump()
	go client.readPump()

	app.logInfo(r, "websocket connection established", slog.Int64("student_id", sui.ID))
}
