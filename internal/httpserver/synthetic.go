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

// zeroChunkNext is the writeExactBytes callback shared by every route that
// serves zero-filled content.
func zeroChunkNext(remaining int64) []byte {
	if remaining < chunkSize {
		return zeroChunk[:remaining]
	}
	return zeroChunk
}

// handleSize streams exactly n zero-filled bytes.
func (s *Server) handleSize(w http.ResponseWriter, r *http.Request) {
	n, ok := parseByteCount(w, r.PathValue("bytes"))
	if !ok {
		return
	}
	writeExactBytes(w, n, "application/octet-stream", zeroChunkNext)
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
		writeExactBytes(w, n, "application/octet-stream", zeroChunkNext)
		return
	}

	seed, err := strconv.ParseInt(seedParam, 10, 64)
	if err != nil {
		http.Error(w, "invalid seed", http.StatusBadRequest)
		return
	}
	rng := rand.New(rand.NewSource(seed))
	buf := make([]byte, chunkSize)
	writeExactBytes(w, n, "application/octet-stream", func(remaining int64) []byte {
		size := int64(chunkSize)
		if remaining < size {
			size = remaining
		}
		_, _ = rng.Read(buf[:size])
		return buf[:size]
	})
}

// handleStreamBytes streams n bytes of random content in chunk_size
// pieces (default and max chunkSize), deliberately without a
// Content-Length so the response goes out chunked — for streaming-
// consumer and backpressure demos, distinct from /bytes's fixed buffer.
func (s *Server) handleStreamBytes(w http.ResponseWriter, r *http.Request) {
	n, ok := parseByteCount(w, r.PathValue("n"))
	if !ok {
		return
	}

	size := chunkSize
	if raw := r.URL.Query().Get("chunk_size"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			http.Error(w, "invalid chunk_size", http.StatusBadRequest)
			return
		}
		if v > chunkSize {
			v = chunkSize
		}
		size = v
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	buf := make([]byte, size)
	remaining := n
	for remaining > 0 {
		this := int64(size)
		if remaining < this {
			this = remaining
		}
		_, _ = rand.Read(buf[:this])
		if _, err := w.Write(buf[:this]); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		remaining -= this

		select {
		case <-r.Context().Done():
			return
		default:
		}
	}
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

// writeExactBytes streams exactly n bytes to w as contentType, requesting
// each chunk from next (which is handed the number of bytes remaining).
func writeExactBytes(w http.ResponseWriter, n int64, contentType string, next func(remaining int64) []byte) {
	w.Header().Set("Content-Type", contentType)
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
