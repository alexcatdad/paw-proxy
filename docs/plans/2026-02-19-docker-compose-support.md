# Docker Compose Support Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable `up docker compose up` to auto-discover services from compose config and register multi-level routes (`service.project.test`).

**Architecture:** The `up` command detects Docker Compose mode by scanning for `docker compose ... up` in args, runs `docker compose config --format json` to discover services, registers a route per service with published ports, then runs the original compose command as a child process with multi-route heartbeats and cleanup.

**Tech Stack:** Go 1.24 stdlib (`encoding/json`, `os/exec`, `net`, `strings`). No new dependencies.

**Design doc:** `docs/plans/2026-02-19-docker-compose-support-design.md`

---

### Task 1: Update `ExtractName` to support dotted route names

The existing `ExtractName()` stops at the first `.`, so `frontend.myapp.test` returns `frontend` instead of `frontend.myapp`. Rewrite it to strip the `.test` suffix instead.

**Files:**
- Modify: `internal/api/routes.go:1-8` (imports), `internal/api/routes.go:100-109` (ExtractName)
- Modify: `internal/api/server_test.go:358-379` (TestExtractName)

**Step 1: Update the `TestExtractName` test cases**

In `internal/api/server_test.go`, replace the `TestExtractName` function with updated expectations:

```go
func TestExtractName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Single-level names (backwards compatible)
		{"myapp.test:443", "myapp"},
		{"myapp.test", "myapp"},
		{"myapp:443", "myapp"},
		{"myapp", "myapp"},

		// Multi-level names (Docker Compose: service.project.test)
		{"frontend.myapp.test", "frontend.myapp"},
		{"frontend.myapp.test:443", "frontend.myapp"},
		{"api.shop.test", "api.shop"},

		// Edge cases
		{"", ""},
		{"a.b.c", "a.b.c"},          // no .test suffix → unchanged
		{".test", ""},                // just .test → empty
		{"app.test:8080", "app"},     // port stripping
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ExtractName(tt.input)
			if got != tt.want {
				t.Errorf("ExtractName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run the test to verify it fails**

Run: `go test -v -race -run TestExtractName ./internal/api/`
Expected: FAIL — `ExtractName("frontend.myapp.test")` returns `"frontend"`, not `"frontend.myapp"`

**Step 3: Implement the new `ExtractName`**

In `internal/api/routes.go`, add `"net"` and `"strings"` to imports, then replace `ExtractName`:

```go
import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)
```

```go
// ExtractName extracts the route name from a host string like
// "myapp.test", "frontend.myapp.test:443", etc. Strips port and .test suffix.
func ExtractName(host string) string {
	// Strip port if present (e.g., "myapp.test:443" → "myapp.test")
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	// Strip .test suffix (e.g., "frontend.myapp.test" → "frontend.myapp")
	return strings.TrimSuffix(host, ".test")
}
```

**Step 4: Run the test to verify it passes**

Run: `go test -v -race -run TestExtractName ./internal/api/`
Expected: PASS

**Step 5: Run all api tests to check for regressions**

Run: `go test -v -race ./internal/api/`
Expected: PASS (the concurrent test at line 172 uses `name + ".test:443"` which will still work)

**Step 6: Commit**

```bash
git add internal/api/routes.go internal/api/server_test.go
git commit -m "refactor: rewrite ExtractName to strip .test suffix instead of splitting on first dot

Supports multi-level route names like frontend.myapp for Docker Compose
service.project.test naming scheme."
```

---

### Task 2: Allow dots in route name validation

The regex `^[a-zA-Z][a-zA-Z0-9_-]{0,62}$` rejects dots. Add `.` to the character class. Also update the existing test that explicitly asserts dots are invalid, and add new test cases for dotted names.

**Files:**
- Modify: `internal/api/server.go:26` (routeNamePattern)
- Modify: `internal/api/server_test.go:69-116` (TestValidateRouteName)

**Step 1: Update `TestValidateRouteName` test cases**

In `internal/api/server_test.go`, change the existing `{"dot", "my.app", true}` entry and add new dotted-name cases:

Find this block in the test table:
```go
		{"dot", "my.app", true},
```

Replace with:
```go
		{"dot-single", "my.app", false},
		{"dot-multi", "frontend.myapp", false},
		{"dot-triple", "a.b.c", false},
```

Also add to the invalid section:
```go
		{"starts-with-dot", ".myapp", true},
		{"ends-with-dot-only", "a.", false},
```

Wait — `"a."` would match `^[a-zA-Z][a-zA-Z0-9._-]{0,62}$`. Is that desirable? A trailing dot is technically valid in DNS (FQDN), but it's ugly. However, since we sanitize names through `sanitizeName()` before registration, and `sanitizeName` would convert the dot to a dash, this is fine. The regex is just the daemon-side validation. Let's keep it simple and allow it.

Actually, let's reconsider. The updated test table:

```go
func TestValidateRouteName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid cases
		{"simple", "myapp", false},
		{"with-dash", "my-app", false},
		{"with-underscore", "my_app", false},
		{"numeric-suffix", "app123", false},
		{"single-char", "a", false},
		{"max-length-63", strings.Repeat("a", 63), false},

		// Valid: dotted names for Docker Compose service.project routes
		{"dotted-single", "my.app", false},
		{"dotted-compose", "frontend.myapp", false},
		{"dotted-triple", "a.b.c", false},

		// Invalid: empty or too long
		{"empty", "", true},
		{"too-long-64", strings.Repeat("a", 64), true},

		// Invalid: must start with a letter
		{"starts-with-dash", "-myapp", true},
		{"starts-with-underscore", "_myapp", true},
		{"starts-with-number", "1app", true},
		{"starts-with-dot", ".myapp", true},

		// Invalid: special characters (injection attempts)
		{"shell-injection-semicolon", "app;rm -rf /", true},
		{"shell-injection-pipe", "app|cat /etc/passwd", true},
		{"shell-injection-backtick", "app`id`", true},
		{"shell-injection-dollar", "app$(whoami)", true},
		{"path-traversal", "../../../etc/passwd", true},
		{"null-byte", "app\x00malicious", true},
		{"newline", "app\nmalicious", true},
		{"space", "my app", true},
		{"slash", "my/app", true},
		{"backslash", "my\\app", true},
		{"unicode", "app™", true},
		{"emoji", "app🚀", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRouteName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRouteName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
```

**Step 2: Run the test to verify it fails**

Run: `go test -v -race -run TestValidateRouteName ./internal/api/`
Expected: FAIL — `"my.app"` is rejected by current regex

**Step 3: Update the regex**

In `internal/api/server.go:26`, change:

```go
var routeNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,62}$`)
```

To:

```go
var routeNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]{0,62}$`)
```

Update the comment on line 25:

```go
// Route name validation pattern: starts with letter; rest can be alphanumeric, dash, underscore, or dot.
```

Also update the error message in `validateRouteName()` at line 96:

```go
return fmt.Errorf("invalid route name: must start with a letter and contain only letters, numbers, dashes, underscores, or dots (max 63 chars)")
```

**Step 4: Run the test to verify it passes**

Run: `go test -v -race -run TestValidateRouteName ./internal/api/`
Expected: PASS

**Step 5: Run all api tests**

Run: `go test -v -race ./internal/api/`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/api/server.go internal/api/server_test.go
git commit -m "feat: allow dots in route names for service.project naming

Enables Docker Compose multi-level route names like frontend.myapp
while maintaining all existing security validation."
```

---

### Task 3: Add Docker Compose detection and config parsing functions

Add pure functions for detecting Docker Compose mode and parsing compose config JSON. These are independently testable with no side effects.

**Files:**
- Create: `cmd/up/compose.go`
- Create: `cmd/up/compose_test.go`

**Step 1: Write the failing tests**

Create `cmd/up/compose_test.go`:

```go
package main

import (
	"testing"
)

func TestDetectDockerCompose(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantDetected   bool
		wantFlags      []string
		wantUpIdx      int
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

func TestBuildComposeRouteNames(t *testing.T) {
	tests := []struct {
		name        string
		services    []discoveredService
		projectName string
		nameFlag    string
		wantNames   map[string]string // service → expected route name
	}{
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
```

**Step 2: Run tests to verify they fail**

Run: `go test -v -race -run 'TestDetectDockerCompose|TestParseComposeConfig|TestBuildComposeRouteNames' ./cmd/up/`
Expected: FAIL — functions don't exist yet

**Step 3: Implement the compose detection and parsing functions**

Create `cmd/up/compose.go`:

```go
// cmd/up/compose.go
package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

// composeDetection holds the result of scanning args for Docker Compose mode.
type composeDetection struct {
	detected     bool
	composeFlags []string // flags between "compose" and "up" (e.g., ["--profile", "frontend"])
	upIdx        int      // index of "up" in args
}

// detectDockerCompose checks if args represent a `docker compose ... up` command.
// Returns detection info including any compose-level flags captured between
// "compose" and "up" (e.g., --profile, -f, --project-name).
func detectDockerCompose(args []string) composeDetection {
	if len(args) < 3 || args[0] != "docker" || args[1] != "compose" {
		return composeDetection{}
	}

	for i := 2; i < len(args); i++ {
		if args[i] == "up" {
			var flags []string
			if i > 2 {
				flags = make([]string, i-2)
				copy(flags, args[2:i])
			}
			return composeDetection{
				detected:     true,
				composeFlags: flags,
				upIdx:        i,
			}
		}
	}

	return composeDetection{}
}

// composeConfig represents the JSON output of `docker compose config --format json`.
type composeConfig struct {
	Name     string                    `json:"name"`
	Services map[string]composeService `json:"services"`
}

type composeService struct {
	Ports []composePort `json:"ports"`
}

type composePort struct {
	Published string `json:"published"`
	Target    int    `json:"target"`
	Protocol  string `json:"protocol"`
}

// discoveredService represents a Docker Compose service with a published port.
type discoveredService struct {
	service       string
	publishedPort string
}

// composeRoute is a fully resolved route ready for registration.
type composeRoute struct {
	service       string // original compose service name
	routeName     string // sanitized name for paw-proxy (e.g., "frontend.myapp")
	upstream      string // e.g., "localhost:3000"
}

// parseComposeConfig parses `docker compose config --format json` output and
// extracts services that have published ports.
func parseComposeConfig(data []byte) ([]discoveredService, string, error) {
	var config composeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, "", fmt.Errorf("parsing compose config: %w", err)
	}

	var services []discoveredService

	// Sort service names for deterministic output
	names := make([]string, 0, len(config.Services))
	for name := range config.Services {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		svc := config.Services[name]
		if len(svc.Ports) == 0 {
			continue
		}
		// Use the first published port
		port := svc.Ports[0]
		if port.Published == "" {
			continue
		}
		services = append(services, discoveredService{
			service:       name,
			publishedPort: port.Published,
		})
	}

	return services, config.Name, nil
}

// buildComposeRouteNames creates route entries with sanitized names.
// If nameFlag is set, it overrides the project name portion.
func buildComposeRouteNames(services []discoveredService, projectName, nameFlag string) []composeRoute {
	project := projectName
	if nameFlag != "" {
		project = nameFlag
	}
	project = sanitizeName(project)

	routes := make([]composeRoute, 0, len(services))
	for _, svc := range services {
		svcName := sanitizeName(svc.service)
		routes = append(routes, composeRoute{
			service:   svc.service,
			routeName: svcName + "." + project,
			upstream:  fmt.Sprintf("localhost:%s", svc.publishedPort),
		})
	}
	return routes
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -v -race -run 'TestDetectDockerCompose|TestParseComposeConfig|TestBuildComposeRouteNames' ./cmd/up/`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/up/compose.go cmd/up/compose_test.go
git commit -m "feat: add Docker Compose detection and config parsing

Pure functions for detecting docker compose up in args,
parsing compose config JSON, and building route names.
No side effects, fully unit tested."
```

---

### Task 4: Add multi-route state and lifecycle functions

Add the multi-route state management, heartbeat, registration, and cleanup functions that the main Docker Compose codepath will use.

**Files:**
- Modify: `cmd/up/compose.go` (add lifecycle functions)
- Modify: `cmd/up/compose_test.go` (add lifecycle tests)

**Step 1: Write the failing tests**

Append to `cmd/up/compose_test.go`:

```go
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

	t.Run("route names returns sorted names", func(t *testing.T) {
		state := newMultiRouteState([]composeRoute{
			{service: "frontend", routeName: "frontend.myapp", upstream: "localhost:3000"},
			{service: "api", routeName: "api.myapp", upstream: "localhost:8080"},
		}, "/tmp/project")

		names := state.RouteNames()
		if len(names) != 2 {
			t.Fatalf("got %d names, want 2", len(names))
		}
		// Should be in same order as initialized
		if names[0] != "frontend.myapp" || names[1] != "api.myapp" {
			t.Errorf("names = %v", names)
		}
	})
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -v -race -run TestMultiRouteState ./cmd/up/`
Expected: FAIL — `newMultiRouteState` doesn't exist

**Step 3: Implement multi-route state**

Append to `cmd/up/compose.go`:

```go
// multiRouteState manages multiple route entries for Docker Compose mode.
type multiRouteState struct {
	mu     sync.RWMutex
	routes []composeRoute
	dir    string
}

func newMultiRouteState(routes []composeRoute, dir string) *multiRouteState {
	return &multiRouteState{
		routes: routes,
		dir:    dir,
	}
}

// Snapshot returns a copy of all routes and the directory.
func (s *multiRouteState) Snapshot() ([]composeRoute, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	routes := make([]composeRoute, len(s.routes))
	copy(routes, s.routes)
	return routes, s.dir
}

// RouteNames returns the route names in order.
func (s *multiRouteState) RouteNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, len(s.routes))
	for i, r := range s.routes {
		names[i] = r.routeName
	}
	return names
}
```

Add `"sync"` to the imports in `compose.go`.

**Step 4: Run tests to verify they pass**

Run: `go test -v -race -run TestMultiRouteState ./cmd/up/`
Expected: PASS

**Step 5: Run all cmd/up tests**

Run: `go test -v -race ./cmd/up/`
Expected: PASS

**Step 6: Commit**

```bash
git add cmd/up/compose.go cmd/up/compose_test.go
git commit -m "feat: add multi-route state management for Docker Compose mode

Thread-safe state container that holds multiple compose routes
with snapshot capability for heartbeat goroutines."
```

---

### Task 5: Wire Docker Compose mode into `main()`

Integrate the compose detection and lifecycle into the main function. This is the wiring task — connecting the pure functions from Tasks 3-4 to the existing `up` command flow.

**Files:**
- Modify: `cmd/up/main.go` (add Docker Compose branch in main)

**Step 1: Add the `runDockerCompose` function to `cmd/up/compose.go`**

This function encapsulates the entire Docker Compose mode flow. Append to `cmd/up/compose.go`:

```go
// runComposeConfig runs `docker compose [flags] config --format json` and returns the raw output.
func runComposeConfig(composeFlags []string) ([]byte, error) {
	configArgs := []string{"compose"}
	configArgs = append(configArgs, composeFlags...)
	configArgs = append(configArgs, "config", "--format", "json")

	cmd := exec.Command("docker", configArgs...)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose config failed: %w", err)
	}
	return output, nil
}

// registerComposeRoutes registers all compose routes with the daemon.
func registerComposeRoutes(client *http.Client, routes []composeRoute, dir string) error {
	for _, r := range routes {
		if err := registerRoute(client, r.routeName, r.upstream, dir); err != nil {
			return fmt.Errorf("registering %s: %w", r.routeName, err)
		}
	}
	return nil
}

// deregisterComposeRoutes deregisters all compose routes from the daemon.
func deregisterComposeRoutes(client *http.Client, routes []composeRoute) {
	for _, r := range routes {
		if err := deregisterRoute(client, r.routeName); err != nil {
			log.Printf("warning: deregister %s failed: %v", r.routeName, err)
		}
	}
}

// heartbeatCompose sends heartbeats for all compose routes.
func heartbeatCompose(ctx context.Context, client *http.Client, state *multiRouteState) {
	heartbeatComposeWithInterval(ctx, client, state, 10*time.Second)
}

func heartbeatComposeWithInterval(ctx context.Context, client *http.Client, state *multiRouteState, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			routes, dir := state.Snapshot()
			for _, r := range routes {
				req, err := http.NewRequest("POST", fmt.Sprintf("http://unix/routes/%s/heartbeat", r.routeName), nil)
				if err != nil {
					log.Printf("warning: compose heartbeat request failed for %s: %v", r.routeName, err)
					continue
				}
				resp, err := client.Do(req)
				if err != nil {
					log.Printf("warning: compose heartbeat failed for %s: %v", r.routeName, err)
					continue
				}

				if resp.StatusCode == http.StatusOK {
					resp.Body.Close()
					continue
				}
				resp.Body.Close()

				if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
					if err := registerRoute(client, r.routeName, r.upstream, dir); err != nil {
						log.Printf("warning: compose auto re-register failed for %s: %v", r.routeName, err)
						continue
					}
					log.Printf("route re-registered after daemon restart: %s.test -> %s", r.routeName, r.upstream)
					continue
				}

				log.Printf("warning: compose heartbeat for %s returned status %d", r.routeName, resp.StatusCode)
			}
		}
	}
}
```

Add `"context"`, `"log"`, `"net/http"`, `"os"`, `"os/exec"`, and `"time"` to the imports in `compose.go`.

**Step 2: Add Docker Compose branch in `main()`**

In `cmd/up/main.go`, after the daemon health check (line 103) and before the `determineName` call (line 106), add the Docker Compose detection and early branch:

```go
	// Check for Docker Compose mode
	args := flag.Args()
	dc := detectDockerCompose(args)
	if dc.detected {
		runDockerComposeMode(client, dc, args, caPath)
		return
	}

	// Determine app name (existing single-app flow)
	name := determineName(*nameFlag)
```

And remove the duplicate `args := flag.Args()` at the old line 165.

Then add the `runDockerComposeMode` function to `cmd/up/compose.go`:

```go
// runDockerComposeMode handles the entire lifecycle when `up` wraps `docker compose up`.
func runDockerComposeMode(client *http.Client, dc composeDetection, args []string, caPath string) {
	// 1. Discover services via docker compose config
	configOutput, err := runComposeConfig(dc.composeFlags)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	services, projectName, err := parseComposeConfig(configOutput)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if len(services) == 0 {
		fmt.Println("Error: no services with published ports found in docker-compose config")
		os.Exit(1)
	}

	// 2. Build route names
	routes := buildComposeRouteNames(services, projectName, *nameFlag)

	dir, _ := os.Getwd()
	state := newMultiRouteState(routes, dir)

	// 3. Register all routes
	if err := registerComposeRoutes(client, routes, dir); err != nil {
		fmt.Printf("Error registering routes: %v\n", err)
		os.Exit(1)
	}

	// 4. Print route mappings
	for _, r := range routes {
		fmt.Printf("🔗 Mapping https://%s.test -> %s...\n", r.routeName, r.upstream)
	}
	fmt.Printf("🚀 %d services live:\n", len(routes))
	for _, r := range routes {
		fmt.Printf("   https://%s.test\n", r.routeName)
	}
	fmt.Println("------------------------------------------------")

	// 5. Start heartbeat
	ctx, cancel := context.WithCancel(context.Background())
	go heartbeatCompose(ctx, client, state)

	// 6. Cleanup function
	cleanup := func() {
		fmt.Printf("\n🛑 Removing %d route mappings...\n", len(routes))
		deregisterComposeRoutes(client, routes)
	}

	// 7. Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// 8. Run docker compose up as child process
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("NODE_EXTRA_CA_CERTS=%s", caPath),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		fmt.Printf("Error starting docker compose: %v\n", err)
		cancel()
		cleanup()
		os.Exit(1)
	}

	// 9. Wait for signal or command exit
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
	}()

	var exitCode int
	select {
	case sig := <-sigCh:
		syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
		select {
		case <-doneCh:
		case <-time.After(10 * time.Second):
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	case err := <-doneCh:
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	cancel()
	cleanup()
	os.Exit(exitCode)
}
```

Add `"os/signal"` and `"syscall"` to the imports in `compose.go`.

**Step 2: Verify compilation**

Run: `go vet ./cmd/up/`
Expected: Clean (no errors)

**Step 3: Run all existing tests to check for regressions**

Run: `go test -v -race ./cmd/up/`
Expected: PASS (all existing tests still work)

**Step 4: Run full test suite**

Run: `go test -v -race ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/up/compose.go cmd/up/main.go
git commit -m "feat: wire Docker Compose mode into up command

When up detects docker compose up in args, it runs compose config
to discover services, registers multi-level routes (service.project.test),
manages heartbeats for all routes, and cleans up on exit."
```

---

### Task 6: Add heartbeat tests for compose mode

Test the compose heartbeat and re-registration logic using the same httptest pattern as existing heartbeat tests.

**Files:**
- Modify: `cmd/up/compose_test.go`

**Step 1: Write the heartbeat test**

Append to `cmd/up/compose_test.go`:

```go
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
```

Add these imports to the test file if not already present: `"context"`, `"net/http"`, `"net/http/httptest"`, `"strings"`, `"sync"`, `"sync/atomic"`.

**Step 2: Run the test**

Run: `go test -v -race -run TestHeartbeatComposeReRegisters ./cmd/up/`
Expected: PASS

**Step 3: Run full test suite**

Run: `go test -v -race ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add cmd/up/compose_test.go
git commit -m "test: add compose heartbeat re-registration test

Verifies that heartbeatCompose re-registers routes when daemon
returns 404, matching existing heartbeat behavior for single-app mode."
```

---

### Task 7: Final verification and full test run

Run all tests, vet, and verify everything works end-to-end.

**Files:** None (verification only)

**Step 1: Run vet**

Run: `go vet ./...`
Expected: Clean

**Step 2: Run full test suite with race detector**

Run: `go test -v -race ./...`
Expected: All tests PASS

**Step 3: Build both binaries**

Run: `go build -o paw-proxy ./cmd/paw-proxy && go build -o up ./cmd/up`
Expected: Both build successfully

**Step 4: Verify up --help still works**

Run: `./up --help`
Expected: Shows help text (no Docker Compose-specific help needed yet)

**Step 5: Clean up binaries**

Run: `rm -f paw-proxy up`

**Step 6: Final commit if any cleanup was needed, otherwise skip**

---

## Summary of changes

| File | Change | Lines |
|------|--------|-------|
| `internal/api/routes.go` | Rewrite `ExtractName()` to strip `.test` suffix | ~8 |
| `internal/api/server.go` | Allow dots in route name regex | ~3 |
| `internal/api/server_test.go` | Update `TestExtractName` + `TestValidateRouteName` | ~20 |
| `cmd/up/compose.go` | New file: detection, parsing, state, lifecycle | ~200 |
| `cmd/up/compose_test.go` | New file: tests for all compose functions | ~250 |
| `cmd/up/main.go` | Add compose mode branch (~5 lines) | ~5 |

Total: ~480 lines of new/changed code, ~270 lines of tests.
