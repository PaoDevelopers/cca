package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	dp42ikTicketVersion = 1
	dp42ikTicketType    = "web1 Ts"
	dp42ikTicketSize    = 233
	dp42ikClockSkew     = 2 * time.Minute
)

type dp42ikTicket struct {
	ServiceID string
	UserID    string
	IssuedAt  time.Time
	ExpiresAt time.Time
	TicketID  string
	AuthCtx   string
}

func (app *App) handleDP42IK(w http.ResponseWriter, r *http.Request) {
	app.logRequestStart(r, "handleDP42IK")
	if r.Method != http.MethodPost {
		app.respondHTTPError(r, w, http.StatusMethodNotAllowed, "Method Not Allowed", nil)
		return
	}

	if len(app.dp42ikKey) != chacha20poly1305.KeySize || app.config.DP42IK.ServiceID == "" {
		app.respondHTTPError(r, w, http.StatusServiceUnavailable, "Service Unavailable\nDP42IK is not configured", nil)
		return
	}

	if err := r.ParseForm(); err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nMalformed form", err)
		return
	}

	rawTicket := r.PostFormValue("ticket")
	if rawTicket == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nTicket expected but not found", nil)
		return
	}

	ticket, err := consumeDP42IKTicket(
		rawTicket,
		app.config.DP42IK.ServiceID,
		byte(app.config.DP42IK.KeyID),
		app.dp42ikKey,
		time.Now(),
	)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusUnauthorized, "Unauthorized\nInvalid DP42IK ticket", err)
		return
	}

	sid, err := studentIDFromDP42IKUserID(ticket.UserID)
	if err != nil {
		app.respondHTTPError(
			r,
			w,
			http.StatusUnauthorized,
			"Unauthorized\nInvalid student ID",
			err,
			slog.String("dp42ik_user_id", ticket.UserID),
			slog.String("ticket_id", ticket.TicketID),
		)
		return
	}

	if err := app.issueStudentSession(r.Context(), w, sid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			app.respondHTTPError(
				r,
				w,
				http.StatusUnauthorized,
				"Unauthorized\nStudent ID not found in database",
				err,
				slog.Int64("student_id", sid),
				slog.String("ticket_id", ticket.TicketID),
			)
			return
		}
		app.respondHTTPError(
			r,
			w,
			http.StatusInternalServerError,
			"Internal Server Error\nCannot set student session token",
			err,
			slog.Int64("student_id", sid),
			slog.String("ticket_id", ticket.TicketID),
		)
		return
	}

	app.logInfo(
		r,
		logMsgAuthStudentDP42IKLogin,
		slog.Int64("student_id", sid),
		slog.String("dp42ik_user_id", ticket.UserID),
		slog.String("ticket_id", ticket.TicketID),
		slog.String("auth_context", ticket.AuthCtx),
	)
	http.Redirect(w, r, "/student/", http.StatusSeeOther)
}

func consumeDP42IKTicket(rawTicket, serviceID string, keyID byte, key []byte, now time.Time) (*dp42ikTicket, error) {
	wire, err := base64.StdEncoding.DecodeString(rawTicket)
	if err != nil {
		return nil, fmt.Errorf("decode ticket: %w", err)
	}

	if len(wire) < 1+chacha20poly1305.NonceSizeX+chacha20poly1305.Overhead {
		return nil, fmt.Errorf("ticket too short: %d", len(wire))
	}
	if wire[0] != keyID {
		return nil, fmt.Errorf("unexpected key id: got %d, want %d", wire[0], keyID)
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("create aead: %w", err)
	}

	nonce := wire[1 : 1+chacha20poly1305.NonceSizeX]
	ciphertext := wire[1+chacha20poly1305.NonceSizeX:]
	aad := append([]byte("web1"), []byte(serviceID)...)
	plain, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("open ticket: %w", err)
	}

	ticket, err := parseDP42IKTicketPlaintext(plain)
	if err != nil {
		return nil, err
	}
	if ticket.ServiceID != serviceID {
		return nil, fmt.Errorf("unexpected service id: got %q, want %q", ticket.ServiceID, serviceID)
	}
	if now.Add(dp42ikClockSkew).Before(ticket.IssuedAt) {
		return nil, fmt.Errorf("ticket issued in the future: %s", ticket.IssuedAt.Format(time.RFC3339))
	}
	if now.After(ticket.ExpiresAt.Add(dp42ikClockSkew)) {
		return nil, fmt.Errorf("ticket expired: %s", ticket.ExpiresAt.Format(time.RFC3339))
	}

	return ticket, nil
}

func parseDP42IKTicketPlaintext(plain []byte) (*dp42ikTicket, error) {
	if len(plain) != dp42ikTicketSize {
		return nil, fmt.Errorf("unexpected plaintext size: got %d, want %d", len(plain), dp42ikTicketSize)
	}
	if plain[0] != dp42ikTicketVersion {
		return nil, fmt.Errorf("unexpected ticket version: %d", plain[0])
	}
	if string(bytes.TrimRight(plain[1:9], "\x00")) != dp42ikTicketType {
		return nil, fmt.Errorf("unexpected ticket type")
	}

	issuedAt, err := unixSecondsToTime(binary.BigEndian.Uint64(plain[137:145]))
	if err != nil {
		return nil, fmt.Errorf("issued_at: %w", err)
	}
	expiresAt, err := unixSecondsToTime(binary.BigEndian.Uint64(plain[145:153]))
	if err != nil {
		return nil, fmt.Errorf("expires_at: %w", err)
	}

	return &dp42ikTicket{
		ServiceID: dp42ikFieldString(plain[9:73]),
		UserID:    dp42ikFieldString(plain[73:137]),
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
		TicketID:  hex.EncodeToString(plain[153:169]),
		AuthCtx:   dp42ikFieldString(plain[169:233]),
	}, nil
}

func dp42ikFieldString(field []byte) string {
	return string(bytes.TrimRight(field, "\x00"))
}

func unixSecondsToTime(secs uint64) (time.Time, error) {
	if secs > uint64(1<<63-1) {
		return time.Time{}, fmt.Errorf("value too large: %d", secs)
	}
	return time.Unix(int64(secs), 0), nil
}

func studentIDFromDP42IKUserID(userID string) (int64, error) {
	userID = strings.ToLower(strings.TrimSpace(userID))
	if userID == "" {
		return 0, errors.New("empty user id")
	}

	sid, err := strconv.ParseInt(strings.TrimLeft(userID, "s"), 10, 64)
	if err != nil {
		return 0, err
	}
	return sid, nil
}
