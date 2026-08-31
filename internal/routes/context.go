package routes

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rlnorthcutt/reflector/internal/identity"
)

// Context is the value exposed to a route's body template.
type Context struct {
	Method     string
	Path       string
	Body       string
	Hostname   string
	InstanceID string
	Port       int
	Timestamp  string

	req *http.Request
}

// NewContext builds a template Context for the given request.
func NewContext(r *http.Request, body string, id identity.Identity) *Context {
	return &Context{
		Method:     r.Method,
		Path:       r.URL.Path,
		Body:       body,
		Hostname:   id.Hostname,
		InstanceID: id.InstanceID,
		Port:       id.Port,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		req:        r,
	}
}

// PathParam returns a named path parameter, e.g. {{ .PathParam "id" }}.
func (c *Context) PathParam(name string) string {
	return c.req.PathValue(name)
}

// QueryParam returns the first value of a query parameter.
func (c *Context) QueryParam(name string) string {
	return c.req.URL.Query().Get(name)
}

// Header returns a request header value.
func (c *Context) Header(name string) string {
	return c.req.Header.Get(name)
}

// BodyJSON best-effort parses the request body as JSON, returning nil if
// it isn't valid JSON.
func (c *Context) BodyJSON() any {
	var v any
	if err := json.Unmarshal([]byte(c.Body), &v); err != nil {
		return nil
	}
	return v
}
