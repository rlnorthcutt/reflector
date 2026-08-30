package envelope

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rlnorthcutt/reflector/internal/identity"
)

func testIdentity() identity.Identity {
	return identity.Identity{
		Hostname:   "web-2",
		InstanceID: "a1b2c3",
		Port:       8080,
		Version:    "0.3.1",
	}
}

func TestBuildPopulatesServerAndRequest(t *testing.T) {
	r := httptest.NewRequest("GET", "/anything/orders/42?verbose=1", strings.NewReader("hello"))
	r.RemoteAddr = "10.0.0.5:55302"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")

	env := Build(r, testIdentity())

	if env.Server.Hostname != "web-2" || env.Server.InstanceID != "a1b2c3" || env.Server.Port != 8080 {
		t.Errorf("Server = %+v, unexpected values", env.Server)
	}
	if env.Request.Method != "GET" {
		t.Errorf("Method = %q, want GET", env.Request.Method)
	}
	if env.Request.Path != "/anything/orders/42" {
		t.Errorf("Path = %q, want /anything/orders/42", env.Request.Path)
	}
	if env.Request.Body != "hello" {
		t.Errorf("Body = %q, want %q", env.Request.Body, "hello")
	}
	if got := env.Request.Headers["X-Forwarded-For"]; len(got) != 1 || got[0] != "203.0.113.9" {
		t.Errorf("Headers[X-Forwarded-For] = %v", got)
	}
	if got := env.Request.Query["verbose"]; len(got) != 1 || got[0] != "1" {
		t.Errorf("Query[verbose] = %v", got)
	}
	if env.Request.Client.IP != "10.0.0.5" || env.Request.Client.Port != 55302 {
		t.Errorf("Client = %+v, want IP=10.0.0.5 Port=55302", env.Request.Client)
	}
	if env.Request.TLS.Enabled {
		t.Error("TLS.Enabled = true, want false for a plaintext request")
	}
	if env.Timestamp == "" {
		t.Error("Timestamp is empty")
	}
}

func TestBuildTruncatesOversizedBody(t *testing.T) {
	big := strings.Repeat("x", MaxBodyBytes+100)
	r := httptest.NewRequest("POST", "/echo", strings.NewReader(big))

	env := Build(r, testIdentity())

	if len(env.Request.Body) != MaxBodyBytes {
		t.Errorf("len(Body) = %d, want %d", len(env.Request.Body), MaxBodyBytes)
	}
}

func TestPlainTextIncludesKeyFields(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"

	text := Build(r, testIdentity()).PlainText()

	for _, want := range []string{"Hostname: web-2", "InstanceID: a1b2c3", "Port: 8080", "Method: GET", "Path: /"} {
		if !strings.Contains(text, want) {
			t.Errorf("PlainText() missing %q, got:\n%s", want, text)
		}
	}
}

func TestClientInfoHandlesMissingPort(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "not-a-valid-addr"

	c := ClientInfo(r)
	if c.IP != "not-a-valid-addr" {
		t.Errorf("IP = %q, want fallback to raw RemoteAddr", c.IP)
	}
}
