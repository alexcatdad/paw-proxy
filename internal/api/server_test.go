// internal/api/server_test.go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
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
