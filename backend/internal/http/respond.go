package http

import (
	"encoding/json"
	"net/http"
)

// errorEnvelope is the shared non-2xx body shape
// (docs/04-api.md §Error envelope).
type errorEnvelope struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// writeJSON encodes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError emits the stable error envelope used across the API.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: code, Message: message})
}
