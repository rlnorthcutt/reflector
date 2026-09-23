package httpserver

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebsocketAccept(t *testing.T) {
	// RFC 6455 §1.3's worked example.
	got := websocketAccept("dGhlIHNhbXBsZSBub25jZQ==")
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got != want {
		t.Errorf("websocketAccept = %q, want %q", got, want)
	}
}

func TestHandleWSNonUpgradeReturns426(t *testing.T) {
	s := testServer()
	ts := httptest.NewServer(s.builtinMux())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ws")
	if err != nil {
		t.Fatalf("GET /ws: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUpgradeRequired)
	}
}

// wsTestClient is a minimal, from-scratch RFC 6455 client used only to
// exercise the server over a real socket, independent of the server's
// own frame (de)coding so the test doesn't just check the server against
// itself.
type wsTestClient struct {
	t    *testing.T
	conn net.Conn
	br   *bufio.Reader
}

func dialWS(t *testing.T, addr string) *wsTestClient {
	t.Helper()
	return dialWSPath(t, addr, "/ws")
}

func dialWSPath(t *testing.T, addr, path string) *wsTestClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	key := base64.StdEncoding.EncodeToString([]byte("0123456789012345"))
	req := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write handshake: %v", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read handshake response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("handshake status = %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}
	wantAccept := websocketAccept(key)
	if got := resp.Header.Get("Sec-WebSocket-Accept"); got != wantAccept {
		t.Fatalf("Sec-WebSocket-Accept = %q, want %q", got, wantAccept)
	}

	return &wsTestClient{t: t, conn: conn, br: br}
}

// send masks and writes one client-to-server frame, as RFC 6455 §5.1
// requires of every real client.
func (c *wsTestClient) send(opcode byte, payload []byte) {
	c.t.Helper()
	var mask [4]byte
	_, _ = rand.Read(mask[:])

	masked := make([]byte, len(payload))
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}

	head := []byte{0x80 | opcode, 0x80 | byte(len(payload))}
	if len(payload) > 125 {
		c.t.Fatalf("test client does not support extended lengths, got %d bytes", len(payload))
	}
	buf := append(head, mask[:]...)
	buf = append(buf, masked...)
	if _, err := c.conn.Write(buf); err != nil {
		c.t.Fatalf("write frame: %v", err)
	}
}

// recv reads one unmasked server-to-client frame.
func (c *wsTestClient) recv() wsFrame {
	c.t.Helper()
	var head [2]byte
	if _, err := io.ReadFull(c.br, head[:]); err != nil {
		c.t.Fatalf("read frame header: %v", err)
	}
	fin := head[0]&0x80 != 0
	opcode := head[0] & 0x0f
	length := uint64(head[1] & 0x7f)
	if head[1]&0x80 != 0 {
		c.t.Fatalf("server frame was masked, must not be per RFC 6455")
	}

	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			c.t.Fatalf("read extended length: %v", err)
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			c.t.Fatalf("read extended length: %v", err)
		}
		length = binary.BigEndian.Uint64(ext[:])
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		c.t.Fatalf("read payload: %v", err)
	}
	return wsFrame{fin: fin, opcode: opcode, payload: payload}
}

func TestHandleWSRoundTrip(t *testing.T) {
	s := testServer()
	ts := httptest.NewServer(s.builtinMux())
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	c := dialWS(t, addr)

	t.Run("banner", func(t *testing.T) {
		frame := c.recv()
		if frame.opcode != wsOpText {
			t.Fatalf("banner opcode = %d, want text", frame.opcode)
		}
		got := string(frame.payload)
		want := "reflector host=web-2 instance=a1b2c3 port=8080"
		if got != want {
			t.Errorf("banner = %q, want %q", got, want)
		}
	})

	t.Run("text echo", func(t *testing.T) {
		c.send(wsOpText, []byte("hello reflector"))
		frame := c.recv()
		if frame.opcode != wsOpText || string(frame.payload) != "hello reflector" {
			t.Errorf("got opcode=%d payload=%q, want text %q", frame.opcode, frame.payload, "hello reflector")
		}
	})

	t.Run("binary echo", func(t *testing.T) {
		payload := []byte{0x00, 0x01, 0xff, 0x10, 0x20}
		c.send(wsOpBinary, payload)
		frame := c.recv()
		if frame.opcode != wsOpBinary || string(frame.payload) != string(payload) {
			t.Errorf("got opcode=%d payload=%v, want binary %v", frame.opcode, frame.payload, payload)
		}
	})

	t.Run("ping pong", func(t *testing.T) {
		c.send(wsOpPing, []byte("are you there"))
		frame := c.recv()
		if frame.opcode != wsOpPong || string(frame.payload) != "are you there" {
			t.Errorf("got opcode=%d payload=%q, want pong %q", frame.opcode, frame.payload, "are you there")
		}
	})

	t.Run("close", func(t *testing.T) {
		payload := make([]byte, 2)
		binary.BigEndian.PutUint16(payload, 1000)
		c.send(wsOpClose, payload)

		frame := c.recv()
		if frame.opcode != wsOpClose {
			t.Errorf("got opcode=%d, want close", frame.opcode)
		}

		_ = c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, err := c.br.ReadByte(); err != io.EOF {
			t.Errorf("read after close = %v, want io.EOF (connection should be closed)", err)
		}
	})
}

func TestHandleWSUnmaskedFrameIsProtocolError(t *testing.T) {
	s := testServer()
	ts := httptest.NewServer(s.builtinMux())
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	c := dialWS(t, addr)
	_ = c.recv() // banner

	// An unmasked frame from a "client" violates RFC 6455 §5.1.
	if _, err := c.conn.Write([]byte{0x80 | wsOpText, 0x05, 'h', 'e', 'l', 'l', 'o'}); err != nil {
		t.Fatalf("write unmasked frame: %v", err)
	}

	frame := c.recv()
	if frame.opcode != wsOpClose {
		t.Fatalf("got opcode=%d, want close (protocol error)", frame.opcode)
	}
	if len(frame.payload) < 2 {
		t.Fatalf("close payload too short: %v", frame.payload)
	}
	if code := binary.BigEndian.Uint16(frame.payload); code != 1002 {
		t.Errorf("close code = %d, want 1002", code)
	}
}

// TestHandleWSWorksThroughLoggingMiddleware guards against a real
// regression: withLogging's statusRecorder wraps every response writer
// whenever access logging is enabled (the default, i.e. not --quiet), and
// embedding http.ResponseWriter does not by itself promote Hijack, so
// without statusRecorder.Hijack forwarding this would 500 instead of
// upgrading.
func TestHandleWSWorksThroughLoggingMiddleware(t *testing.T) {
	s := testServer()
	srv := New("127.0.0.1:0", s.ID, s.Logger, false, Options{Health: s.Health})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	c := dialWS(t, addr)

	c.send(wsOpText, []byte("through logging middleware"))
	// First frame is the identity banner, second is our echo.
	_ = c.recv()
	frame := c.recv()
	if frame.opcode != wsOpText || string(frame.payload) != "through logging middleware" {
		t.Errorf("got opcode=%d payload=%q, want text echo", frame.opcode, frame.payload)
	}
}

// TestHandleWSIgnoresStatusBodyOverrides guards against a real regression:
// withOverrides buffers the response in a *bufferedRecorder whenever
// ?_status or ?_body is present, and bufferedRecorder isn't a
// http.Hijacker, so /ws?_status=200 used to 500 instead of upgrading.
func TestHandleWSIgnoresStatusBodyOverrides(t *testing.T) {
	s := testServer()
	srv := New("127.0.0.1:0", s.ID, s.Logger, false, Options{Health: s.Health})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	c := dialWSPath(t, addr, "/ws?_status=200&_body=nope")

	frame := c.recv() // banner: proves the upgrade succeeded, override ignored
	if frame.opcode != wsOpText {
		t.Fatalf("got opcode=%d, want text banner (upgrade should succeed despite overrides)", frame.opcode)
	}
}

func TestBuiltinPatternsIncludesWS(t *testing.T) {
	for _, p := range BuiltinPatterns() {
		if p == "/ws" {
			return
		}
	}
	t.Errorf("BuiltinPatterns() = %v, want it to include /ws", BuiltinPatterns())
}
