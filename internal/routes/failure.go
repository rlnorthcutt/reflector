package routes

import (
	"fmt"
	"math/rand"
)

// Failure injects a probability of returning a different status instead
// of the route's normal response.
type Failure struct {
	Rate   float64
	Status int
}

func newFailure(raw *rawFailure) (*Failure, error) {
	if raw == nil {
		return nil, nil
	}
	if raw.Rate < 0 || raw.Rate > 1 {
		return nil, fmt.Errorf("failure.rate %v must be between 0 and 1", raw.Rate)
	}
	status := raw.Status
	if status == 0 {
		status = 503
	}
	return &Failure{Rate: raw.Rate, Status: status}, nil
}

// Triggered rolls the dice for this request, returning true if the
// simulated failure should fire.
func (f *Failure) Triggered() bool {
	if f == nil || f.Rate <= 0 {
		return false
	}
	return rand.Float64() < f.Rate
}
