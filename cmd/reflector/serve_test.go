package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/rlnorthcutt/reflector/internal/adminserver"
	"github.com/rlnorthcutt/reflector/internal/envelope"
)

// freePort asks the OS for an ephemeral port and immediately releases it,
// so runServeCtx can bind it moments later. There is a theoretical race if
// something else grabs the port first, but it's the standard, good-enough
// way to test against a real listener without hardcoding ports.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// waitForReady polls url until it accepts connections, or fails fast if
// errCh receives runServeCtx's return value first (e.g. a bind error) so
// a startup failure reports its real cause instead of a generic timeout.
func waitForReady(t *testing.T, errCh <-chan error, url string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			t.Fatalf("server exited before becoming ready: %v", err)
		default:
		}
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become ready: %v", url, lastErr)
}

// TestRunServeEndToEnd starts the real server via runServeCtx (the same
// path main() uses) on real ports, exercises the HTTP and admin surfaces
// over the wire, and confirms context cancellation shuts it down cleanly.
// It's the one test that proves the wiring in runServe itself is correct,
// as opposed to the individual components it assembles.
func TestRunServeEndToEnd(t *testing.T) {
	chdir(t, t.TempDir())

	httpPort := freePort(t)
	adminPort := freePort(t)
	httpBase := fmt.Sprintf("http://127.0.0.1:%d", httpPort)
	adminBase := fmt.Sprintf("http://127.0.0.1:%d", adminPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runServeCtx(ctx, []string{
			"--host", "127.0.0.1",
			"--port", fmt.Sprintf("%d", httpPort),
			"--admin-port", fmt.Sprintf("%d", adminPort),
			"--hostname-override", "test-instance",
			"--preset", "rest-api",
			"--quiet",
		})
	}()

	waitForReady(t, errCh, httpBase+"/healthz")
	waitForReady(t, errCh, adminBase+"/admin/routes")

	t.Run("root envelope reports identity", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, httpBase+"/", nil)
		if err != nil {
			t.Fatalf("building request: %v", err)
		}
		req.Header.Set("Accept", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /: %v", err)
		}
		defer resp.Body.Close()

		var env envelope.Envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decoding envelope: %v", err)
		}
		if env.Server.Hostname != "test-instance" {
			t.Errorf("hostname = %q, want %q", env.Server.Hostname, "test-instance")
		}
		if env.Server.Port != httpPort {
			t.Errorf("port = %d, want %d", env.Server.Port, httpPort)
		}
	})

	t.Run("healthz starts healthy", func(t *testing.T) {
		resp, err := http.Get(httpBase + "/healthz")
		if err != nil {
			t.Fatalf("GET /healthz: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("admin health toggle propagates to healthz", func(t *testing.T) {
		resp, err := http.Post(adminBase+"/admin/health/down", "", nil)
		if err != nil {
			t.Fatalf("POST /admin/health/down: %v", err)
		}
		resp.Body.Close()

		resp, err = http.Get(httpBase + "/healthz")
		if err != nil {
			t.Fatalf("GET /healthz: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("status after health/down = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
		}

		resp, err = http.Post(adminBase+"/admin/health/up", "", nil)
		if err != nil {
			t.Fatalf("POST /admin/health/up: %v", err)
		}
		resp.Body.Close()
	})

	t.Run("preset route is live", func(t *testing.T) {
		resp, err := http.Get(httpBase + "/api/users")
		if err != nil {
			t.Fatalf("GET /api/users: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("admin routes lists the loaded preset", func(t *testing.T) {
		resp, err := http.Get(adminBase + "/admin/routes")
		if err != nil {
			t.Fatalf("GET /admin/routes: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var list []adminserver.RouteInfo
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			t.Fatalf("decoding route list: %v", err)
		}
		if len(list) == 0 {
			t.Fatal("route list is empty, want the rest-api preset's routes")
		}
		var found bool
		for _, r := range list {
			if r.Path == "/api/users" && r.Source == "preset:rest-api" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("route list %+v does not contain /api/users from preset:rest-api", list)
		}
	})

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("runServeCtx returned error after shutdown: %v", err)
		}
	case <-time.After(shutdownGracePeriod + 5*time.Second):
		t.Fatal("runServeCtx did not return after context cancellation")
	}

	// Ports should be free again post-shutdown.
	if l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", httpPort)); err != nil {
		t.Errorf("http port not released after shutdown: %v", err)
	} else {
		l.Close()
	}
}
