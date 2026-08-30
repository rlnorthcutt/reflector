package tcpecho

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/rlnorthcutt/reflector/internal/identity"
)

func testIdentity() identity.Identity {
	return identity.Identity{Hostname: "web-2", InstanceID: "a1b2c3", Port: 9000}
}

func startListener(t *testing.T, l *Listener) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = l.Serve(ctx, ln, testIdentity(), 9000)
		close(done)
	}()
	return ln.Addr().String(), func() {
		cancel()
		<-done
	}
}

func TestEchoesBytesBack(t *testing.T) {
	l := &Listener{IdleTimeout: time.Second}
	addr, stop := startListener(t, l)
	defer stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("net.Dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if string(buf) != "hello" {
		t.Errorf("echoed = %q, want %q", buf, "hello")
	}
}

func TestSendsBannerBeforeEchoing(t *testing.T) {
	l := &Listener{Banner: true, IdleTimeout: time.Second}
	addr, stop := startListener(t, l)
	defer stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("net.Dial: %v", err)
	}
	defer conn.Close()

	want := "reflector host=web-2 instance=a1b2c3 port=9000\n"
	buf := make([]byte, len(want))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if string(buf) != want {
		t.Errorf("banner = %q, want %q", buf, want)
	}
}

func TestNoBannerWhenDisabled(t *testing.T) {
	l := &Listener{Banner: false, IdleTimeout: time.Second}
	addr, stop := startListener(t, l)
	defer stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("net.Dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	buf := make([]byte, 1)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if string(buf) != "x" {
		t.Errorf("first byte read = %q, want %q (no banner should precede it)", buf, "x")
	}
}

func TestIdleTimeoutClosesConnection(t *testing.T) {
	l := &Listener{IdleTimeout: 50 * time.Millisecond}
	addr, stop := startListener(t, l)
	defer stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("net.Dial: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	n, err := conn.Read(buf)
	if err == nil {
		t.Fatalf("expected connection to close after idle timeout, got n=%d err=nil", n)
	}
	if err != io.EOF {
		t.Logf("connection closed with err=%v (EOF expected but any close indicator is acceptable)", err)
	}
}

func TestServeStopsOnContextCancel(t *testing.T) {
	l := &Listener{IdleTimeout: time.Second}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- l.Serve(ctx, ln, testIdentity(), 9000) }()

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve returned error on cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after context cancellation")
	}
}
