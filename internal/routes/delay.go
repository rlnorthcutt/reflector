package routes

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// Delay describes a route's response latency: a fixed base with optional
// uniform jitter, parsed from strings like "150ms" or "800ms±400ms".
type Delay struct {
	Base   time.Duration
	Jitter time.Duration
}

// ParseDelay parses a duration string with an optional "±jitter" suffix.
func ParseDelay(s string) (*Delay, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	parts := strings.SplitN(s, "±", 2)
	base, err := time.ParseDuration(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("invalid delay %q: %w", s, err)
	}
	if base < 0 {
		return nil, fmt.Errorf("invalid delay %q: must not be negative", s)
	}

	d := &Delay{Base: base}
	if len(parts) == 2 {
		jitter, err := time.ParseDuration(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid delay jitter %q: %w", s, err)
		}
		if jitter < 0 {
			return nil, fmt.Errorf("invalid delay jitter %q: must not be negative", s)
		}
		d.Jitter = jitter
	}
	return d, nil
}

// Sample returns a duration to actually wait: Base if there's no jitter,
// otherwise a value uniformly distributed in [Base-Jitter, Base+Jitter]
// (clamped at zero).
func (d *Delay) Sample() time.Duration {
	if d == nil {
		return 0
	}
	if d.Jitter <= 0 {
		return d.Base
	}
	span := int64(2*d.Jitter) + 1
	offset := time.Duration(rand.Int63n(span)) - d.Jitter
	v := d.Base + offset
	if v < 0 {
		v = 0
	}
	return v
}
