package httpapi

import (
	"net/http"
	"strings"
)

func serveFrontendIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, "./frontend/dist/index.html")
}

func frontendAssetsHandler() http.Handler {
	fileServer := http.StripPrefix("/assets/", http.FileServer(http.Dir("frontend/dist/assets")))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, ".") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	})
}
