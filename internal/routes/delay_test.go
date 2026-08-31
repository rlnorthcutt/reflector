package routes

import (
	"testing"
	"time"
)

func TestParseDelayEmptyReturnsNil(t *testing.T) {
	d, err := ParseDelay("")
	if err != nil || d != nil {
		t.Fatalf("ParseDelay(\"\") = %v, %v; want nil, nil", d, err)
	}
}

func TestParseDelayFixed(t *testing.T) {
	d, err := ParseDelay("150ms")
	if err != nil {
		t.Fatalf("ParseDelay: %v", err)
	}
	if d.Base != 150*time.Millisecond || d.Jitter != 0 {
		t.Errorf("d = %+v, want Base=150ms Jitter=0", d)
	}
	if got := d.Sample(); got != 150*time.Millisecond {
		t.Errorf("Sample() = %v, want 150ms", got)
	}
}

func TestParseDelayWithJitter(t *testing.T) {
	d, err := ParseDelay("800ms±400ms")
	if err != nil {
		t.Fatalf("ParseDelay: %v", err)
	}
	if d.Base != 800*time.Millisecond || d.Jitter != 400*time.Millisecond {
		t.Errorf("d = %+v, want Base=800ms Jitter=400ms", d)
	}
	for i := 0; i < 50; i++ {
		got := d.Sample()
		if got < 400*time.Millisecond || got > 1200*time.Millisecond {
			t.Fatalf("Sample() = %v, want within [400ms, 1200ms]", got)
		}
	}
}

func TestParseDelayJitterNeverNegative(t *testing.T) {
	d, err := ParseDelay("100ms±500ms")
	if err != nil {
		t.Fatalf("ParseDelay: %v", err)
	}
	for i := 0; i < 50; i++ {
		if got := d.Sample(); got < 0 {
			t.Fatalf("Sample() = %v, want >= 0", got)
		}
	}
}

func TestParseDelayRejectsInvalid(t *testing.T) {
	for _, s := range []string{"notaduration", "-5ms", "5ms±notaduration", "5ms±-1ms"} {
		if _, err := ParseDelay(s); err == nil {
			t.Errorf("ParseDelay(%q): expected error", s)
		}
	}
}

func TestNilDelaySampleIsZero(t *testing.T) {
	var d *Delay
	if got := d.Sample(); got != 0 {
		t.Errorf("nil Delay.Sample() = %v, want 0", got)
	}
}
