package main

import (
	"fmt"
	"log/slog"
	"net/http"
)

func (app *App) handleStuAPIEvents(w http.ResponseWriter, r *http.Request, _ *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPIEvents")
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		app.respondHTTPError(
			r,
			w,
			http.StatusInternalServerError,
			"Internal Server Error\nConnection does not seem to support streaming for some reason?",
			fmt.Errorf("response writer does not implement http.Flusher"),
		)
		return
	}

	ch := app.broker.Subscribe()
	app.logInfo(r, "subscribed to sse broker")
	defer app.broker.Unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			app.logInfo(r, "sse request context canceled")
			return
		case msg := <-ch:
			switch {
			case msg.event == "" && msg.data == "":
				panic("programmer error: unsupported message with both event and data empty")
			case msg.event == "" && msg.data != "":
				if _, err := fmt.Fprintf(w, "data: %s\n\n", msg.data); err != nil {
					app.logError(r, "failed writing sse data", slog.Any("error", err))
					return
				}
				flusher.Flush()
			case msg.event != "" && msg.data == "":
				if _, err := fmt.Fprintf(w, "event: %s\ndata:\n\n", msg.event); err != nil {
					app.logError(r, "failed writing sse event", slog.Any("error", err))
					return
				}
				flusher.Flush()
			case msg.event != "" && msg.data != "":
				if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.event, msg.data); err != nil {
					app.logError(r, "failed writing sse event and data", slog.Any("error", err))
					return
				}
				flusher.Flush()
			}
		}
	}
}
