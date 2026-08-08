package web

import (
	"encoding/json/v2"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

func requestAttrs(r *http.Request) []slog.Attr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	attrs := []slog.Attr{
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
	}

	if r.URL.RawQuery != "" {
		attrs = append(attrs, slog.String("query", r.URL.RawQuery))
	}

	if host != "" {
		attrs = append(attrs, slog.String("remote_addr", host))
	}

	if ua := strings.TrimSpace(r.UserAgent()); ua != "" {
		attrs = append(attrs, slog.String("user_agent", ua))
	}

	return attrs
}

// The attributes are already slog.Attr, so they go to the handler as
// attributes rather than being widened to []any and re-parsed as
// alternating keys and values by Logger.With. Doing it the other way
// meant a slice allocation and a type switch per attribute per request
// line, to arrive at the values it started with.
func (app *Server) requestLogger(r *http.Request, extra ...slog.Attr) *slog.Logger {
	attrs := append(requestAttrs(r), extra...)

	return slog.New(slog.Default().Handler().WithAttrs(attrs))
}

func (app *Server) logRequestStart(r *http.Request, handler string, extra ...slog.Attr) {
	app.requestLogger(r, append(extra, slog.String("handler", handler))...).Info(logMsgHTTPRequestStart)
}

func (app *Server) logInfo(r *http.Request, msg string, extra ...slog.Attr) {
	app.requestLogger(r, extra...).Info(msg)
}

func (app *Server) logWarn(r *http.Request, msg string, extra ...slog.Attr) {
	app.requestLogger(r, extra...).Warn(msg)
}

func (app *Server) logError(r *http.Request, msg string, extra ...slog.Attr) {
	app.requestLogger(r, extra...).Error(msg)
}

func (app *Server) respondHTTPError(r *http.Request, w http.ResponseWriter, status int, message string, err error, extra ...slog.Attr) {
	attrs := []slog.Attr{
		slog.Int("status", status),
	}
	if message != "" {
		attrs = append(attrs, slog.String("response", message))
	}

	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}

	if len(extra) > 0 {
		attrs = append(attrs, extra...)
	}

	if status >= http.StatusInternalServerError {
		app.logError(r, logMsgHTTPResponseError, attrs...)
	} else {
		app.logWarn(r, logMsgHTTPResponseError, attrs...)
	}

	http.Error(w, message, status)
}

func apiHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Content-Type", "application/json")
}

// Reads answer 200 through here — and so do the three student writes,
// which answer with the resulting enrollment set rather than a bare
// 204, so a client never has to guess what its change did. Every
// administrator write does return 204.
func (app *Server) writeJSON(r *http.Request, w http.ResponseWriter, payload any, extra ...slog.Attr) {
	apiHeaders(w)
	app.logInfo(r, logMsgHTTPResponseJSON, append(extra, slog.Int("status", http.StatusOK))...)
	w.WriteHeader(http.StatusOK)

	if err := json.MarshalWrite(w, payload); err != nil {
		app.logError(r, logMsgHTTPResponseEncodeError, slog.Any("error", err))
	}
}
