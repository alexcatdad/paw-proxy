package launchd

import "testing"

func TestActivateSocket_FallbackWhenNotLaunchdManaged(t *testing.T) {
	// When not launched by launchd (or on non-macOS), ActivateSocket
	// should return (nil, false, nil) to signal fallback to direct binding.
	listener, activated, err := ActivateSocket("http")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if activated {
		t.Fatal("expected activated=false when not launched by launchd")
	}
	if listener != nil {
		listener.Close()
		t.Fatal("expected nil listener when not launched by launchd")
	}
}

func TestActivateSocket_UnknownSocketName(t *testing.T) {
	// Even with a bogus socket name, the function should gracefully
	// fall back rather than error (not launched by launchd).
	listener, activated, err := ActivateSocket("nonexistent_socket_xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if activated {
		t.Fatal("expected activated=false for unknown socket name")
	}
	if listener != nil {
		listener.Close()
		t.Fatal("expected nil listener for unknown socket name")
	}
}
