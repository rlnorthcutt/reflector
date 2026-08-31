package httpserver

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/rlnorthcutt/reflector/internal/envelope"
)

// compressionRetentionCap bounds what sync.Pool is allowed to retain: a
// writer or buffer whose backing storage grew past this, from an
// unusually large request body, is dropped instead of pooled so one big
// request doesn't inflate memory use for everyone after it.
const compressionRetentionCap = 64 * 1024

var gzipWriterPool = sync.Pool{
	New: func() any { return gzip.NewWriter(io.Discard) },
}

var flateWriterPool = sync.Pool{
	New: func() any {
		w, _ := flate.NewWriter(io.Discard, flate.DefaultCompression)
		return w
	},
}

var compressionBufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

func getCompressionBuffer() *bytes.Buffer {
	return compressionBufferPool.Get().(*bytes.Buffer)
}

func putCompressionBuffer(buf *bytes.Buffer) {
	if buf.Cap() > compressionRetentionCap {
		return
	}
	buf.Reset()
	compressionBufferPool.Put(buf)
}

// encodedEnvelope is the envelope plus which encoding was applied, mirroring
// httpbin's /gzip and /deflate response shape in Reflector's own schema.
type encodedEnvelope struct {
	envelope.Envelope
	Encoding string `json:"encoding"`
}

// handleGzip always returns the envelope gzip-compressed, regardless of
// the client's Accept-Encoding — the point of the route is to exercise a
// client's decompression, not to perform ordinary negotiation.
func (s *Server) handleGzip(w http.ResponseWriter, r *http.Request) {
	body, err := json.Marshal(encodedEnvelope{Envelope: envelope.Build(r, s.ID), Encoding: "gzip"})
	if err != nil {
		http.Error(w, "encoding error", http.StatusInternalServerError)
		return
	}

	buf := getCompressionBuffer()
	defer putCompressionBuffer(buf)

	gw := gzipWriterPool.Get().(*gzip.Writer)
	gw.Reset(buf)
	_, _ = gw.Write(body)
	_ = gw.Close()
	gzipWriterPool.Put(gw)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// handleDeflate always returns the envelope deflate-compressed.
func (s *Server) handleDeflate(w http.ResponseWriter, r *http.Request) {
	body, err := json.Marshal(encodedEnvelope{Envelope: envelope.Build(r, s.ID), Encoding: "deflate"})
	if err != nil {
		http.Error(w, "encoding error", http.StatusInternalServerError)
		return
	}

	buf := getCompressionBuffer()
	defer putCompressionBuffer(buf)

	fw := flateWriterPool.Get().(*flate.Writer)
	fw.Reset(buf)
	_, _ = fw.Write(body)
	_ = fw.Close()
	flateWriterPool.Put(fw)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Encoding", "deflate")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
