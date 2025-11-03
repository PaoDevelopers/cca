package main

import (
	"context"
	"log"
	"log/slog"
	"strings"
)

const tlsHandshakePrefix = "http: TLS handshake error from "

type httpServerLogWriter struct {
	logger *slog.Logger
}

func newHTTPServerErrorLogger() *log.Logger {
	return log.New(&httpServerLogWriter{logger: slog.Default()}, "", 0)
}

func (w *httpServerLogWriter) Write(p []byte) (int, error) {
	if w.logger == nil {
		return len(p), nil
	}

	msg := strings.TrimSpace(string(p))
	switch {
	case strings.HasPrefix(msg, tlsHandshakePrefix):
		w.logTLSHandshakeError(msg)
	default:
		w.logger.LogAttrs(context.Background(), slog.LevelError, logMsgHTTPServerError, slog.String("detail", msg))
	}
	return len(p), nil
}

func (w *httpServerLogWriter) logTLSHandshakeError(msg string) {
	rest := strings.TrimPrefix(msg, tlsHandshakePrefix)
	remoteAddr := rest
	errorText := ""
	if sep := strings.Index(rest, ": "); sep != -1 {
		remoteAddr = strings.TrimSpace(rest[:sep])
		errorText = strings.TrimSpace(rest[sep+2:])
	}

	attrs := []slog.Attr{
		slog.String("remote_addr", remoteAddr),
	}
	if errorText != "" {
		attrs = append(attrs, slog.String("error", errorText))
	}

	w.logger.LogAttrs(context.Background(), slog.LevelError, logMsgHTTPServerTLSHandshake, attrs...)
}
