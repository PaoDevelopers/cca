package web //nolint:testpackage

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompressGzipsTextAndLeavesTheRestAlone(t *testing.T) {
	t.Parallel()

	body := strings.Repeat(`{"id":"s1001","name":"Alice"},`, 200)

	handler := compress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "999999")
		case "/xlsx":
			w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		}

		_, _ = io.WriteString(w, body)
	}))

	get := func(path string, headers map[string]string) *http.Response {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		for k, v := range headers {
			request.Header.Set(k, v)
		}

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		return recorder.Result()
	}

	gzipped := get("/json", map[string]string{"Accept-Encoding": "br, gzip"})
	defer func() { _ = gzipped.Body.Close() }()

	if gzipped.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("JSON for a gzip client: Content-Encoding = %q", gzipped.Header.Get("Content-Encoding"))
	}

	if gzipped.Header.Get("Content-Length") != "" {
		t.Errorf("the uncompressed Content-Length survived: %q", gzipped.Header.Get("Content-Length"))
	}

	reader, err := gzip.NewReader(gzipped.Body)
	if err != nil {
		t.Fatal(err)
	}

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != body {
		t.Errorf("the body did not round-trip through gzip")
	}

	for _, tc := range []struct {
		name    string
		path    string
		headers map[string]string
	}{
		{"no Accept-Encoding", "/json", nil},
		{"gzip refused", "/json", map[string]string{"Accept-Encoding": "gzip;q=0"}},
		{"a binary type", "/xlsx", map[string]string{"Accept-Encoding": "gzip"}},
		{"a range request", "/json", map[string]string{"Accept-Encoding": "gzip", "Range": "bytes=0-9"}},
		{"a socket upgrade", "/json", map[string]string{"Accept-Encoding": "gzip", "Upgrade": "websocket"}},
	} {
		response := get(tc.path, tc.headers)
		_ = response.Body.Close()

		if encoding := response.Header.Get("Content-Encoding"); encoding != "" {
			t.Errorf("%s: Content-Encoding = %q, want none", tc.name, encoding)
		}
	}
}
