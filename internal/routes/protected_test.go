package routes

import "testing"

func TestIsProtectedPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/healthz", true},
		{"/readyz", true},
		{"/ws", true},
		{"/admin", true},
		{"/admin/health/down", true},
		{"/", false},
		{"/echo", false},
		{"/wsx", false},
		{"/admins", false},
	}
	for _, c := range cases {
		if got := IsProtectedPath(c.path); got != c.want {
			t.Errorf("IsProtectedPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
