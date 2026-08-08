package web

import (
	"net/http"
	"strings"
)

func requestAbsoluteURL(r *http.Request, path string) string {
	scheme := inferScheme(r)
	host := inferHost(r)

	if path == "" || path[0] != '/' {
		path = "/" + path
	}

	return scheme + "://" + host + path
}

func inferScheme(r *http.Request) string {
	if proto := headerFirstValue(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		return strings.ToLower(proto)
	}

	if r.TLS != nil {
		return "https"
	}

	return "http"
}

func inferHost(r *http.Request) string {
	if host := headerFirstValue(r.Header.Get("X-Forwarded-Host")); host != "" {
		return host
	}

	host := strings.TrimSpace(r.Host)
	if host == "" {
		return "localhost"
	}

	return host
}

// The first value of a comma-separated header. Cut rather than Split,
// because only the first element is wanted and splitting a
// twelve-proxy X-Forwarded-For to discard eleven of them allocates a
// slice per request to do it.
func headerFirstValue(header string) string {
	first, _, _ := strings.Cut(header, ",")

	return strings.TrimSpace(first)
}
