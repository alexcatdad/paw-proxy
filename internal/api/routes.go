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

func (r *RouteRegistry) Lookup(name string) (*Route, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	route, ok := r.routes[name]
	return route, ok
}

func (r *RouteRegistry) LookupByHost(host string) (*Route, bool) {
	// host is like "myapp.test" or "myapp.test:443"
	// Extract just the name part
	name := host
	for i, c := range host {
		if c == '.' || c == ':' {
			name = host[:i]
			break
		}
	}

	return r.Lookup(name)
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

func (r *RouteRegistry) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-r.timeout)
	for name, route := range r.routes {
		if route.LastHeartbeat.Before(cutoff) {
			delete(r.routes, name)
		}
	}
}

func (r *RouteRegistry) List() []*Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routes := make([]*Route, 0, len(r.routes))
	for _, route := range r.routes {
		routes = append(routes, route)
	}
	return routes
}
