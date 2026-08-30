// Package envelope builds the response envelope that every built-in
// inspection route returns: server identity, request details, and a
// timestamp, renderable as JSON or as whoami-style plaintext.
package envelope

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rlnorthcutt/reflector/internal/identity"
)

// MaxBodyBytes bounds how much of a request body Build will read, so a
// client cannot force unbounded memory use via a huge request body.
const MaxBodyBytes = 1 << 20 // 1 MiB

// Server identifies the reflector instance serving the response.
type Server struct {
	Hostname   string `json:"hostname"`
	InstanceID string `json:"instance_id"`
	Port       int    `json:"port"`
	Version    string `json:"version"`
}

// Client identifies the peer that made the request.
type Client struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// TLS describes the TLS connection state, if any.
type TLS struct {
	Enabled bool   `json:"enabled"`
	Version string `json:"version,omitempty"`
	SNI     string `json:"sni,omitempty"`
}

// Request captures the inbound request as reflected back to the client.
type Request struct {
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Proto   string              `json:"proto"`
	Headers map[string][]string `json:"headers"`
	Query   map[string][]string `json:"query"`
	Body    string              `json:"body"`
	Client  Client              `json:"client"`
	TLS     TLS                 `json:"tls"`
}

// Envelope is the response envelope built-in routes return.
type Envelope struct {
	Server    Server  `json:"server"`
	Request   Request `json:"request"`
	Timestamp string  `json:"timestamp"`
}

// Build constructs an Envelope from the current request and server
// identity. It reads up to MaxBodyBytes of the request body; the caller
// remains responsible for closing r.Body.
func Build(r *http.Request, id identity.Identity) Envelope {
	body, _ := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes))

	return Envelope{
		Server: Server{
			Hostname:   id.Hostname,
			InstanceID: id.InstanceID,
			Port:       id.Port,
			Version:    id.Version,
		},
		Request: Request{
			Method:  r.Method,
			Path:    r.URL.Path,
			Proto:   r.Proto,
			Headers: map[string][]string(r.Header),
			Query:   map[string][]string(r.URL.Query()),
			Body:    string(body),
			Client:  ClientInfo(r),
			TLS:     tlsInfo(r),
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// ClientInfo extracts the client IP and port from the request, for routes
// that need it without building a full Envelope.
func ClientInfo(r *http.Request) Client {
	host, portStr, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return Client{IP: r.RemoteAddr}
	}
	port, _ := strconv.Atoi(portStr)
	return Client{IP: host, Port: port}
}

func tlsInfo(r *http.Request) TLS {
	if r.TLS == nil {
		return TLS{Enabled: false}
	}
	return TLS{
		Enabled: true,
		Version: tlsVersionName(r.TLS.Version),
		SNI:     r.TLS.ServerName,
	}
}

func tlsVersionName(v uint16) string {
	switch v {
	case 0x0301:
		return "1.0"
	case 0x0302:
		return "1.1"
	case 0x0303:
		return "1.2"
	case 0x0304:
		return "1.3"
	default:
		return fmt.Sprintf("0x%04x", v)
	}
}

// PlainText renders the envelope as whoami-style "Key: value" lines.
func (e Envelope) PlainText() string {
	var b strings.Builder

	fmt.Fprintf(&b, "Hostname: %s\n", e.Server.Hostname)
	fmt.Fprintf(&b, "InstanceID: %s\n", e.Server.InstanceID)
	fmt.Fprintf(&b, "Port: %d\n", e.Server.Port)
	fmt.Fprintf(&b, "Version: %s\n", e.Server.Version)
	fmt.Fprintf(&b, "Method: %s\n", e.Request.Method)
	fmt.Fprintf(&b, "Path: %s\n", e.Request.Path)
	fmt.Fprintf(&b, "Proto: %s\n", e.Request.Proto)
	fmt.Fprintf(&b, "ClientIP: %s\n", e.Request.Client.IP)
	fmt.Fprintf(&b, "ClientPort: %d\n", e.Request.Client.Port)
	fmt.Fprintf(&b, "TLS: %t\n", e.Request.TLS.Enabled)

	for _, k := range sortedKeys(e.Request.Headers) {
		for _, v := range e.Request.Headers[k] {
			fmt.Fprintf(&b, "Header[%s]: %s\n", k, v)
		}
	}
	for _, k := range sortedKeys(e.Request.Query) {
		for _, v := range e.Request.Query[k] {
			fmt.Fprintf(&b, "Query[%s]: %s\n", k, v)
		}
	}
	if e.Request.Body != "" {
		fmt.Fprintf(&b, "Body: %s\n", e.Request.Body)
	}
	fmt.Fprintf(&b, "Timestamp: %s\n", e.Timestamp)

	return b.String()
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
