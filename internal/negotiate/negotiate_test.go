package negotiate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWantsJSON(t *testing.T) {
	tests := []struct {
		name   string
		accept string
		want   bool
	}{
		{"no accept header", "", false},
		{"bare wildcard (curl default)", "*/*", false},
		{"explicit json", "application/json", true},
		{"json with quality params", "application/json;q=0.9", true},
		{"json among multiple", "text/html, application/json", true},
		{"unrelated type", "text/html", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.accept != "" {
				r.Header.Set("Accept", tt.accept)
			}
			if got := WantsJSON(r); got != tt.want {
				t.Errorf("WantsJSON(Accept=%q) = %v, want %v", tt.accept, got, tt.want)
			}
		})
	}
}

type fakeView struct {
	Value string `json:"value"`
}

func (f fakeView) PlainText() string { return "value: " + f.Value + "\n" }

func TestWritePlaintextByDefault(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	Write(w, r, http.StatusOK, fakeView{Value: "x"})

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain prefix", ct)
	}
	if w.Body.String() != "value: x\n" {
		t.Errorf("body = %q, want %q", w.Body.String(), "value: x\n")
	}
}

func TestWriteJSONWhenRequested(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()

	Write(w, r, http.StatusCreated, fakeView{Value: "x"})

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got fakeView
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got.Value != "x" {
		t.Errorf("Value = %q, want %q", got.Value, "x")
	}
}
