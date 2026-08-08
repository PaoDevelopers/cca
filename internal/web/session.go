package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Sessions are stateless: a signed cookie naming the subject and an
// expiry, and nothing in the database. There is no session table to
// grow, no row to expire, and no lookup on the authenticated path.
//
// The trade is the usual one. A stateless session cannot be revoked
// individually before it expires; the lever is rotating the signing
// key, which invalidates every session at once. For a school running
// one selection season with a 72-hour lifetime that is the right side
// of the trade, and it is why the lifetime is short.
//
// The cookie is:
//
//	v1:<role>:<subject>:<expiry>:<mac>
//
// with the MAC taken over everything before it, delimiters included,
// so no field boundary can be moved without changing the signed text.
//
// The delimiter is ':' for the same reason the violation codes use it:
// it is the one character the identifier grammars exclude outright, so
// no subject can contain one and no two distinct sessions can share a
// signed string. A '.' would not do — localparts hold dots, which is
// how 'runxi.yu' is spelled. encodeSession refuses a subject holding a
// delimiter rather than trusting the grammar from here.

const (
	sessionVersion   = "v1"
	sessionDelimiter = ":"
)

var (
	// errBadSession covers every way a cookie can fail to name a live
	// session: malformed, wrong signature, wrong role, or expired.
	// The caller turns it into errNoSession; the distinctions matter
	// for logs, never for the answer.
	errBadSession = errors.New("malformed or unsigned session")

	errSessionExpired = errors.New("session expired")

	errSessionKeyTooShort = errors.New("session key must be at least 32 bytes")
	errSessionFieldSyntax = errors.New("session field is empty or holds the delimiter")
)

// sessionKey is the HMAC key. It comes from the configuration file and
// is never derived or generated: a key invented at startup would sign
// every user out on every restart, silently and unpredictably.
type sessionKey []byte

// newSessionKey decodes and checks configured key material.
func newSessionKey(hexKey string) (sessionKey, error) {
	raw, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("session key is not hex: %w", err)
	}

	// 32 bytes is the block size of the hash; less than that is a
	// weaker key than the primitive assumes.
	if len(raw) < 32 {
		return nil, fmt.Errorf("%w: got %d", errSessionKeyTooShort, len(raw))
	}

	return sessionKey(raw), nil
}

func (k sessionKey) sign(signed string) string {
	mac := hmac.New(sha256.New, k)
	// hash.Hash never returns an error from Write.
	_, _ = mac.Write([]byte(signed))

	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// encodeSession mints a cookie value for a subject in a role.
func (k sessionKey) encodeSession(role string, subject string, expiry time.Time) (string, error) {
	// The delimiter must not appear inside a field, or two distinct
	// sessions could share a signed string.
	if subject == "" ||
		strings.Contains(role, sessionDelimiter) ||
		strings.Contains(subject, sessionDelimiter) {
		return "", fmt.Errorf("%w: role %q subject %q", errSessionFieldSyntax, role, subject)
	}

	signed := strings.Join([]string{
		sessionVersion, role, subject,
		strconv.FormatInt(expiry.Unix(), 10),
	}, sessionDelimiter)

	return signed + sessionDelimiter + k.sign(signed), nil
}

// decodeSession returns the subject a cookie names, once its signature
// and expiry check out for this role. A cookie signed for the other
// role is rejected here rather than anywhere later: the two areas hold
// independent sessions, and an administrator's cookie must not name a
// student.
func (k sessionKey) decodeSession(role string, value string, now time.Time) (string, error) {
	// Five fields exactly: no component of a well-formed cookie may
	// contain the delimiter, so a value that splits into any other
	// number was not minted here.
	parts := strings.Split(value, sessionDelimiter)
	if len(parts) != 5 {
		return "", errBadSession
	}

	signed := strings.Join(parts[:4], sessionDelimiter)

	// Constant time, and before anything is parsed or trusted.
	if !hmac.Equal([]byte(parts[4]), []byte(k.sign(signed))) {
		return "", errBadSession
	}

	if parts[0] != sessionVersion || parts[1] != role || parts[2] == "" {
		return "", errBadSession
	}

	expiry, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return "", errBadSession
	}

	if !now.Before(time.Unix(expiry, 0)) {
		return "", errSessionExpired
	}

	return parts[2], nil
}
