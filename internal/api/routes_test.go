// internal/api/routes_test.go
package api

import (
	"testing"
	"time"
)

func TestRouteRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRouteRegistry(30 * time.Second)

	err := r.Register("myapp", "localhost:3000", "/path/to/project")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	route, ok := r.Lookup("myapp")
	if !ok {
		t.Fatal("Lookup failed")
	}

	if route.Name != "myapp" {
		t.Errorf("expected myapp, got %s", route.Name)
	}
	if route.Upstream != "localhost:3000" {
		t.Errorf("expected localhost:3000, got %s", route.Upstream)
	}
}

func TestRouteRegistry_ConflictFromSameDir(t *testing.T) {
	r := NewRouteRegistry(30 * time.Second)

	err := r.Register("myapp", "localhost:3000", "/path/to/project")
	if err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	// Same name, same dir = error
	err = r.Register("myapp", "localhost:4000", "/path/to/project")
	if err == nil {
		t.Fatal("expected error for conflict from same dir")
	}
}

func TestRouteRegistry_ConflictFromDifferentDir(t *testing.T) {
	r := NewRouteRegistry(30 * time.Second)

	err := r.Register("myapp", "localhost:3000", "/path/to/project1")
	if err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	// Same name, different dir = returns conflict info
	err = r.Register("myapp", "localhost:4000", "/path/to/project2")
	if err == nil {
		t.Fatal("expected error for conflict")
	}

	conflict, ok := err.(*ConflictError)
	if !ok {
		t.Fatalf("expected ConflictError, got %T", err)
	}
	if conflict.ExistingDir != "/path/to/project1" {
		t.Errorf("unexpected existing dir: %s", conflict.ExistingDir)
	}
}

func TestRouteRegistry_Heartbeat(t *testing.T) {
	r := NewRouteRegistry(100 * time.Millisecond)

	err := r.Register("myapp", "localhost:3000", "/path")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Heartbeat should succeed
	err = r.Heartbeat("myapp")
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)
	r.Cleanup()

	_, ok := r.Lookup("myapp")
	if ok {
		t.Error("expected route to be expired")
	}
}
