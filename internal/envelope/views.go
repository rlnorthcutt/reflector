package envelope

import (
	"fmt"
	"strings"
)

// HeadersView is the response shape for /headers: just the headers section
// of the envelope.
type HeadersView struct {
	Headers map[string][]string `json:"headers"`
}

func (v HeadersView) PlainText() string {
	var b strings.Builder
	for _, k := range sortedKeys(v.Headers) {
		for _, val := range v.Headers[k] {
			fmt.Fprintf(&b, "%s: %s\n", k, val)
		}
	}
	return b.String()
}

// IPView is the response shape for /ip: just the client IP.
type IPView struct {
	IP string `json:"ip"`
}

func (v IPView) PlainText() string {
	return v.IP + "\n"
}

// UserAgentView is the response shape for /user-agent: just the User-Agent
// header value.
type UserAgentView struct {
	UserAgent string `json:"user_agent"`
}

func (v UserAgentView) PlainText() string {
	return v.UserAgent + "\n"
}
