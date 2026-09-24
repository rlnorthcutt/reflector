package httpserver

import (
	"net/http"
	"testing"
)

func TestFileRouteReturnsExactByteCountAndType(t *testing.T) {
	cases := []struct {
		ext         string
		wantContent string
	}{
		{"jpg", "image/jpeg"},
		{"JPG", "image/jpeg"}, // extension lookup is case-insensitive
		{"pdf", "application/pdf"},
		{"docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"txt", "text/plain"},
		{"svg", "image/svg+xml"},
		{"xyz", "application/octet-stream"}, // unmapped but valid extension
	}
	for _, c := range cases {
		t.Run(c.ext, func(t *testing.T) {
			s := testServer()
			w := doRequest(t, s.builtinMux(), http.MethodGet, "/file/1024."+c.ext, false)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			if len(w.Body.Bytes()) != 1024 {
				t.Errorf("len(body) = %d, want 1024", len(w.Body.Bytes()))
			}
			if got := w.Header().Get("Content-Type"); got != c.wantContent {
				t.Errorf("Content-Type = %q, want %q", got, c.wantContent)
			}
			wantDisposition := `attachment; filename="1024.` + c.ext + `"`
			if got := w.Header().Get("Content-Disposition"); got != wantDisposition {
				t.Errorf("Content-Disposition = %q, want %q", got, wantDisposition)
			}
		})
	}
}

func TestFileRouteContentIsZeroFilled(t *testing.T) {
	s := testServer()
	w := doRequest(t, s.builtinMux(), http.MethodGet, "/file/256.bin", false)
	for i, b := range w.Body.Bytes() {
		if b != 0 {
			t.Fatalf("byte %d = %d, want 0", i, b)
		}
	}
}

func TestFileRouteRejectsMissingExtension(t *testing.T) {
	s := testServer()
	w := doRequest(t, s.builtinMux(), http.MethodGet, "/file/1024", false)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestFileRouteRejectsInvalidExtension(t *testing.T) {
	s := testServer()
	w := doRequest(t, s.builtinMux(), http.MethodGet, "/file/1024.j%20g", false)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestFileRouteRejectsInvalidByteCount(t *testing.T) {
	s := testServer()
	w := doRequest(t, s.builtinMux(), http.MethodGet, "/file/abc.jpg", false)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestFileRouteRejectsTooLarge(t *testing.T) {
	s := testServer()
	w := doRequest(t, s.builtinMux(), http.MethodGet, "/file/99999999999999999999.jpg", false)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestBuiltinPatternsIncludesFile(t *testing.T) {
	for _, p := range BuiltinPatterns() {
		if p == "/file/{bytes}.{ext}" {
			return
		}
	}
	t.Errorf("BuiltinPatterns() = %v, want it to include /file/{bytes}.{ext}", BuiltinPatterns())
}
