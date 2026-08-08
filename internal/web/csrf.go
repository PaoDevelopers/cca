package web

import "net/http"

// Cross-origin write protection, on top of what the session cookies
// already do.
//
// The session cookies are SameSite=Lax, which stops a *cross-site*
// POST from carrying them. Lax is same-site rather than same-origin,
// though, and same-site is generous: any host under the same
// registrable domain counts, as does any port. A page on a sibling
// host is same-site, so its cross-origin writes do carry the session
// cookie, and Lax alone does not stop them. This does, because it
// requires same-origin.
//
// That gap is reachable in practice. Both CSV imports post
// multipart/form-data, and the two logout forms are plain <form
// method="post">, which the browser sends as
// application/x-www-form-urlencoded. Those are two of the three
// CORS-safelisted content types, which is exactly what makes them
// sendable cross-origin with no preflight — the JSON writes are
// already preflighted by virtue of theirs.
//
// Note that the event WebSockets do not need anything here: they are
// GETs, which are always allowed, and coder/websocket's Accept checks
// the Origin against the Host by default.
func (app *Server) crossOriginProtection() *http.CrossOriginProtection {
	protection := http.NewCrossOriginProtection()

	// The OIDC callback is a cross-site form_post from the identity
	// provider by design, so it can never look same-origin. What
	// protects it is the nonce cookie and the signature check on the
	// id_token, neither of which depends on this; see handleAuth.
	protection.AddInsecureBypassPattern("POST /auth")

	// Rejections read like every other refusal from this server rather
	// than like a bare 403 from the standard library.
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.apiError(
			r, w,
			http.StatusForbidden,
			codeForbidden,
			"Cross-origin request rejected",
			nil,
		)
	}))

	return protection
}
