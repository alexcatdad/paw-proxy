// internal/api/server_test.go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAPIServer_RegisterRoute(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	registry := NewRouteRegistry(30 * time.Second)
	srv := NewServer(socketPath, registry)

	go srv.Start()
	defer srv.Stop()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	// Create HTTP client over unix socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	// Register a route
	body, _ := json.Marshal(map[string]string{
		"name":     "myapp",
		"upstream": "localhost:3000",
		"dir":      "/path/to/project",
	})

	resp, err := client.Post("http://unix/routes", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /routes failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Verify route exists
	route, ok := registry.Lookup("myapp")
	if !ok {
		t.Fatal("route not registered")
	}
	if route.Upstream != "localhost:3000" {
		t.Errorf("unexpected upstream: %s", route.Upstream)
	}
}

// Edge case tests for input validation

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

func TestValidateUpstream(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid localhost variants
		{"localhost", "localhost:3000", false},
		{"ipv4-loopback", "127.0.0.1:3000", false},
		{"ipv6-loopback", "[::1]:3000", false},
		{"port-1", "localhost:1", false},
		{"port-max", "localhost:65535", false},

		// Invalid: SSRF attempts - external hosts
		{"external-ip", "192.168.1.1:80", true},
		{"external-ip-10", "10.0.0.1:80", true},
		{"external-domain", "example.com:80", true},
		{"metadata-aws", "169.254.169.254:80", true},
		{"metadata-gcp", "metadata.google.internal:80", true},
		{"internal-hostname", "internal-service:8080", true},

		// SSRF: non-loopback addresses that look like localhost
		{"localhost-variant-0", "0.0.0.0:3000", true},
		// 127.0.0.2 is a valid loopback address (127.0.0.0/8), accepted by IsLoopback
		{"localhost-variant-127-2", "127.0.0.2:3000", false},
		{"localhost-hex", "0x7f000001:3000", true},
		{"localhost-octal", "0177.0.0.1:3000", true},
		{"localhost-decimal", "2130706433:3000", true},

		// Invalid: port out of range
		{"port-zero", "localhost:0", true},
		{"port-negative", "localhost:-1", true},
		{"port-too-high", "localhost:65536", true},
		{"port-way-too-high", "localhost:99999", true},

		// Invalid: malformed
		{"no-port", "localhost", true},
		{"empty-port", "localhost:", true},
		{"non-numeric-port", "localhost:abc", true},
		{"empty", "", true},
		{"just-port", ":3000", true},
		{"url-scheme", "http://localhost:3000", true},
		{"path-included", "localhost:3000/api", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUpstream(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateUpstream(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDir(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid absolute paths
		{"absolute-unix", "/home/user/project", false},
		{"absolute-with-spaces", "/home/user/my project", false},
		{"root", "/", false},

		// Invalid: empty
		{"empty", "", true},

		// Invalid: relative paths
		{"relative-dot", "./project", true},
		{"relative-dotdot", "../project", true},
		{"relative-plain", "project", true},

		// Invalid: path traversal
		{"traversal-in-path", "/home/user/../../../etc/passwd", true},
		{"traversal-at-end", "/home/user/project/..", true},
		{"double-slash", "/home//user/project", true},
		{"trailing-slash", "/home/user/project/", true},
		{"dot-in-middle", "/home/./user/project", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDir(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDir(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestAPIServer_ValidationRejection(t *testing.T) {
	tests := []struct {
		name     string
		reqBody  map[string]string
		wantCode int
	}{
		{
			name: "invalid route name",
			reqBody: map[string]string{
				"name":     "my;app",
				"upstream": "localhost:3000",
				"dir":      "/path/to/project",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "ssrf attempt",
			reqBody: map[string]string{
				"name":     "myapp",
				"upstream": "169.254.169.254:80",
				"dir":      "/path/to/project",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "path traversal",
			reqBody: map[string]string{
				"name":     "myapp",
				"upstream": "localhost:3000",
				"dir":      "/home/../../../etc/passwd",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "relative path",
			reqBody: map[string]string{
				"name":     "myapp",
				"upstream": "localhost:3000",
				"dir":      "./project",
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use /tmp directly to avoid socket path length limits
			socketPath := filepath.Join("/tmp", fmt.Sprintf("paw-test-%d.sock", time.Now().UnixNano()))
			defer os.Remove(socketPath)

			registry := NewRouteRegistry(30 * time.Second)
			srv := NewServer(socketPath, registry)

			go srv.Start()
			defer srv.Stop()

			time.Sleep(50 * time.Millisecond)

			client := &http.Client{
				Transport: &http.Transport{
					DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						return net.Dial("unix", socketPath)
					},
				},
			}

			body, _ := json.Marshal(tt.reqBody)
			resp, err := client.Post("http://unix/routes", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("POST /routes failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantCode {
				t.Errorf("expected %d, got %d", tt.wantCode, resp.StatusCode)
			}
		})
	}
}

func TestAPIServer_RegisterRouteLimit(t *testing.T) {
	socketPath := filepath.Join("/tmp", fmt.Sprintf("paw-test-limit-%d.sock", time.Now().UnixNano()))
	defer os.Remove(socketPath)

	registry := NewRouteRegistry(30 * time.Second)
	for i := 0; i < maxRoutes; i++ {
		name := fmt.Sprintf("app%d", i)
		if err := registry.Register(name, fmt.Sprintf("localhost:%d", 3000+i), "/tmp"); err != nil {
			t.Fatalf("failed pre-registering route %d: %v", i, err)
		}
	}
	srv := NewServer(socketPath, registry)

	go srv.Start()
	defer srv.Stop()

	time.Sleep(50 * time.Millisecond)

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	body, _ := json.Marshal(map[string]string{
		"name":     "overflow",
		"upstream": "localhost:9090",
		"dir":      "/tmp",
	})
	resp, err := client.Post("http://unix/routes", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /routes failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp.StatusCode)
	}

	var errResp map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed decoding JSON response: %v", err)
	}
	if !strings.Contains(errResp["error"], "route limit reached") {
		t.Fatalf("expected route limit error, got %q", errResp["error"])
	}
}

func TestValidateUpstreamIPv6Loopback(t *testing.T) {
	// Explicitly test that IPv6 loopback is accepted (fixes #46)
	if err := validateUpstream("[::1]:3000"); err != nil {
		t.Errorf("expected [::1]:3000 to be accepted, got error: %v", err)
	}
	if err := validateUpstream("[::1]:8080"); err != nil {
		t.Errorf("expected [::1]:8080 to be accepted, got error: %v", err)
	}
	// Non-loopback IPv6 must be rejected
	if err := validateUpstream("[::2]:3000"); err == nil {
		t.Error("expected [::2]:3000 to be rejected")
	}
	// Non-loopback private IPv4 must be rejected
	if err := validateUpstream("192.168.1.1:3000"); err == nil {
		t.Error("expected 192.168.1.1:3000 to be rejected")
	}
}

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

func TestAPIServer_JSONErrorFormat(t *testing.T) {
	// Use /tmp directly to avoid socket path length limits
	socketPath := filepath.Join("/tmp", fmt.Sprintf("paw-test-json-%d.sock", time.Now().UnixNano()))
	defer os.Remove(socketPath)

	registry := NewRouteRegistry(30 * time.Second)
	srv := NewServer(socketPath, registry)

	go srv.Start()
	defer srv.Stop()

	time.Sleep(50 * time.Millisecond)

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	// Send invalid request to trigger JSON error
	body, _ := json.Marshal(map[string]string{
		"name":     "my;app", // invalid name
		"upstream": "localhost:3000",
		"dir":      "/path/to/project",
	})

	resp, err := client.Post("http://unix/routes", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /routes failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify Content-Type is JSON
	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	// Verify body is valid JSON with "error" field
	var errResp map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode JSON error response: %v", err)
	}
	if errResp["error"] == "" {
		t.Error("expected non-empty 'error' field in JSON response")
	}
}

func TestAPIServer_RequestBodyLimit(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	registry := NewRouteRegistry(30 * time.Second)
	srv := NewServer(socketPath, registry)

	go srv.Start()
	defer srv.Stop()

	time.Sleep(50 * time.Millisecond)

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	// Create oversized body (> 1MB)
	oversizedBody := strings.Repeat("x", 2*1024*1024)
	resp, err := client.Post("http://unix/routes", "application/json", strings.NewReader(oversizedBody))
	if err != nil {
		t.Fatalf("POST /routes failed: %v", err)
	}
	defer resp.Body.Close()

	// Should reject with 400 (bad request due to body limit exceeded)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized body, got %d", resp.StatusCode)
	}
}
