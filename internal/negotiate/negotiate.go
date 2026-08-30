// Package negotiate implements Reflector's content negotiation: JSON when
// a client explicitly asks for application/json, plaintext otherwise
// (including the curl default of "Accept: */*").
package negotiate

import (
	"encoding/json"
	"net/http"
	"strings"
)

// WantsJSON reports whether the request's Accept header explicitly asks
// for application/json. A bare "*/*" or missing Accept header does not
// count, so an unadorned curl request gets plaintext by default.
func WantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if accept == "" {
		return false
	}
	for _, part := range strings.Split(accept, ",") {
		mediaType := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if mediaType == "application/json" {
			return true
		}
	}
	return false
}

// PlainTexter is implemented by types that can render themselves as
// whoami-style plaintext.
type PlainTexter interface {
	PlainText() string
}

// Write renders v as JSON or plaintext depending on the request's Accept
// header, writing status as the response status code.
func Write(w http.ResponseWriter, r *http.Request, status int, v PlainTexter) {
	if WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(v.PlainText()))
}
