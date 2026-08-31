package capture

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBufferFIFOEviction(t *testing.T) {
	b := NewBuffer(2)
	b.Add(Entry{Path: "/a"})
	b.Add(Entry{Path: "/b"})
	b.Add(Entry{Path: "/c"})

	all := b.All()
	if len(all) != 2 {
		t.Fatalf("len(All()) = %d, want 2", len(all))
	}
	if all[0].Path != "/b" || all[1].Path != "/c" {
		t.Errorf("all = %+v, want [/b, /c] (oldest evicted)", all)
	}
}

func TestBufferClear(t *testing.T) {
	b := NewBuffer(10)
	b.Add(Entry{Path: "/a"})
	b.Clear()
	if len(b.All()) != 0 {
		t.Error("expected empty buffer after Clear()")
	}
}

func TestNewBufferClampsMinimum(t *testing.T) {
	b := NewBuffer(0)
	b.Add(Entry{Path: "/a"})
	b.Add(Entry{Path: "/b"})
	if len(b.All()) != 1 {
		t.Errorf("len(All()) = %d, want 1 (max clamped to 1)", len(b.All()))
	}
}

func TestMiddlewareRecordsRequestAndPreservesBody(t *testing.T) {
	buf := NewBuffer(10)
	var gotBody string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
	})

	handler := Middleware(buf, inner)
	req := httptest.NewRequest(http.MethodPost, "/echo?x=1", strings.NewReader("hello"))
	req.Header.Set("X-Test", "abc")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if gotBody != "hello" {
		t.Errorf("downstream body = %q, want %q (must be preserved)", gotBody, "hello")
	}

	all := buf.All()
	if len(all) != 1 {
		t.Fatalf("len(All()) = %d, want 1", len(all))
	}
	e := all[0]
	if e.Method != http.MethodPost || e.Path != "/echo" || e.Body != "hello" {
		t.Errorf("entry = %+v, unexpected fields", e)
	}
	if got := e.Headers["X-Test"]; len(got) != 1 || got[0] != "abc" {
		t.Errorf("Headers[X-Test] = %v", got)
	}
	if e.Truncated {
		t.Error("Truncated = true for a small body")
	}
}

func TestMiddlewareTruncatesOversizedBody(t *testing.T) {
	buf := NewBuffer(10)
	big := strings.Repeat("x", MaxBodyBytes+100)
	var gotLen int
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotLen = len(b)
	})

	handler := Middleware(buf, inner)
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(big))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if gotLen != len(big) {
		t.Errorf("downstream saw %d bytes, want %d (full body preserved)", gotLen, len(big))
	}

	e := buf.All()[0]
	if !e.Truncated {
		t.Error("Truncated = false for an oversized body")
	}
	if len(e.Body) != MaxBodyBytes {
		t.Errorf("len(captured Body) = %d, want %d", len(e.Body), MaxBodyBytes)
	}
}

func TestMiddlewareNilBufferIsNoop(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	handler := Middleware(nil, inner)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if !called {
		t.Error("inner handler was not called when capture is disabled")
	}
}
