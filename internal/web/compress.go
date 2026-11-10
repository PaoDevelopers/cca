package web

import (
	"compress/gzip"
	"net/http"
	"strings"
	"sync"
)

// The panels' scripts, styles and JSON are text, and the admin panel
// loads several hundred kilobytes of them: a roster, its standing, the
// course list. Gzipped they are about a quarter of that, which is the
// difference on a school network at the start of an enrollment window.
//
// Only text is compressed. Spreadsheets are already zip archives, and
// a range request asks for bytes of the uncompressed file, which a
// compressed body does not have. The event socket is never wrapped: an
// upgrade hijacks the connection, and there is no body to compress.
func compress(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !acceptsGzip(r) || r.Header.Get("Upgrade") != "" || r.Header.Get("Range") != "" {
			h.ServeHTTP(w, r)

			return
		}

		w.Header().Add("Vary", "Accept-Encoding")

		gw := &gzipWriter{ResponseWriter: w} //exhaustruct:ignore
		defer gw.close()

		h.ServeHTTP(gw, r)
	})
}

func acceptsGzip(r *http.Request) bool {
	for part := range strings.SplitSeq(r.Header.Get("Accept-Encoding"), ",") {
		coding, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(coding, "gzip") && strings.ReplaceAll(params, " ", "") != "q=0" {
			return true
		}
	}

	return false
}

func compressible(contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	mediaType = strings.TrimSpace(strings.ToLower(mediaType))

	return strings.HasPrefix(mediaType, "text/") ||
		mediaType == "application/json" ||
		mediaType == "application/javascript" ||
		mediaType == "image/svg+xml"
}

// gzip.Writer holds a large window; one per response would be most of
// what a small JSON reply costs to send.
//
//nolint:gochecknoglobals // a pool is shared by definition
var gzipPool = sync.Pool{New: func() any { return gzip.NewWriter(nil) }}

// gzipWriter decides at WriteHeader, when the content type is known,
// whether this response is compressed; until then it is undecided.
type gzipWriter struct {
	http.ResponseWriter

	decided bool
	gz      *gzip.Writer
}

func (g *gzipWriter) WriteHeader(status int) {
	if !g.decided {
		g.decided = true
		header := g.Header()

		// 204 and 304 have no body; a handler that set its own encoding
		// knows better.
		if status != http.StatusNoContent && status != http.StatusNotModified &&
			header.Get("Content-Encoding") == "" && compressible(header.Get("Content-Type")) {
			header.Set("Content-Encoding", "gzip")
			// The length was of the uncompressed body.
			header.Del("Content-Length")

			gz, _ := gzipPool.Get().(*gzip.Writer)
			gz.Reset(g.ResponseWriter)
			g.gz = gz
		}
	}

	g.ResponseWriter.WriteHeader(status)
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	if !g.decided {
		if g.Header().Get("Content-Type") == "" {
			g.Header().Set("Content-Type", http.DetectContentType(b))
		}

		g.WriteHeader(http.StatusOK)
	}

	if g.gz == nil {
		return g.ResponseWriter.Write(b) //nolint:wrapcheck // a pass-through
	}

	return g.gz.Write(b) //nolint:wrapcheck // a pass-through
}

// Flush pushes what has been compressed so far, so a handler that
// flushes still gets its bytes to the client.
func (g *gzipWriter) Flush() {
	if g.gz != nil {
		_ = g.gz.Flush()
	}

	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the real writer.
func (g *gzipWriter) Unwrap() http.ResponseWriter {
	return g.ResponseWriter
}

func (g *gzipWriter) close() {
	if g.gz == nil {
		return
	}

	_ = g.gz.Close()
	g.gz.Reset(nil)
	gzipPool.Put(g.gz)
	g.gz = nil
}
