package web

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/PaoDevelopers/cca/ui"
)

// errNoSession covers every way a request can fail to name a live
// session: no cookie, a forged or tampered one, one signed for the
// other area, one whose expiry has passed, or one naming somebody no
// longer on the administrator allowlist.
//
// They are one error because they are one answer. Verification is pure
// computation over the cookie and the signing key, so unlike a session
// table it cannot fail transiently: there is no case where the right
// response is "try again shortly" rather than "sign in".
var errNoSession = errors.New("no valid session")

// UserInfoStudent is an authenticated student session. It holds the
// student's id and nothing else: everything else about them is in the
// database, changes without the session changing, and is read where it
// is needed.
type UserInfoStudent struct {
	ID string `json:"id"`
}

// UserInfoAdmin is an authenticated administrator session. An
// administrator has no database row at all — the allowlist is in the
// configuration file — so the username is the whole of it.
type UserInfoAdmin struct {
	Username string `json:"username"`
}

func (app *Server) authenticateStudent(r *http.Request) (*UserInfoStudent, error) {
	id, err := app.session(r, roleStudent, studentCookie)
	if err != nil {
		return nil, err
	}

	return &UserInfoStudent{ID: id}, nil
}

// As authenticateStudent, for administrators. The allowlist is
// re-checked on every request rather than trusted from the cookie:
// removing someone from the configuration and restarting must take
// their access away, even though their cookie is still well signed.
func (app *Server) authenticateAdmin(r *http.Request) (*UserInfoAdmin, error) {
	username, err := app.session(r, roleAdmin, adminCookie)
	if err != nil {
		return nil, err
	}

	if !app.isAdmin("", username) {
		return nil, errNoSession
	}

	return &UserInfoAdmin{Username: username}, nil
}

// isAdmin decides whether an identity is on the allowlist.
//
// The configuration may name either a bare localpart or a full
// address. A bare localpart is the historical spelling and is still
// accepted, but it matches across both school domains, which is the
// hazard: an entry written as an address is checked against the
// address and cannot be satisfied by a same-named account in the
// other domain.
//
// email is empty once a session exists, because the cookie carries
// only the subject; at that point the address form can no longer be
// distinguished and the localpart is all there is to check. The
// binding that matters was made at sign-in, where the address was
// still in hand.
func (app *Server) isAdmin(email string, localpart string) bool {
	if email != "" {
		if _, ok := app.config.Admins[email]; ok {
			return true
		}
	}

	_, ok := app.config.Admins[localpart]

	return ok
}

// session returns the subject named by a valid cookie for this role.
func (app *Server) session(r *http.Request, role string, cookieName string) (string, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return "", errNoSession
	}

	subject, err := app.sessionKey.decodeSession(role, cookie.Value, time.Now())
	if err != nil {
		// Forged, stale, and mistyped cookies are all the same
		// answer. Only the log distinguishes them.
		return "", errNoSession
	}

	return subject, nil
}

// Unauthenticated requests get a 401 JSON error, never a redirect.
func (app *Server) studentAPI(handlerName string, handler func(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		app.logRequestStart(r, handlerName, slog.String("middleware", "studentAPI"))

		sui, err := app.authenticateStudent(r)
		if err != nil {
			app.apiError(r, w, http.StatusUnauthorized, codeUnauthenticated, "no valid student session", err)

			return
		}

		handler(w, r, sui)
	}
}

// Unauthenticated requests get a 401 JSON error, never a redirect.
func (app *Server) adminAPI(handlerName string, handler func(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		app.logRequestStart(r, handlerName, slog.String("middleware", "adminAPI"))

		aui, err := app.authenticateAdmin(r)
		if err != nil {
			app.apiError(r, w, http.StatusUnauthorized, codeUnauthenticated, "no valid admin session", err)

			return
		}

		handler(w, r, aui)
	}
}

// spaHandler gates an area root: a valid session serves the SPA index
// and anything else serves that role's sign-in page.
func (app *Server) spaHandler(role string, indexFile string, authenticate func(*http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		app.logRequestStart(r, role+"Frontend")

		if err := authenticate(r); err != nil {
			app.serveSignin(w, r, role, takeFlash(w, r))

			return
		}

		http.ServeFileFS(w, r, ui.Dist, indexFile)
	}
}
