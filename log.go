package main

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

func attrsToArgs(attrs []slog.Attr) []any {
	args := make([]any, len(attrs))
	for i := range attrs {
		args[i] = attrs[i]
	}
	return args
}

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

func (app *App) requestLogger(r *http.Request, extra ...slog.Attr) *slog.Logger {
	attrs := append(requestAttrs(r), extra...)
	return slog.Default().With(attrsToArgs(attrs)...)
}

func (app *App) logRequestStart(r *http.Request, handler string, extra ...slog.Attr) {
	app.requestLogger(r, append(extra, slog.String("handler", handler))...).Info(logMsgHTTPRequestStart)
}

func (app *App) logInfo(r *http.Request, msg string, extra ...slog.Attr) {
	app.requestLogger(r, extra...).Info(msg)
}

func (app *App) logWarn(r *http.Request, msg string, extra ...slog.Attr) {
	app.requestLogger(r, extra...).Warn(msg)
}

func (app *App) logError(r *http.Request, msg string, extra ...slog.Attr) {
	app.requestLogger(r, extra...).Error(msg)
}

func (app *App) respondHTTPError(r *http.Request, w http.ResponseWriter, status int, message string, err error, extra ...slog.Attr) {
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

func (app *App) apiError(r *http.Request, w http.ResponseWriter, code int, v any, extra ...slog.Attr) {
	apiHeaders(w)
	attrs := []slog.Attr{
		slog.Int("status", code),
		slog.Any("payload", v),
	}
	if len(extra) > 0 {
		attrs = append(attrs, extra...)
	}
	if code >= http.StatusInternalServerError {
		app.logError(r, logMsgAPIResponseError, attrs...)
	} else {
		app.logWarn(r, logMsgAPIResponseError, attrs...)
	}
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		app.logError(r, logMsgAPIResponseEncodeError, slog.Any("error", err))
	}
}

func (app *App) writeJSON(r *http.Request, w http.ResponseWriter, status int, payload any, extra ...slog.Attr) {
	apiHeaders(w)
	if status == 0 {
		status = http.StatusOK
	}
	app.logInfo(r, logMsgHTTPResponseJSON, append(extra, slog.Int("status", status))...)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		app.logError(r, logMsgHTTPResponseEncodeError, slog.Any("error", err))
	}
}
