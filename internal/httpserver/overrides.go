package httpserver

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// bufferedRecorder is a minimal in-package http.ResponseWriter that
// buffers a full response, used only when a status/body override needs
// to inspect or replace what the inner handler produced.
type bufferedRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newBufferedRecorder() *bufferedRecorder {
	return &bufferedRecorder{header: make(http.Header), status: http.StatusOK}
}

func (r *bufferedRecorder) Header() http.Header { return r.header }

func (r *bufferedRecorder) Write(b []byte) (int, error) { return r.body.Write(b) }

func (r *bufferedRecorder) WriteHeader(status int) { r.status = status }

// withOverrides wraps next with per-request overrides, honored on every
// route (built-in and user/preset) unless --no-overrides is set:
//
//   - ?_status=503 / X-Reflector-Status: 503 — replace the response status
//   - ?_delay=2s   / X-Reflector-Delay: 2s   — wait before responding
//   - ?_body=...   / X-Reflector-Body: ...   — replace the response body
//   - ?_connection=close / X-Reflector-Connection: close — force the
//     connection closed after this response
//   - ?_abort=1    / X-Reflector-Abort: 1    — write a partial response
//     then abort the connection, simulating a mid-response crash
//
// Status/body overrides require buffering the inner handler's full
// response, so that cost is only paid on requests that actually ask for
// one — every other request (including streaming routes like /drip and
// /stream-bytes) passes through untouched.
func withOverrides(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if delay, ok := overrideDuration(r, "_delay", "X-Reflector-Delay"); ok {
			if delay > 0 && !wait(r, delay) {
				return
			}
		}

		if forceClose, ok := overrideValue(r, "_connection", "X-Reflector-Connection"); ok && strings.EqualFold(forceClose, "close") {
			w.Header().Set("Connection", "close")
		}

		if abort, ok := overrideBool(r, "_abort", "X-Reflector-Abort"); ok && abort {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("partial response before simulated connection reset\n"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			panic(http.ErrAbortHandler)
		}

		statusOverride, hasStatus := overrideInt(r, "_status", "X-Reflector-Status")
		bodyOverride, hasBody := overrideValue(r, "_body", "X-Reflector-Body")

		// Status/body overrides require buffering the full response,
		// which is incompatible with a connection handed off via
		// Hijack — so a WebSocket upgrade always passes through
		// untouched rather than 500ing on an unsupported hijack.
		if (!hasStatus && !hasBody) || isWebSocketUpgrade(r) {
			next.ServeHTTP(w, r)
			return
		}

		rec := newBufferedRecorder()
		next.ServeHTTP(rec, r)

		for k, v := range rec.header {
			w.Header()[k] = v
		}
		body := rec.body.Bytes()
		if hasBody {
			body = []byte(bodyOverride)
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		}
		status := rec.status
		if hasStatus {
			status = statusOverride
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
}

func overrideValue(r *http.Request, queryName, headerName string) (string, bool) {
	if v := r.URL.Query().Get(queryName); v != "" {
		return v, true
	}
	if v := r.Header.Get(headerName); v != "" {
		return v, true
	}
	return "", false
}

func overrideInt(r *http.Request, queryName, headerName string) (int, bool) {
	raw, ok := overrideValue(r, queryName, headerName)
	if !ok {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return v, true
}

func overrideDuration(r *http.Request, queryName, headerName string) (time.Duration, bool) {
	raw, ok := overrideValue(r, queryName, headerName)
	if !ok {
		return 0, false
	}
	v, err := time.ParseDuration(raw)
	if err != nil || v < 0 {
		return 0, false
	}
	return v, true
}

func overrideBool(r *http.Request, queryName, headerName string) (bool, bool) {
	raw, ok := overrideValue(r, queryName, headerName)
	if !ok {
		return false, false
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes":
		return true, true
	}
	return false, true
}
