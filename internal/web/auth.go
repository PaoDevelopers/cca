package web

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
)

// The two areas hold independent sessions in separate cookies, so
// signing in to one never signs you out of the other. Each area serves
// its SPA when its cookie is valid and a sign-in page otherwise; that
// page links to the OIDC authorize endpoint with state
// "<role>:<nonce>", which the fixed /auth callback dispatches on.
// API endpoints never redirect; they return 401.

// Claims are the OIDC ID token claims we consume.
//
// Nonce is consumed rather than merely carried: it is what binds a
// token to the sign-in that asked for it. Without checking it, any
// id_token the identity provider ever issued for a school account —
// including one minted for a different application in the same tenant,
// which the same JWKS signs — would be accepted here by whoever
// obtained it.
type Claims struct {
	jwt.RegisteredClaims

	Name  string `json:"name"`
	Email string `json:"email"`
	Nonce string `json:"nonce"`
}

const (
	roleStudent = "student"
	roleAdmin   = "admin"

	studentCookie = "student_session"
	adminCookie   = "admin_session"
	nonceCookie   = "oidc_nonce"
	flashCookie   = "signin_error"

	sessionLifetime = 72 * time.Hour

	// An OIDC form_post carries an id_token and a state. Entra's
	// tokens run to a few kilobytes; this is generous and still
	// nothing like the 10 MB ParseForm would otherwise accept.
	maxAuthCallback = 256 << 10
)

// Redirect targets stay constant-derived, never taken from the request.
func rolePath(role string) string {
	if role == roleAdmin {
		return "/admin/"
	}

	return "/student/"
}

func sessionCookieName(role string) string {
	if role == roleAdmin {
		return adminCookie
	}

	return studentCookie
}

// cookieMaxAge expresses a lifetime the way a browser can evaluate it
// without agreeing with us about the time.
//
// Expires is an instant, and RFC 6265 has the user agent compare it
// against its own clock — not against the response's Date header. So a
// device whose clock runs fast discards our cookies on arrival. Ten
// minutes fast is enough to lose the OIDC nonce, and losing the nonce
// means every sign-in attempt ends on "Sign-in session expired; please
// try again", forever, with retrying unable to help. A school laptop
// with a flat CMOS battery is routinely that far out.
//
// Max-Age is a duration, so it is immune: the browser counts down from
// whenever it receives the response. It also takes precedence over
// Expires where both are given, so there is nothing to be gained by
// sending both.
func cookieMaxAge(lifetime time.Duration) int {
	// A zero Max-Age is "unset" to net/http, which would silently turn
	// a bounded cookie into a session cookie.
	return max(1, int(lifetime.Seconds()))
}

func setCookie(w http.ResponseWriter, name string, value string, lifetime time.Duration) {
	//exhaustruct:ignore
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
		Secure:   true,
		MaxAge:   cookieMaxAge(lifetime),
	})
}

func clearCookie(w http.ResponseWriter, name string) {
	// MaxAge -1 is what deletes the cookie.
	//exhaustruct:ignore
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
		Secure:   true,
		MaxAge:   -1,
	})
}

// nonce is used twice on purpose: as the OIDC nonce, which comes back
// inside the signed token, and as the state, which comes back beside
// it. One value, checked in both places, ties the token and the
// redirect to the same sign-in — the token by its signature, the
// redirect by the cookie.
func (app *Server) authorizeURL(r *http.Request, role string, nonce string) (string, error) {
	u, err := url.Parse(app.config.OIDC.Authorize)
	if err != nil {
		return "", fmt.Errorf("parse OIDC authorize endpoint: %w", err)
	}

	q := u.Query()
	q.Set("client_id", app.config.OIDC.Client)
	q.Set("response_type", "id_token code")
	q.Set("redirect_uri", requestAbsoluteURL(r, "/auth"))
	q.Set("response_mode", "form_post")
	q.Set("scope", "openid profile email User.Read")
	q.Set("nonce", nonce)
	q.Set("state", role+":"+nonce)

	u.RawQuery = q.Encode()

	return u.String(), nil
}

// Sign-in failures travel as codes in a one-shot flash cookie. Codes
// rather than text, so a forged cookie can at worst select one of our
// own messages and needs no integrity protection.
//
//nolint:gochecknoglobals
var signinErrors = map[string]string{
	"external":    "Sign-in failed at the identity provider",
	"expired":     "Sign-in session expired; please try again",
	"bad_email":   "Your account has no usable email address",
	"domain":      "Please sign in with a school account",
	"not_admin":   "Your account is not an administrator",
	"not_student": "Your account is not a student account",
	"no_student":  "Your account is not in the student database",
}

func (app *Server) serveSignin(w http.ResponseWriter, r *http.Request, role string, errMsg string) {
	nonce := rand.Text()
	// SameSite=None because the nonce must survive the cross-site
	// form_post from the identity provider back to /auth, which
	// SameSite=Lax cookies do not (Lax only covers cross-site GETs).
	//exhaustruct:ignore
	http.SetCookie(w, &http.Cookie{ //#nosec:G124
		Name:     nonceCookie,
		Value:    nonce,
		Path:     "/",
		SameSite: http.SameSiteNoneMode,
		HttpOnly: true,
		Secure:   true,
		MaxAge:   cookieMaxAge(10 * time.Minute),
	})

	target, err := app.authorizeURL(r, role, nonce)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nCannot build OIDC authorize URL", err)

		return
	}

	app.renderPage(w, r, signinTemplate, signinData{
		Role:  role,
		URL:   target,
		Error: errMsg,
	})
}

func signinRedirect(w http.ResponseWriter, r *http.Request, role string, code string) {
	setCookie(w, flashCookie, code, time.Minute)
	http.Redirect(w, r, rolePath(role), http.StatusSeeOther)
}

// takeFlash reads and clears the flash cookie; unknown codes read as
// no error.
func takeFlash(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie(flashCookie)
	if err != nil || cookie.Value == "" {
		return ""
	}

	clearCookie(w, flashCookie)

	return signinErrors[cookie.Value]
}

// handleAuth is the fixed OIDC redirect target for both roles.
func (app *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	app.logRequestStart(r, "handleAuth")

	if r.Method != http.MethodPost {
		app.respondHTTPError(r, w, http.StatusMethodNotAllowed, "Method Not Allowed", nil)

		return
	}

	// The one body-reading handler outside the API surface, and the
	// only one reachable without any session at all. It gets the same
	// bound and the same deadline as the rest: an OIDC callback is a
	// few kilobytes of form, and a client dribbling one out over an
	// hour is not signing in — it is holding a goroutine and a file
	// descriptor for free.
	boundBodyRead(w, bodyReadTimeout)

	r.Body = http.MaxBytesReader(w, r.Body, maxAuthCallback)

	if err := r.ParseForm(); err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nMalformed form", err)

		return
	}

	role, nonce, ok := strings.Cut(r.PostFormValue("state"), ":")
	if !ok || (role != roleStudent && role != roleAdmin) {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nMissing or malformed state", nil)

		return
	}

	if e := r.PostFormValue("error"); e != "" {
		ed := r.PostFormValue("error_description")
		app.logWarn(r, logMsgAuthExternalError, slog.String("external_error", e), slog.String("external_description", ed))
		signinRedirect(w, r, role, "external")

		return
	}

	nc, err := r.Cookie(nonceCookie)
	if err != nil || nc.Value == "" || nc.Value != nonce {
		signinRedirect(w, r, role, "expired")

		return
	}

	clearCookie(w, nonceCookie)

	claims, ok := app.parseIDToken(w, r, nonce)
	if !ok {
		return
	}

	claims.Email = strings.ToLower(claims.Email)

	lp, dp, ok := strings.Cut(claims.Email, "@")
	if !ok {
		signinRedirect(w, r, role, "bad_email")

		return
	}

	if dp != "ykpaoschool.cn" && dp != "stu.ykpaoschool.cn" {
		app.logWarn(r, logMsgAuthWrongDomain, slog.String("email", claims.Email))
		signinRedirect(w, r, role, "domain")

		return
	}

	expiry := time.Now().Add(sessionLifetime)

	switch role {
	case roleAdmin:
		// Matched against the whole address, not the localpart. The
		// two school domains are separate namespaces, and a student
		// account whose localpart happened to equal a member of staff
		// would otherwise be handed an administrator session.
		if !app.isAdmin(claims.Email, lp) {
			app.logWarn(r, logMsgAuthNotAdmin, slog.String("username", lp))
			signinRedirect(w, r, role, "not_admin")

			return
		}

		token, err := app.sessionKey.encodeSession(roleAdmin, lp, expiry)
		if err != nil {
			app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nCannot mint admin session", err, slog.String("admin_username", lp))

			return
		}

		setCookie(w, adminCookie, token, sessionLifetime)
		app.logInfo(r, logMsgAuthAdminLogin, slog.String("admin_username", lp))

	case roleStudent:
		// The email localpart is the student id: no parsing, no
		// prefix stripping, and the same string the roster carries.
		// Whether it names a student is the database's answer, asked
		// here so that signing in tells them at once rather than
		// leaving them at an empty catalogue.
		statusCtx, cancelStatus := readCtx(r.Context())
		defer cancelStatus()

		if _, err := app.queries.GetStudentStatusByID(statusCtx, lp); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				app.logWarn(r, logMsgAuthNotStudent, slog.String("student_id", lp))
				signinRedirect(w, r, role, "no_student")

				return
			}

			app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nCannot look up student", err, slog.String("student_id", lp))

			return
		}

		token, err := app.sessionKey.encodeSession(roleStudent, lp, expiry)
		if err != nil {
			app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nCannot mint student session", err, slog.String("student_id", lp))

			return
		}

		setCookie(w, studentCookie, token, sessionLifetime)
		app.logInfo(r, logMsgAuthStudentLogin, slog.String("student_id", lp))
	}

	http.Redirect(w, r, rolePath(role), http.StatusSeeOther)
}

// parseIDToken validates the posted id_token and returns its claims.
// nonce is the value this sign-in was started with; the token must
// carry the same one.
func (app *Server) parseIDToken(w http.ResponseWriter, r *http.Request, nonce string) (*Claims, bool) {
	idts := r.PostFormValue("id_token")
	if idts == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nID token expected but not found", nil)

		return nil, false
	}

	//exhaustruct:ignore
	idt, err := jwt.ParseWithClaims(idts, &Claims{}, app.verifyKey,
		// The token must have been issued *to us*. The JWKS signs for
		// the whole tenant, so without this any application registered
		// beside ours mints tokens this server would accept.
		jwt.WithAudience(app.config.OIDC.Client),
		// And it must be signed, with a real signature: pinning the
		// algorithms keeps a token that nominates "none", or a
		// symmetric algorithm keyed on the public key, from ever
		// reaching the verifier.
		jwt.WithValidMethods([]string{"RS256", "RS384", "RS512",
			"PS256", "PS384", "PS512", "ES256", "ES384", "ES512"}),
	)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnparsable JWT", err)

		return nil, false
	}

	claims, ok := idt.Claims.(*Claims)
	if !ok || !idt.Valid {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nInvalid JWT", err)

		return nil, false
	}

	// The nonce ties this token to this sign-in. It is compared in
	// constant time because it is a secret of exactly that kind: a
	// value an attacker wants to guess.
	if subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonce)) != 1 {
		app.logWarn(r, logMsgAuthNonceMismatch)
		app.respondHTTPError(r, w, http.StatusBadRequest,
			"Bad Request\nThis sign-in did not match the one that started", nil)

		return nil, false
	}

	return claims, true
}

func (app *Server) handleLogout(role string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		app.logRequestStart(r, "handleLogout", slog.String("role", role))

		// Sessions are stateless, so signing out is exactly deleting
		// the cookie. A copy of the cookie kept elsewhere stays valid
		// until it expires; that is the documented cost of having no
		// session table, and the reason the lifetime is short.
		clearCookie(w, sessionCookieName(role))
		app.logInfo(r, logMsgAuthLogout, slog.String("role", role))
		http.Redirect(w, r, rolePath(role), http.StatusSeeOther)
	}
}
