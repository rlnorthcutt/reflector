package httpserver

import (
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// maxDripBytes bounds /drip's numbytes: it's a slow-trickle demo, not a
// bulk-payload one, so this is far smaller than MaxSyntheticBytes.
const maxDripBytes = 1 << 20 // 1 MiB

// handleDrip trickles numbytes over duration, after an optional initial
// delay, writing the given status code up front. Each inter-byte wait
// selects on the request context so a client disconnect frees resources
// immediately.
func (s *Server) handleDrip(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	numBytes, ok := parseDripQueryInt(w, q, "numbytes", 10, 0, maxDripBytes)
	if !ok {
		return
	}
	code, ok := parseDripQueryInt(w, q, "code", http.StatusOK, 100, 599)
	if !ok {
		return
	}
	duration, ok := parseDripQueryDuration(w, q, "duration", 2*time.Second)
	if !ok {
		return
	}
	delay, ok := parseDripQueryDuration(w, q, "delay", 0)
	if !ok {
		return
	}
	jitter, ok := parseDripQueryDuration(w, q, "jitter", 0)
	if !ok {
		return
	}

	if delay > 0 && !wait(r, delay) {
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(numBytes))
	w.WriteHeader(code)

	if numBytes == 0 {
		return
	}
	flusher, _ := w.(http.Flusher)
	interval := duration / time.Duration(numBytes)

	for i := 0; i < numBytes; i++ {
		if _, err := w.Write([]byte{0}); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		if i == numBytes-1 {
			return
		}
		gap := interval
		if jitter > 0 {
			gap += time.Duration(rand.Int63n(int64(jitter)))
		}
		if gap > 0 && !wait(r, gap) {
			return
		}
	}
}

// wait blocks for d, returning false early (without having waited the
// full duration) if the request's context is done first.
func wait(r *http.Request, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-r.Context().Done():
		return false
	}
}

func parseDripQueryInt(w http.ResponseWriter, q map[string][]string, name string, def, min, max int) (int, bool) {
	raw := firstOr(q, name, "")
	if raw == "" {
		return def, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < min || v > max {
		http.Error(w, "invalid "+name, http.StatusBadRequest)
		return 0, false
	}
	return v, true
}

func parseDripQueryDuration(w http.ResponseWriter, q map[string][]string, name string, def time.Duration) (time.Duration, bool) {
	raw := firstOr(q, name, "")
	if raw == "" {
		return def, true
	}
	v, err := time.ParseDuration(raw)
	if err != nil || v < 0 {
		http.Error(w, "invalid "+name, http.StatusBadRequest)
		return 0, false
	}
	return v, true
}

func firstOr(q map[string][]string, name, def string) string {
	if v, ok := q[name]; ok && len(v) > 0 {
		return v[0]
	}
	return def
}
