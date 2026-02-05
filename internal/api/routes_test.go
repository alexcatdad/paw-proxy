// internal/api/routes_test.go
package api

import (
	"fmt"
	"sync"
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

func TestRouteRegistry_LookupReturnsCopy(t *testing.T) {
	r := NewRouteRegistry(30 * time.Second)

	err := r.Register("myapp", "localhost:3000", "/path")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Get a copy and mutate it
	route, ok := r.Lookup("myapp")
	if !ok {
		t.Fatal("Lookup failed")
	}
	route.Upstream = "mutated:9999"

	// Original should be unchanged
	original, ok := r.Lookup("myapp")
	if !ok {
		t.Fatal("second Lookup failed")
	}
	if original.Upstream != "localhost:3000" {
		t.Errorf("mutation leaked: got upstream %q, want %q", original.Upstream, "localhost:3000")
	}
}

func TestRouteRegistry_ListReturnsCopies(t *testing.T) {
	r := NewRouteRegistry(30 * time.Second)

	err := r.Register("myapp", "localhost:3000", "/path")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	routes := r.List()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}

	// Mutate the returned copy
	routes[0].Upstream = "mutated:9999"

	// Original should be unchanged
	original, ok := r.Lookup("myapp")
	if !ok {
		t.Fatal("Lookup failed")
	}
	if original.Upstream != "localhost:3000" {
		t.Errorf("List mutation leaked: got upstream %q, want %q", original.Upstream, "localhost:3000")
	}
}

func TestRouteRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRouteRegistry(100 * time.Millisecond)

	// Pre-register some routes
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("app%d", i)
		err := r.Register(name, fmt.Sprintf("localhost:%d", 3000+i), "/path")
		if err != nil {
			t.Fatalf("Register %s failed: %v", name, err)
		}
	}

	var wg sync.WaitGroup
	done := make(chan struct{})

	// Concurrent Lookups
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := fmt.Sprintf("app%d", id%10)
			for {
				select {
				case <-done:
					return
				default:
					r.Lookup(name)
					r.LookupByHost(name + ".test:443")
				}
			}
		}(i)
	}

	// Concurrent Heartbeats
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := fmt.Sprintf("app%d", id%10)
			for {
				select {
				case <-done:
					return
				default:
					r.Heartbeat(name)
				}
			}
		}(i)
	}

	// Concurrent List
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				r.List()
			}
		}
	}()

	// Concurrent Cleanup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				r.Cleanup()
			}
		}
	}()

	// Let it run for a bit under -race
	time.Sleep(200 * time.Millisecond)
	close(done)
	wg.Wait()
}
