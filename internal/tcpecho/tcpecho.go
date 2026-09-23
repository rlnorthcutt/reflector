// Package tcpecho implements a raw TCP echo listener (RFC 862) with an
// optional identity banner, for L4 load-balancing demos.
package tcpecho

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"

	"github.com/rlnorthcutt/reflector/internal/identity"
)

const readBufferSize = 4096

// Listener runs a TCP echo server: bytes in, bytes out, connection stays
// open until the client closes it or it goes idle past IdleTimeout.
type Listener struct {
	Banner      bool
	IdleTimeout time.Duration
	Logger      *slog.Logger
}

// bannerLine builds the identity line sent on connect when Banner is
// enabled, e.g. "reflector host=web-2 instance=a1b2c3 port=9000".
func bannerLine(id identity.Identity, port int) string {
	return id.Banner(port) + "\n"
}

// Serve accepts connections on ln until ctx is cancelled, echoing bytes on
// each. It blocks until the listener is closed and all connections have
// been dispatched.
func (l *Listener) Serve(ctx context.Context, ln net.Listener, id identity.Identity, port int) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	var banner string
	if l.Banner {
		banner = bannerLine(id, port)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go l.handleConn(conn, banner)
	}
}

func (l *Listener) handleConn(conn net.Conn, banner string) {
	defer conn.Close()

	if banner != "" {
		if _, err := conn.Write([]byte(banner)); err != nil {
			return
		}
	}

	buf := make([]byte, readBufferSize)
	for {
		if err := conn.SetReadDeadline(time.Now().Add(l.IdleTimeout)); err != nil {
			return
		}
		n, err := conn.Read(buf)
		if n > 0 {
			if _, werr := conn.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
