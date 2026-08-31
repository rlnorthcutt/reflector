// Package capture implements Reflector's request capture ring buffer:
// a bounded, in-memory record of recent requests, so traffic sent through
// a load balancer can be inspected for exactly what reached the backend —
// forwarded headers, rewritten paths, persistence cookies.
package capture

import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rlnorthcutt/reflector/internal/envelope"
)

// MaxBodyBytes bounds how much of a captured request's body is retained.
const MaxBodyBytes = 1 << 20 // 1 MiB

// Entry is one captured request.
type Entry struct {
	Timestamp string              `json:"timestamp"`
	Method    string              `json:"method"`
	Path      string              `json:"path"`
	Proto     string              `json:"proto"`
	Headers   map[string][]string `json:"headers"`
	Query     map[string][]string `json:"query"`
	Body      string              `json:"body"`
	Truncated bool                `json:"truncated"`
	Client    envelope.Client     `json:"client"`
}

// Buffer is a fixed-capacity FIFO ring buffer of Entries.
type Buffer struct {
	mu      sync.Mutex
	max     int
	entries []Entry
}

// NewBuffer returns a Buffer holding at most max entries, evicting the
// oldest on overflow.
func NewBuffer(max int) *Buffer {
	if max < 1 {
		max = 1
	}
	return &Buffer{max: max}
}

// Add appends e, evicting the oldest entry if the buffer is full.
func (b *Buffer) Add(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = append(b.entries, e)
	if len(b.entries) > b.max {
		b.entries = b.entries[len(b.entries)-b.max:]
	}
}

// All returns a snapshot of the currently captured entries, oldest first.
func (b *Buffer) All() []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Entry, len(b.entries))
	copy(out, b.entries)
	return out
}

// Clear empties the buffer.
func (b *Buffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = nil
}

// Middleware records every request that passes through next into buf. If
// buf is nil, capture is disabled and next is returned unwrapped. The
// request body is fully preserved for next despite being peeked at here.
func Middleware(buf *Buffer, next http.Handler) http.Handler {
	if buf == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peek, _ := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
		truncated := len(peek) > MaxBodyBytes
		stored := peek
		if truncated {
			stored = peek[:MaxBodyBytes]
		}

		r.Body = struct {
			io.Reader
			io.Closer
		}{
			Reader: io.MultiReader(bytes.NewReader(peek), r.Body),
			Closer: r.Body,
		}

		buf.Add(Entry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Method:    r.Method,
			Path:      r.URL.Path,
			Proto:     r.Proto,
			Headers:   map[string][]string(r.Header),
			Query:     map[string][]string(r.URL.Query()),
			Body:      string(stored),
			Truncated: truncated,
			Client:    envelope.ClientInfo(r),
		})

		next.ServeHTTP(w, r)
	})
}
