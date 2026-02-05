// internal/api/routes.go
package api

import (
	"fmt"
	"sync"
	"time"
)

type Route struct {
	Name          string    `json:"name"`
	Upstream      string    `json:"upstream"`
	Dir           string    `json:"dir"`
	Registered    time.Time `json:"registered"`
	LastHeartbeat time.Time `json:"lastHeartbeat"`
}

type ConflictError struct {
	Name        string
	ExistingDir string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("route %q already registered from %s", e.Name, e.ExistingDir)
}

type RouteRegistry struct {
	routes  map[string]*Route
	timeout time.Duration
	mu      sync.RWMutex
}

func NewRouteRegistry(timeout time.Duration) *RouteRegistry {
	return &RouteRegistry{
		routes:  make(map[string]*Route),
		timeout: timeout,
	}
}

func (r *RouteRegistry) Register(name, upstream, dir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.routes[name]; ok {
		return &ConflictError{
			Name:        name,
			ExistingDir: existing.Dir,
		}
	}

	now := time.Now()
	r.routes[name] = &Route{
		Name:          name,
		Upstream:      upstream,
		Dir:           dir,
		Registered:    now,
		LastHeartbeat: now,
	}

	return nil
}

func (r *RouteRegistry) Deregister(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.routes[name]; ok {
		delete(r.routes, name)
		return true
	}
	return false
}

// Lookup returns a copy of the route with the given name.
// Returning a copy prevents callers from mutating registry-owned data.
func (r *RouteRegistry) Lookup(name string) (Route, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	route, ok := r.routes[name]
	if !ok {
		return Route{}, false
	}
	return *route, true
}

// ExtractName extracts the route name from a host string like
// "myapp.test" or "myapp.test:443", returning just "myapp".
func ExtractName(host string) string {
	for i, c := range host {
		if c == '.' || c == ':' {
			return host[:i]
		}
	}
	return host
}

// LookupByHost extracts the route name from a host string and looks it up.
func (r *RouteRegistry) LookupByHost(host string) (Route, bool) {
	return r.Lookup(ExtractName(host))
}

func (r *RouteRegistry) Heartbeat(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	route, ok := r.routes[name]
	if !ok {
		return fmt.Errorf("route %q not found", name)
	}

	route.LastHeartbeat = time.Now()
	return nil
}

// Cleanup removes routes whose heartbeat has expired. It uses a read-lock
// to scan for expired routes, then upgrades to a write-lock only if
// deletions are needed, reducing contention on the hot path.
func (r *RouteRegistry) Cleanup() {
	r.mu.RLock()
	cutoff := time.Now().Add(-r.timeout)
	var expired []string
	for name, route := range r.routes {
		if route.LastHeartbeat.Before(cutoff) {
			expired = append(expired, name)
		}
	}
	r.mu.RUnlock()

	if len(expired) == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, name := range expired {
		// Re-check under write lock in case a heartbeat arrived between
		// releasing the read lock and acquiring the write lock.
		if route, ok := r.routes[name]; ok && route.LastHeartbeat.Before(cutoff) {
			delete(r.routes, name)
		}
	}
}

// List returns copies of all registered routes.
func (r *RouteRegistry) List() []Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routes := make([]Route, 0, len(r.routes))
	for _, route := range r.routes {
		routes = append(routes, *route)
	}
	return routes
}
