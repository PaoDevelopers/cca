package main

import (
	"fmt"
	"net/http"
)

func (app *App) handleStuAPIEvents(w http.ResponseWriter, r *http.Request, _ *UserInfoStudent) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, nil)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Internal Server Error\nConnection does not seem to support streaming for some reason?", http.StatusInternalServerError)
		return
	}

	ch := app.broker.Subscribe()
	defer app.broker.Unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			switch {
			case msg.event == "" && msg.data == "":
				panic("programmer error: unsupported message with both event and data empty")
			case msg.event == "" && msg.data != "":
				fmt.Fprintf(w, "data: %s\n\n", msg.data)
				flusher.Flush()
			case msg.event != "" && msg.data == "":
				fmt.Fprintf(w, "event: %s\ndata:\n\n", msg.event)
				flusher.Flush()
			case msg.event != "" && msg.data != "":
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.event, msg.data)
				flusher.Flush()
			}
		}
	}
}
