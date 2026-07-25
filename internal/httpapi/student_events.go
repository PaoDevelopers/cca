package httpapi

import (
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

	if err := app.wsHub.Connect(conn, sui.ID); err != nil {
		app.logError(r, logMsgStudentEventsHelloError, slog.Any("error", err))
		return
	}

	app.logDebug(r, logMsgStudentEventsEstablished, slog.Int64("student_id", sui.ID))
}
