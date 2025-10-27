package main

import (
	"encoding/json"
	"net/http"
)

func apiHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Content-Type", "application/json")
}

func apiError(w http.ResponseWriter, code int, v any) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
