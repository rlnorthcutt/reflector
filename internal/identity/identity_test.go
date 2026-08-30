package identity

import "testing"

func TestNewUsesOSHostnameByDefault(t *testing.T) {
	id := New("", 8080)
	if id.Hostname == "" {
		t.Error("expected non-empty hostname")
	}
	if id.Port != 8080 {
		t.Errorf("Port = %d, want 8080", id.Port)
	}
	if id.Version != Version {
		t.Errorf("Version = %q, want %q", id.Version, Version)
	}
}

func TestNewHonorsHostnameOverride(t *testing.T) {
	id := New("custom-host", 9090)
	if id.Hostname != "custom-host" {
		t.Errorf("Hostname = %q, want %q", id.Hostname, "custom-host")
	}
}

func TestNewInstanceIDIsSixHexChars(t *testing.T) {
	id := New("", 8080)
	if len(id.InstanceID) != 6 {
		t.Errorf("InstanceID = %q, want length 6", id.InstanceID)
	}
	for _, c := range id.InstanceID {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("InstanceID %q contains non-hex character %q", id.InstanceID, c)
		}
	}
}

func TestNewInstanceIDsAreUnique(t *testing.T) {
	a := New("", 8080)
	b := New("", 8080)
	if a.InstanceID == b.InstanceID {
		t.Errorf("expected different instance IDs, got %q twice", a.InstanceID)
	}
}
