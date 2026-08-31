package routes

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/rlnorthcutt/reflector/internal/identity"
)

// maxRequestBodyBytes bounds how much of a request body a route template
// may read, mirroring the built-in envelope's cap.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

// BuildMux compiles a resolved, precedence-ordered route list into an
// http.ServeMux. Each Route already carries its body_file content, read
// at load time; body templates render fresh per request.
func BuildMux(list []Route, id identity.Identity) (*http.ServeMux, error) {
	mux := http.NewServeMux()
	for _, r := range list {
		if err := safeHandle(mux, r.Pattern(), r.handler(id)); err != nil {
			return nil, fmt.Errorf("%s: %s: %w", r.Source, r.Path, err)
		}
	}
	return mux, nil
}

// safeHandle registers pattern on mux, converting the panic ServeMux
// raises on an ambiguous/duplicate pattern into a regular error.
func safeHandle(mux *http.ServeMux, pattern string, h http.Handler) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("route conflict registering %q: %v", pattern, rec)
		}
	}()
	mux.Handle(pattern, h)
	return nil
}

func (r Route) handler(id identity.Identity) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if d := r.Delay.Sample(); d > 0 {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-req.Context().Done():
				return
			}
		}

		if r.Auth == "bearer" && !hasBearerToken(req) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("missing or invalid bearer token\n"))
			return
		}

		if r.Failure.Triggered() {
			w.WriteHeader(r.Failure.Status)
			_, _ = w.Write([]byte("simulated failure\n"))
			return
		}

		payload := r.bodyFileBytes
		contentType := r.bodyFileContentType
		if r.Body != nil {
			bodyBytes, _ := io.ReadAll(io.LimitReader(req.Body, maxRequestBodyBytes))
			ctx := NewContext(req, string(bodyBytes), id)
			var buf bytes.Buffer
			if err := r.Body.Execute(&buf, ctx); err != nil {
				http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
				return
			}
			payload = buf.Bytes()
		}

		for k, v := range r.Headers {
			w.Header().Set(k, v)
		}
		if w.Header().Get("Content-Type") == "" && contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(r.Status)
		_, _ = w.Write(payload)
	}
}

func hasBearerToken(r *http.Request) bool {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	return len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) && strings.TrimSpace(auth[len(prefix):]) != ""
}

func contentTypeForFile(name string) string {
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
