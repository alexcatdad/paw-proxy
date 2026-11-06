package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func startTestServer(t *testing.T) (*RouteRegistry, *http.Client) {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "api.sock")
	registry := NewRouteRegistry(30 * time.Second)
	srv := NewServer(socketPath, registry)
	go func() { _ = srv.Start() }()
	t.Cleanup(func() { _ = srv.Stop() })

	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("API socket never came up: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}}
	return registry, client
}

// do sends one request and returns its status and body; the body is read and
// closed here.
func do(t *testing.T, client *http.Client, method, path string, body any) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, "http://unix"+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, data
}

func TestAPIServer_RouteLifecycle(t *testing.T) {
	registry, client := startTestServer(t)

	reg := map[string]string{"name": "life", "upstream": "localhost:3000", "dir": "/tmp/life"}
	if status, _ := do(t, client, http.MethodPost, "/routes", reg); status != http.StatusOK {
		t.Fatalf("register: got %d", status)
	}

	// The same name from another directory is a conflict naming the owner.
	other := map[string]string{"name": "life", "upstream": "localhost:3001", "dir": "/tmp/other"}
	status, data := do(t, client, http.MethodPost, "/routes", other)
	if status != http.StatusConflict {
		t.Fatalf("conflicting register: got %d", status)
	}
	var conflict map[string]string
	if err := json.Unmarshal(data, &conflict); err != nil {
		t.Fatal(err)
	}
	if conflict["existingDir"] != "/tmp/life" {
		t.Errorf("conflict body = %v", conflict)
	}

	_, data = do(t, client, http.MethodGet, "/routes", nil)
	var routes []Route
	if err := json.Unmarshal(data, &routes); err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || routes[0].Name != "life" {
		t.Fatalf("list = %+v", routes)
	}

	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/routes/life/heartbeat", http.StatusOK},
		{http.MethodPost, "/routes/ghost/heartbeat", http.StatusNotFound},
		{http.MethodPost, "/routes/-bad-/heartbeat", http.StatusBadRequest},
		{http.MethodDelete, "/routes/-bad-", http.StatusBadRequest},
		{http.MethodDelete, "/routes/life", http.StatusOK},
		{http.MethodDelete, "/routes/life", http.StatusNotFound},
	}
	for _, c := range cases {
		if status, _ := do(t, client, c.method, c.path, nil); status != c.want {
			t.Errorf("%s %s: got %d, want %d", c.method, c.path, status, c.want)
		}
	}
	if _, ok := registry.Lookup("life"); ok {
		t.Error("route still registered after DELETE")
	}
}

func TestRegistryErrorMessages(t *testing.T) {
	c := &ConflictError{Name: "app", ExistingDir: "/src/app"}
	if got := c.Error(); !strings.Contains(got, `"app"`) || !strings.Contains(got, "/src/app") {
		t.Errorf("ConflictError = %q", got)
	}
	l := &LimitError{Limit: 7}
	if got := l.Error(); !strings.Contains(got, "7") {
		t.Errorf("LimitError = %q", got)
	}
}
