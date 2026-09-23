package routes

import "strings"

// IsProtectedPath reports whether path is reserved for built-ins that user
// and preset routes may never shadow, regardless of --strict-builtins.
func IsProtectedPath(path string) bool {
	switch path {
	case "/healthz", "/readyz", "/ws", "/admin":
		return true
	}
	return strings.HasPrefix(path, "/admin/")
}
