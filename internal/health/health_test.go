package health

import (
	"sync"
	"testing"
)

func TestNewStartsHealthy(t *testing.T) {
	s := New()
	if !s.Healthy() {
		t.Error("New() should start healthy")
	}
}

func TestSetHealthyToggles(t *testing.T) {
	s := New()
	s.SetHealthy(false)
	if s.Healthy() {
		t.Error("Healthy() = true after SetHealthy(false)")
	}
	s.SetHealthy(true)
	if !s.Healthy() {
		t.Error("Healthy() = false after SetHealthy(true)")
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(down bool) {
			defer wg.Done()
			s.SetHealthy(!down)
		}(i%2 == 0)
		go func() {
			defer wg.Done()
			_ = s.Healthy()
		}()
	}
	wg.Wait()
}
