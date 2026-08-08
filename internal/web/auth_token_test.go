package web //nolint:testpackage

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// An id_token is only evidence if it was issued to this application,
// for this sign-in. The JWKS that signs it signs for a whole tenant,
// so a valid signature says almost nothing on its own: every other
// application registered beside this one produces tokens it verifies.
//
// Two claims close that. The audience says the token was minted for
// us. The nonce says it was minted for the sign-in now in progress,
// and not replayed from another one.

type testIDP struct {
	key *rsa.PrivateKey
}

func newTestIDP(t *testing.T) *testIDP {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	return &testIDP{key: key}
}

// mint issues a token the signature check will accept, with whatever
// audience and nonce the test wants to try.
func (idp *testIDP) mint(t *testing.T, audience, nonce string) string {
	t.Helper()

	// Assigned rather than built as a literal: the embedded
	// RegisteredClaims cannot be named alongside the outer fields
	// without one linter or another objecting, and this reads as
	// what it is.
	var claims Claims

	claims.Audience = jwt.ClaimStrings{audience}
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
	claims.IssuedAt = jwt.NewNumericDate(time.Now())
	claims.Name = "Test Person"
	claims.Email = "s1@stu.ykpaoschool.cn"
	claims.Nonce = nonce

	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, &claims).SignedString(idp.key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	return signed
}

// serverFor builds a server whose key function accepts this IdP.
func (idp *testIDP) serverFor(t *testing.T) *Server {
	t.Helper()

	app := testServer(nil)
	app.sessionKey = testSessionKey(t)
	app.config.OIDC.Client = "our-client-id"
	app.verifyKey = func(*jwt.Token) (any, error) {
		return &idp.key.PublicKey, nil
	}

	return app
}

func postToken(t *testing.T, app *Server, token, nonce string) *httptest.ResponseRecorder {
	t.Helper()

	form := url.Values{"id_token": {token}}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth",
		strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	app.parseIDToken(rec, r, nonce)

	return rec
}

func TestAnIDTokenMustBeAddressedToThisApplication(t *testing.T) {
	t.Parallel()

	idp := newTestIDP(t)
	app := idp.serverFor(t)

	// The same signer, a different application in the same tenant.
	foreign := idp.mint(t, "another-application", "n1")
	if rec := postToken(t, app, foreign, "n1"); rec.Code == http.StatusOK {
		t.Error("a token minted for another application was accepted")
	}

	ours := idp.mint(t, "our-client-id", "n1")
	if rec := postToken(t, app, ours, "n1"); rec.Code != http.StatusOK {
		t.Errorf("our own token was rejected: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAnIDTokenMustCarryThisSignInsNonce(t *testing.T) {
	t.Parallel()

	idp := newTestIDP(t)
	app := idp.serverFor(t)

	// A token from some earlier or parallel sign-in, replayed.
	replayed := idp.mint(t, "our-client-id", "some-other-nonce")
	if rec := postToken(t, app, replayed, "this-sign-in"); rec.Code == http.StatusOK {
		t.Error("a token from a different sign-in was accepted")
	}

	// And a token with no nonce at all.
	none := idp.mint(t, "our-client-id", "")
	if rec := postToken(t, app, none, "this-sign-in"); rec.Code == http.StatusOK {
		t.Error("a token with no nonce was accepted")
	}

	matching := idp.mint(t, "our-client-id", "this-sign-in")
	if rec := postToken(t, app, matching, "this-sign-in"); rec.Code != http.StatusOK {
		t.Errorf("the matching token was rejected: %d %s", rec.Code, rec.Body.String())
	}
}

// The authorize URL must send the same nonce it stores, or the check
// above can never pass and sign-in breaks entirely.
func TestTheAuthorizeURLSendsTheNonceItWillCheck(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	app.config.OIDC.Authorize = "https://login.example.com/authorize"
	app.config.OIDC.Client = "our-client-id"

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://cca.example.org/student/", nil)

	target, err := app.authorizeURL(r, roleStudent, "the-nonce")
	if err != nil {
		t.Fatalf("authorizeURL: %v", err)
	}

	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if got := parsed.Query().Get("nonce"); got != "the-nonce" {
		t.Errorf("nonce = %q, want the-nonce", got)
	}

	if got := parsed.Query().Get("state"); got != "student:the-nonce" {
		t.Errorf("state = %q, want student:the-nonce", got)
	}
}
