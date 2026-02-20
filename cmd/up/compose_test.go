package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDetectDockerCompose(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantDetected bool
		wantFlags    []string
		wantUpIdx    int
	}{
		{
			name:         "basic docker compose up",
			args:         []string{"docker", "compose", "up"},
			wantDetected: true,
			wantFlags:    nil,
			wantUpIdx:    2,
		},
		{
			name:         "with profile flag",
			args:         []string{"docker", "compose", "--profile", "frontend", "up"},
			wantDetected: true,
			wantFlags:    []string{"--profile", "frontend"},
			wantUpIdx:    4,
		},
		{
			name:         "with -f flag",
			args:         []string{"docker", "compose", "-f", "docker-compose.prod.yml", "up"},
			wantDetected: true,
			wantFlags:    []string{"-f", "docker-compose.prod.yml"},
			wantUpIdx:    4,
		},
		{
			name:         "with project-name",
			args:         []string{"docker", "compose", "--project-name", "myapp", "up"},
			wantDetected: true,
			wantFlags:    []string{"--project-name", "myapp"},
			wantUpIdx:    4,
		},
		{
			name:         "with up flags after",
			args:         []string{"docker", "compose", "up", "-d", "--build"},
			wantDetected: true,
			wantFlags:    nil,
			wantUpIdx:    2,
		},
		{
			name:         "multiple compose flags",
			args:         []string{"docker", "compose", "--profile", "dev", "-f", "compose.yml", "up", "-d"},
			wantDetected: true,
			wantFlags:    []string{"--profile", "dev", "-f", "compose.yml"},
			wantUpIdx:    6,
		},
		{
			name:         "not docker compose",
			args:         []string{"bun", "dev"},
			wantDetected: false,
		},
		{
			name:         "docker but not compose",
			args:         []string{"docker", "run", "nginx"},
			wantDetected: false,
		},
		{
			name:         "docker compose but not up",
			args:         []string{"docker", "compose", "down"},
			wantDetected: false,
		},
		{
			name:         "too few args",
			args:         []string{"docker"},
			wantDetected: false,
		},
		{
			name:         "empty args",
			args:         []string{},
			wantDetected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectDockerCompose(tt.args)
			if result.detected != tt.wantDetected {
				t.Errorf("detected = %v, want %v", result.detected, tt.wantDetected)
				return
			}
			if !tt.wantDetected {
				return
			}
			if len(result.composeFlags) != len(tt.wantFlags) {
				t.Errorf("composeFlags = %v, want %v", result.composeFlags, tt.wantFlags)
				return
			}
			for i := range tt.wantFlags {
				if result.composeFlags[i] != tt.wantFlags[i] {
					t.Errorf("composeFlags[%d] = %q, want %q", i, result.composeFlags[i], tt.wantFlags[i])
				}
			}
			if result.upIdx != tt.wantUpIdx {
				t.Errorf("upIdx = %d, want %d", result.upIdx, tt.wantUpIdx)
			}
		})
	}
}

func TestParseComposeConfig(t *testing.T) {
	t.Run("basic services with ports", func(t *testing.T) {
		configJSON := `{
			"name": "myapp",
			"services": {
				"frontend": {
					"ports": [
						{"published": "3000", "target": 3000, "protocol": "tcp"}
					]
				},
				"api": {
					"ports": [
						{"published": "8080", "target": 8080, "protocol": "tcp"}
					]
				},
				"db": {}
			}
		}`

		routes, projectName, err := parseComposeConfig([]byte(configJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if projectName != "myapp" {
			t.Errorf("projectName = %q, want %q", projectName, "myapp")
		}

		if len(routes) != 2 {
			t.Fatalf("got %d routes, want 2", len(routes))
		}

		// Build a map for order-independent checking
		routeMap := make(map[string]string)
		for _, r := range routes {
			routeMap[r.service] = r.publishedPort
		}

		if routeMap["frontend"] != "3000" {
			t.Errorf("frontend port = %q, want %q", routeMap["frontend"], "3000")
		}
		if routeMap["api"] != "8080" {
			t.Errorf("api port = %q, want %q", routeMap["api"], "8080")
		}
		if _, ok := routeMap["db"]; ok {
			t.Error("db should not be included (no ports)")
		}
	})

	t.Run("service with multiple ports uses first", func(t *testing.T) {
		configJSON := `{
			"name": "myapp",
			"services": {
				"web": {
					"ports": [
						{"published": "3000", "target": 3000, "protocol": "tcp"},
						{"published": "3001", "target": 3001, "protocol": "tcp"}
					]
				}
			}
		}`

		routes, _, err := parseComposeConfig([]byte(configJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(routes) != 1 {
			t.Fatalf("got %d routes, want 1", len(routes))
		}
		if routes[0].publishedPort != "3000" {
			t.Errorf("port = %q, want %q", routes[0].publishedPort, "3000")
		}
	})

	t.Run("empty services", func(t *testing.T) {
		configJSON := `{"name": "myapp", "services": {}}`

		routes, _, err := parseComposeConfig([]byte(configJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(routes) != 0 {
			t.Errorf("got %d routes, want 0", len(routes))
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, _, err := parseComposeConfig([]byte("not json"))
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("service with empty published port is skipped", func(t *testing.T) {
		configJSON := `{
			"name": "myapp",
			"services": {
				"internal": {
					"ports": [
						{"published": "", "target": 8080, "protocol": "tcp"}
					]
				}
			}
		}`

		routes, _, err := parseComposeConfig([]byte(configJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(routes) != 0 {
			t.Errorf("got %d routes, want 0 (empty published port)", len(routes))
		}
	})
}

func TestMultiRouteState(t *testing.T) {
	t.Run("snapshot returns copy of routes", func(t *testing.T) {
		state := newMultiRouteState([]composeRoute{
			{service: "frontend", routeName: "frontend.myapp", upstream: "localhost:3000"},
			{service: "api", routeName: "api.myapp", upstream: "localhost:8080"},
		}, "/tmp/project")

		routes, dir := state.Snapshot()
		if dir != "/tmp/project" {
			t.Errorf("dir = %q, want %q", dir, "/tmp/project")
		}
		if len(routes) != 2 {
			t.Fatalf("got %d routes, want 2", len(routes))
		}

		// Mutating returned slice should not affect state
		routes[0].routeName = "mutated"
		routes2, _ := state.Snapshot()
		if routes2[0].routeName == "mutated" {
			t.Error("mutation leaked through snapshot")
		}
	})

	t.Run("route names returns names in order", func(t *testing.T) {
		state := newMultiRouteState([]composeRoute{
			{service: "frontend", routeName: "frontend.myapp", upstream: "localhost:3000"},
			{service: "api", routeName: "api.myapp", upstream: "localhost:8080"},
		}, "/tmp/project")

		names := state.RouteNames()
		if len(names) != 2 {
			t.Fatalf("got %d names, want 2", len(names))
		}
		if names[0] != "frontend.myapp" || names[1] != "api.myapp" {
			t.Errorf("names = %v", names)
		}
	})
}

func TestBuildComposeRouteNames(t *testing.T) {
	tests := []struct {
		name        string
		services    []discoveredService
		projectName string
		nameFlag    string
		wantNames   map[string]string // service → expected route name
	}{
		{
			name:        "empty services",
			services:    []discoveredService{},
			projectName: "myapp",
			wantNames:   map[string]string{},
		},
		{
			name: "basic naming",
			services: []discoveredService{
				{service: "frontend", publishedPort: "3000"},
				{service: "api", publishedPort: "8080"},
			},
			projectName: "myapp",
			wantNames: map[string]string{
				"frontend": "frontend.myapp",
				"api":      "api.myapp",
			},
		},
		{
			name: "name flag overrides project",
			services: []discoveredService{
				{service: "frontend", publishedPort: "3000"},
			},
			projectName: "myapp",
			nameFlag:    "shop",
			wantNames: map[string]string{
				"frontend": "frontend.shop",
			},
		},
		{
			name: "sanitizes project name",
			services: []discoveredService{
				{service: "frontend", publishedPort: "3000"},
			},
			projectName: "My Cool App",
			wantNames: map[string]string{
				"frontend": "frontend.my-cool-app",
			},
		},
		{
			name: "sanitizes service name",
			services: []discoveredService{
				{service: "my_service", publishedPort: "3000"},
			},
			projectName: "myapp",
			wantNames: map[string]string{
				"my_service": "my-service.myapp",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes := buildComposeRouteNames(tt.services, tt.projectName, tt.nameFlag)
			if len(routes) != len(tt.wantNames) {
				t.Fatalf("got %d routes, want %d", len(routes), len(tt.wantNames))
			}
			for _, r := range routes {
				want, ok := tt.wantNames[r.service]
				if !ok {
					t.Errorf("unexpected service %q in routes", r.service)
					continue
				}
				if r.routeName != want {
					t.Errorf("service %q: routeName = %q, want %q", r.service, r.routeName, want)
				}
			}
		})
	}
}

func TestRegisterComposeRoutes(t *testing.T) {
	t.Run("registers all routes successfully", func(t *testing.T) {
		var registered []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/routes" {
				var body map[string]string
				json.NewDecoder(r.Body).Decode(&body)
				registered = append(registered, body["name"])
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := unixHostClient(t, server)
		routes := []composeRoute{
			{service: "frontend", routeName: "frontend.myapp", upstream: "localhost:3000"},
			{service: "api", routeName: "api.myapp", upstream: "localhost:8080"},
		}

		err := registerComposeRoutes(client, routes, "/tmp/project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(registered) != 2 {
			t.Fatalf("got %d registrations, want 2", len(registered))
		}
		if registered[0] != "frontend.myapp" || registered[1] != "api.myapp" {
			t.Errorf("registered = %v", registered)
		}
	})

	t.Run("stops on first error and wraps route name", func(t *testing.T) {
		var registered []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/routes" {
				var body map[string]string
				json.NewDecoder(r.Body).Decode(&body)
				registered = append(registered, body["name"])
				if body["name"] == "api.myapp" {
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{"error": "daemon error"})
					return
				}
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := unixHostClient(t, server)
		routes := []composeRoute{
			{service: "frontend", routeName: "frontend.myapp", upstream: "localhost:3000"},
			{service: "api", routeName: "api.myapp", upstream: "localhost:8080"},
			{service: "worker", routeName: "worker.myapp", upstream: "localhost:9090"},
		}

		err := registerComposeRoutes(client, routes, "/tmp/project")
		if err == nil {
			t.Fatal("expected error when second route fails")
		}
		if !strings.Contains(err.Error(), "api.myapp") {
			t.Errorf("error should mention failing route name, got: %v", err)
		}
		// Worker should never be attempted since api failed
		if len(registered) != 2 {
			t.Fatalf("got %d registrations, want 2 (frontend + api)", len(registered))
		}
	})
}

func TestDeregisterComposeRoutes(t *testing.T) {
	var deregistered []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/routes/") {
			name := strings.TrimPrefix(r.URL.Path, "/routes/")
			deregistered = append(deregistered, name)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := unixHostClient(t, server)
	routes := []composeRoute{
		{service: "frontend", routeName: "frontend.myapp", upstream: "localhost:3000"},
		{service: "api", routeName: "api.myapp", upstream: "localhost:8080"},
	}

	deregisterComposeRoutes(client, routes)
	if len(deregistered) != 2 {
		t.Fatalf("got %d deregistrations, want 2", len(deregistered))
	}
	if deregistered[0] != "frontend.myapp" || deregistered[1] != "api.myapp" {
		t.Errorf("deregistered = %v", deregistered)
	}
}

func TestHeartbeatComposeReRegisters(t *testing.T) {
	var heartbeatCounts sync.Map // route name → count
	var registerCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/heartbeat"):
			// Extract route name from path: /routes/{name}/heartbeat
			parts := strings.Split(r.URL.Path, "/")
			name := parts[2]
			val, _ := heartbeatCounts.LoadOrStore(name, &atomic.Int32{})
			counter := val.(*atomic.Int32)
			if counter.Add(1) == 1 {
				// First heartbeat returns 404 to trigger re-registration
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/routes":
			registerCount.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := unixHostClient(t, server)
	state := newMultiRouteState([]composeRoute{
		{service: "frontend", routeName: "frontend.myapp", upstream: "localhost:3000"},
		{service: "api", routeName: "api.myapp", upstream: "localhost:8080"},
	}, "/tmp/project")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go heartbeatComposeWithInterval(ctx, client, state, 20*time.Millisecond)

	// Wait for re-registrations (both routes should re-register after first 404)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if registerCount.Load() >= 2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("expected at least 2 re-registrations, got %d", registerCount.Load())
}
