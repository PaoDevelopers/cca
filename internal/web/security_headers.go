package web

import "net/http"

// The headers a browser needs to enforce our assumptions for us.
//
// Everything below is defence in depth: none of it is what stops an
// attack, and each covers a case where something else was already
// wrong. That is the point — they are cheap, and the expensive
// mitigations are the ones that have to be right the first time.
//
// A note on what is *not* here. HSTS is deliberately omitted: this
// process serves plain HTTP behind a reverse proxy that terminates
// TLS, and a max-age emitted from here would be a promise made by
// something with no knowledge of whether it can be kept — and one that
// a browser then caches for months. It belongs to whatever holds the
// certificate.
func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()

		// Both SPAs are entirely self-hosted: their scripts, styles and
		// fonts are built by vite into assets/ and served from here.
		// Nothing is fetched from a CDN, so 'self' is not a compromise
		// between safety and function — it is exactly the truth.
		//
		// 'unsafe-inline' for styles only, because vite inlines a small
		// critical style block; scripts have no such exception, which
		// is where it matters.
		//
		// connect-src is 'self' alone, which CSP3 defines to cover the
		// same origin's ws:// and wss:// as well — so the event socket
		// connects and nothing else does. It used to also list "ws:
		// wss:", which read as "allow the socket" but are
		// scheme-sources: they match any host, and turned the one
		// directive whose job is to stop a script phoning home into one
		// that permitted it to any server on the internet.
		//
		// frame-ancestors 'none' and object-src 'none' close the two
		// remaining ways to get someone else's code onto the page;
		// base-uri 'self' stops an injected <base> from repointing
		// every relative URL on it.
		header.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; "+
				"font-src 'self'; "+
				"connect-src 'self'; "+
				"form-action 'self'; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"object-src 'none'")

		// The exports are the reason. A CSV whose first bytes look like
		// HTML is sniffed as HTML by older browsers and rendered in our
		// origin, which turns a spreadsheet upload into stored XSS. We
		// label every response correctly; this says to believe us.
		header.Set("X-Content-Type-Options", "nosniff")

		// frame-ancestors covers this for anything current. Kept for
		// the browsers that a school's managed fleet still runs.
		header.Set("X-Frame-Options", "DENY")

		// A student id and a course id in a path are not secrets, but
		// they are not an outside site's business either, and the OIDC
		// redirect carries state through a referrer.
		header.Set("Referrer-Policy", "same-origin")

		// Nothing here uses any of them, and a compromised dependency
		// asking for the microphone should be refused by the browser
		// rather than by a prompt the user is trained to accept.
		header.Set("Permissions-Policy",
			"camera=(), microphone=(), geolocation=(), payment=(), usb=()")

		h.ServeHTTP(w, r)
	})
}
