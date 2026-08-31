package routes

import "testing"

func TestNewFailureNilRaw(t *testing.T) {
	f, err := newFailure(nil)
	if err != nil || f != nil {
		t.Fatalf("newFailure(nil) = %v, %v; want nil, nil", f, err)
	}
}

func TestNewFailureDefaultsStatus(t *testing.T) {
	f, err := newFailure(&rawFailure{Rate: 0.5})
	if err != nil {
		t.Fatalf("newFailure: %v", err)
	}
	if f.Status != 503 {
		t.Errorf("Status = %d, want 503 (default)", f.Status)
	}
}

func TestNewFailureRejectsOutOfRangeRate(t *testing.T) {
	for _, rate := range []float64{-0.1, 1.1} {
		if _, err := newFailure(&rawFailure{Rate: rate}); err == nil {
			t.Errorf("rate %v: expected error", rate)
		}
	}
}

func TestFailureTriggeredRateZeroNeverFires(t *testing.T) {
	f := &Failure{Rate: 0, Status: 503}
	for i := 0; i < 100; i++ {
		if f.Triggered() {
			t.Fatal("Triggered() = true with rate 0")
		}
	}
}

func TestFailureTriggeredRateOneAlwaysFires(t *testing.T) {
	f := &Failure{Rate: 1, Status: 503}
	for i := 0; i < 100; i++ {
		if !f.Triggered() {
			t.Fatal("Triggered() = false with rate 1")
		}
	}
}

func TestNilFailureNeverTriggers(t *testing.T) {
	var f *Failure
	if f.Triggered() {
		t.Error("nil Failure.Triggered() = true")
	}
}
