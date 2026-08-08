package web //nolint:testpackage

import (
	"strings"
	"testing"
	"time"
)

// testSessionKey is a fixed key for tests. Fixed rather than random so
// a failure reproduces, and distinct from the all-'a' key some tests
// use as "another key".
func testSessionKey(t *testing.T) sessionKey {
	t.Helper()

	key, err := newSessionKey(strings.Repeat("5c", 32))
	if err != nil {
		t.Fatalf("newSessionKey: %v", err)
	}

	return key
}

// A weak or unset key must stop the server starting rather than
// silently protecting nothing.
func TestSessionKeyRefusesWeakMaterial(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		key  string
	}{
		{"empty", ""},
		{"the unedited placeholder", "CHANGE-ME-openssl-rand-hex-32"},
		{"not hex", "zzzz"},
		{"odd digit count", strings.Repeat("a", 63)},
		{"31 bytes", strings.Repeat("ab", 31)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := newSessionKey(tt.key); err == nil {
				t.Errorf("newSessionKey(%q) was accepted", tt.key)
			}
		})
	}

	if _, err := newSessionKey(strings.Repeat("ab", 32)); err != nil {
		t.Errorf("a 32-byte key was rejected: %v", err)
	}
}

func TestSessionRoundTrips(t *testing.T) {
	t.Parallel()

	key := testSessionKey(t)
	now := time.Now()

	for _, subject := range []string{"s22537", "runxi.yu", "ed.chapman"} {
		token, err := key.encodeSession(roleStudent, subject, now.Add(time.Hour))
		if err != nil {
			t.Fatalf("encode %q: %v", subject, err)
		}

		got, err := key.decodeSession(roleStudent, token, now)
		if err != nil {
			t.Fatalf("decode %q: %v", subject, err)
		}

		if got != subject {
			t.Errorf("round trip gave %q, want %q", got, subject)
		}
	}
}

// The expiry is checked here, not only advertised to the browser: a
// cookie kept past its lifetime by a client that ignores Expires must
// still stop working.
func TestSessionExpiryIsEnforcedServerSide(t *testing.T) {
	t.Parallel()

	key := testSessionKey(t)
	now := time.Now()

	token, err := key.encodeSession(roleStudent, "s1", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	if _, err := key.decodeSession(roleStudent, token, now.Add(59*time.Second)); err != nil {
		t.Errorf("a live session was refused: %v", err)
	}

	if _, err := key.decodeSession(roleStudent, token, now.Add(time.Minute)); err == nil {
		t.Error("a session was accepted exactly at its expiry")
	}

	if _, err := key.decodeSession(roleStudent, token, now.Add(time.Hour)); err == nil {
		t.Error("a long-expired session was accepted")
	}
}

// Every field is under the signature, so none of them can be moved,
// swapped or rewritten without invalidating the whole cookie.
func TestSessionRejectsTampering(t *testing.T) {
	t.Parallel()

	key := testSessionKey(t)
	now := time.Now()

	token, err := key.encodeSession(roleStudent, "s1", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	parts := strings.Split(token, sessionDelimiter)
	if len(parts) != 5 {
		t.Fatalf("token has %d fields, want 5: %q", len(parts), token)
	}

	for _, tt := range []struct {
		name  string
		value string
	}{
		{"a different subject", strings.Replace(token, "s1", "s2", 1)},
		{"a later expiry", strings.Join([]string{
			parts[0], parts[1], parts[2],
			"99999999999", parts[4],
		}, sessionDelimiter)},
		{"the other role", strings.Join([]string{
			parts[0], roleAdmin, parts[2], parts[3], parts[4],
		}, sessionDelimiter)},
		{"a different version", strings.Join([]string{
			"v2", parts[1], parts[2], parts[3], parts[4],
		}, sessionDelimiter)},
		{"no signature", strings.Join(parts[:4], sessionDelimiter)},
		{"an empty signature", strings.Join(parts[:4], sessionDelimiter) + sessionDelimiter},
		{"an extra field", token + sessionDelimiter + "extra"},
		{"nothing at all", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := key.decodeSession(roleStudent, tt.value, now); err == nil {
				t.Errorf("a tampered cookie was accepted: %q", tt.value)
			}
		})
	}
}

// The two areas hold independent sessions, so a cookie minted for one
// must never authenticate the other — the administrator area is the
// one that matters, and it is reached by the same browser.
func TestSessionsDoNotCrossRoles(t *testing.T) {
	t.Parallel()

	key := testSessionKey(t)
	now := time.Now()

	token, err := key.encodeSession(roleAdmin, "ed.chapman", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	if _, err := key.decodeSession(roleStudent, token, now); err == nil {
		t.Error("an administrator cookie authenticated a student")
	}

	if _, err := key.decodeSession(roleAdmin, token, now); err != nil {
		t.Errorf("an administrator cookie failed in its own area: %v", err)
	}
}

// A subject holding the delimiter would let two distinct sessions
// share one signed string. The grammars exclude ':', but encodeSession
// refuses rather than trusting that from here.
func TestSessionRefusesDelimitersInFields(t *testing.T) {
	t.Parallel()

	key := testSessionKey(t)

	for _, subject := range []string{"", "a:b", "s1:s2:s3"} {
		if _, err := key.encodeSession(roleStudent, subject, time.Now().Add(time.Hour)); err == nil {
			t.Errorf("encodeSession accepted subject %q", subject)
		}
	}
}

// Dotted localparts are ordinary: a teacher or administrator enrolled
// for testing is 'runxi.yu', and a delimiter that split on '.' would
// have made their session unrepresentable.
func TestSessionAcceptsDottedLocalparts(t *testing.T) {
	t.Parallel()

	key := testSessionKey(t)
	now := time.Now()

	token, err := key.encodeSession(roleStudent, "runxi.yu", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	got, err := key.decodeSession(roleStudent, token, now)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got != "runxi.yu" {
		t.Errorf("subject = %q, want runxi.yu", got)
	}
}
