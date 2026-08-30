package httpserver

import (
	"bytes"
	"io"
	"math/rand"
	"net/http"
	"strconv"
)

// chunkSize bounds per-request buffer allocation for synthetic payloads so
// memory use doesn't scale with the requested response size.
const chunkSize = 32 * 1024

// MaxSyntheticBytes caps /size and /bytes requests to keep a single
// misbehaving request from generating an unbounded response.
const MaxSyntheticBytes = 1 << 30 // 1 GiB

// zeroChunk is a shared, read-only buffer of zero bytes reused across all
// /size requests to avoid per-request allocation proportional to size.
var zeroChunk = make([]byte, chunkSize)

// handleSize streams exactly n zero-filled bytes.
func (s *Server) handleSize(w http.ResponseWriter, r *http.Request) {
	n, ok := parseByteCount(w, r.PathValue("bytes"))
	if !ok {
		return
	}
	writeExactBytes(w, n, func(remaining int64) []byte {
		if remaining < chunkSize {
			return zeroChunk[:remaining]
		}
		return zeroChunk
	})
}

// handleBytes streams exactly n bytes: zero-filled by default, or
// deterministic pseudo-random when ?seed= is given.
func (s *Server) handleBytes(w http.ResponseWriter, r *http.Request) {
	n, ok := parseByteCount(w, r.PathValue("n"))
	if !ok {
		return
	}

	seedParam := r.URL.Query().Get("seed")
	if seedParam == "" {
		writeExactBytes(w, n, func(remaining int64) []byte {
			if remaining < chunkSize {
				return zeroChunk[:remaining]
			}
			return zeroChunk
		})
		return
	}

	seed, err := strconv.ParseInt(seedParam, 10, 64)
	if err != nil {
		http.Error(w, "invalid seed", http.StatusBadRequest)
		return
	}
	rng := rand.New(rand.NewSource(seed))
	buf := make([]byte, chunkSize)
	writeExactBytes(w, n, func(remaining int64) []byte {
		size := int64(chunkSize)
		if remaining < size {
			size = remaining
		}
		_, _ = rng.Read(buf[:size])
		return buf[:size]
	})
}

func parseByteCount(w http.ResponseWriter, raw string) (int64, bool) {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		http.Error(w, "invalid byte count", http.StatusBadRequest)
		return 0, false
	}
	if n > MaxSyntheticBytes {
		http.Error(w, "byte count too large", http.StatusBadRequest)
		return 0, false
	}
	return n, true
}

// writeExactBytes streams exactly n bytes to w, requesting each chunk from
// next (which is handed the number of bytes remaining).
func writeExactBytes(w http.ResponseWriter, n int64, next func(remaining int64) []byte) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
	w.WriteHeader(http.StatusOK)

	remaining := n
	for remaining > 0 {
		chunk := next(remaining)
		if _, err := io.CopyN(w, bytes.NewReader(chunk), int64(len(chunk))); err != nil {
			return
		}
		remaining -= int64(len(chunk))
	}
}
