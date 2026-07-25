package httpapi

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

func TestConsumeDP42IKTicket(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	now := time.Unix(1_700_000_000, 0)
	rawTicket := makeDP42IKTestTicket(t, "paospace", "s12345", "OIDC", 0, key, now, now.Add(time.Minute))

	ticket, err := consumeDP42IKTicket(rawTicket, "paospace", 0, key, now)
	if err != nil {
		t.Fatalf("consumeDP42IKTicket returned error: %v", err)
	}
	if ticket.ServiceID != "paospace" {
		t.Fatalf("ServiceID = %q, want %q", ticket.ServiceID, "paospace")
	}
	if ticket.UserID != "s12345" {
		t.Fatalf("UserID = %q, want %q", ticket.UserID, "s12345")
	}
	if ticket.AuthCtx != "OIDC" {
		t.Fatalf("AuthCtx = %q, want %q", ticket.AuthCtx, "OIDC")
	}
}

func TestConsumeDP42IKTicketRejectsWrongServiceID(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	now := time.Unix(1_700_000_000, 0)
	rawTicket := makeDP42IKTestTicket(t, "paospace", "s12345", "OIDC", 0, key, now, now.Add(time.Minute))

	_, err := consumeDP42IKTicket(rawTicket, "cca", 0, key, now)
	if err == nil {
		t.Fatal("consumeDP42IKTicket unexpectedly accepted wrong service ID")
	}
	if !strings.Contains(err.Error(), "open ticket") {
		t.Fatalf("error = %v, want open ticket failure", err)
	}
}

func TestStudentIDFromDP42IKUserID(t *testing.T) {
	got, err := studentIDFromDP42IKUserID(" S12345 ")
	if err != nil {
		t.Fatalf("studentIDFromDP42IKUserID returned error: %v", err)
	}
	if got != 12345 {
		t.Fatalf("studentIDFromDP42IKUserID = %d, want 12345", got)
	}
}

func makeDP42IKTestTicket(t *testing.T, serviceID, userID, authCtx string, keyID byte, key []byte, issuedAt, expiresAt time.Time) string {
	t.Helper()

	plain := make([]byte, dp42ikTicketSize)
	plain[0] = dp42ikTicketVersion
	copy(plain[1:9], dp42ikTicketType)
	copy(plain[9:73], serviceID)
	copy(plain[73:137], userID)
	// #nosec G115 -- test timestamps are deliberately positive Unix values.
	binary.BigEndian.PutUint64(plain[137:145], uint64(issuedAt.Unix()))
	// #nosec G115 -- test timestamps are deliberately positive Unix values.
	binary.BigEndian.PutUint64(plain[145:153], uint64(expiresAt.Unix()))
	copy(plain[153:169], "ticket-id-000000")
	copy(plain[169:233], authCtx)

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		t.Fatalf("NewX returned error: %v", err)
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	aad := append([]byte("web1"), []byte(serviceID)...)
	ciphertext := aead.Seal(nil, nonce, plain, aad)

	wire := make([]byte, 1+len(nonce)+len(ciphertext))
	wire[0] = keyID
	copy(wire[1:], nonce)
	copy(wire[1+len(nonce):], ciphertext)

	return base64.StdEncoding.EncodeToString(wire)
}
