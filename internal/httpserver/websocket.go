package httpserver

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// websocketGUID is the magic constant RFC 6455 §1.3 mixes into the
// Sec-WebSocket-Key to derive the handshake's Accept value.
const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// WebSocket opcodes (RFC 6455 §5.2).
const (
	wsOpContinuation byte = 0x0
	wsOpText         byte = 0x1
	wsOpBinary       byte = 0x2
	wsOpClose        byte = 0x8
	wsOpPing         byte = 0x9
	wsOpPong         byte = 0xA
)

// maxWSFramePayload bounds a single frame's payload: /ws is an echo demo,
// not a bulk-transfer tool.
const maxWSFramePayload = 1 << 20 // 1 MiB

// errWSProtocol marks a frame that violates RFC 6455 framing rules
// (unmasked client frame, reserved bits set, an oversized or fragmented
// control frame, or an over-limit payload), as opposed to a plain I/O
// error from the connection going away.
var errWSProtocol = errors.New("websocket: protocol error")

// wsFrame is one decoded WebSocket frame, already unmasked.
type wsFrame struct {
	fin     bool
	opcode  byte
	payload []byte
}

// handleWS upgrades the connection to WebSocket (RFC 6455) and echoes
// text and binary frames back verbatim, replying to ping with pong. On
// connect it sends one identity banner text frame — the WebSocket
// analogue of --tcp-banner — so L7 round-robin and stickiness stay
// visible over a persistent connection. A request that isn't a valid
// WebSocket upgrade gets 426 Upgrade Required instead of hanging.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if !isWebSocketUpgrade(r) || key == "" {
		w.Header().Set("Upgrade", "websocket")
		w.WriteHeader(http.StatusUpgradeRequired)
		fmt.Fprintln(w, "this endpoint speaks the WebSocket upgrade only (RFC 6455); send GET with Upgrade: websocket")
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket hijacking unsupported", http.StatusInternalServerError)
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		s.Logger.Error("websocket hijack failed", "error", err)
		return
	}
	defer conn.Close()

	handshake := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + websocketAccept(key) + "\r\n\r\n"
	if _, err := buf.WriteString(handshake); err != nil || buf.Flush() != nil {
		return
	}

	start := time.Now()
	s.Logger.Info("websocket connected", "path", r.URL.Path, "remote", r.RemoteAddr)
	defer func() {
		s.Logger.Info("websocket disconnected", "path", r.URL.Path, "remote", r.RemoteAddr, "duration", time.Since(start))
	}()

	banner := s.ID.Banner(s.ID.Port)
	if err := writeWSFrame(buf.Writer, true, wsOpText, []byte(banner)); err != nil {
		return
	}

	for {
		frame, err := readWSFrame(buf.Reader)
		if err != nil {
			if errors.Is(err, errWSProtocol) {
				_ = writeWSClose(buf.Writer, 1002, "protocol error")
			}
			return
		}

		switch frame.opcode {
		case wsOpText, wsOpBinary, wsOpContinuation:
			if err := writeWSFrame(buf.Writer, frame.fin, frame.opcode, frame.payload); err != nil {
				return
			}
		case wsOpPing:
			if err := writeWSFrame(buf.Writer, true, wsOpPong, frame.payload); err != nil {
				return
			}
		case wsOpPong:
			// Unsolicited pong; nothing to do.
		case wsOpClose:
			code := uint16(1000)
			if len(frame.payload) >= 2 {
				code = binary.BigEndian.Uint16(frame.payload)
			}
			_ = writeWSClose(buf.Writer, code, "")
			return
		default:
			_ = writeWSClose(buf.Writer, 1002, "unsupported opcode")
			return
		}
	}
}

// isWebSocketUpgrade reports whether r carries the headers RFC 6455 §4.1
// requires of a client handshake: GET, "Connection: Upgrade" (token
// among possibly several), and "Upgrade: websocket".
func isWebSocketUpgrade(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if !headerHasToken(r.Header, "Connection", "upgrade") {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket")
}

func headerHasToken(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// websocketAccept computes Sec-WebSocket-Accept from a client's
// Sec-WebSocket-Key per RFC 6455 §1.3: base64(sha1(key + magic GUID)).
func websocketAccept(key string) string {
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte(websocketGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// readWSFrame reads and unmasks one client-to-server frame. Client frames
// must be masked (RFC 6455 §5.1); an unmasked frame, reserved bits, a
// fragmented or oversized control frame, or a payload over
// maxWSFramePayload all return errWSProtocol. Any other error is the
// underlying connection going away.
func readWSFrame(br *bufio.Reader) (wsFrame, error) {
	var head [2]byte
	if _, err := io.ReadFull(br, head[:]); err != nil {
		return wsFrame{}, err
	}
	if head[0]&0x70 != 0 { // RSV1-3: no extensions are negotiated
		return wsFrame{}, errWSProtocol
	}

	fin := head[0]&0x80 != 0
	opcode := head[0] & 0x0f
	masked := head[1]&0x80 != 0
	length := uint64(head[1] & 0x7f)

	if opcode >= 0x8 && (!fin || length > 125) {
		return wsFrame{}, errWSProtocol
	}

	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(br, ext[:]); err != nil {
			return wsFrame{}, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(br, ext[:]); err != nil {
			return wsFrame{}, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > maxWSFramePayload {
		return wsFrame{}, errWSProtocol
	}
	if !masked {
		return wsFrame{}, errWSProtocol
	}

	var maskKey [4]byte
	if _, err := io.ReadFull(br, maskKey[:]); err != nil {
		return wsFrame{}, err
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(br, payload); err != nil {
		return wsFrame{}, err
	}
	for i := range payload {
		payload[i] ^= maskKey[i%4]
	}

	return wsFrame{fin: fin, opcode: opcode, payload: payload}, nil
}

// writeWSFrame writes one unmasked server-to-client frame (RFC 6455
// requires server frames NOT be masked).
func writeWSFrame(bw *bufio.Writer, fin bool, opcode byte, payload []byte) error {
	b0 := opcode
	if fin {
		b0 |= 0x80
	}

	var head []byte
	switch length := len(payload); {
	case length <= 125:
		head = []byte{b0, byte(length)}
	case length <= 0xFFFF:
		head = make([]byte, 4)
		head[0], head[1] = b0, 126
		binary.BigEndian.PutUint16(head[2:], uint16(length))
	default:
		head = make([]byte, 10)
		head[0], head[1] = b0, 127
		binary.BigEndian.PutUint64(head[2:], uint64(length))
	}

	if _, err := bw.Write(head); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := bw.Write(payload); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// writeWSClose writes a close frame carrying the given status code and
// optional UTF-8 reason (RFC 6455 §5.5.1).
func writeWSClose(bw *bufio.Writer, code uint16, reason string) error {
	payload := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(payload, code)
	copy(payload[2:], reason)
	return writeWSFrame(bw, true, wsOpClose, payload)
}
