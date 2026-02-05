// internal/proxy/proxy_test.go
package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxy_ForwardsRequest(t *testing.T) {
	// Create upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-Proto") != "https" {
			t.Error("missing X-Forwarded-Proto header")
		}
		w.Write([]byte("hello from upstream"))
	}))
	defer upstream.Close()

	// Create proxy
	p := New()

	// Create test request
	req := httptest.NewRequest("GET", "https://myapp.test/api", nil)
	w := httptest.NewRecorder()

	// Proxy the request
	p.ServeHTTP(w, req, upstream.URL[7:]) // strip "http://"

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != "hello from upstream" {
		t.Errorf("unexpected body: %s", string(body))
	}
}

func TestProxyPreservesHostHeader(t *testing.T) {
	var receivedHost string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	p := New()

	req := httptest.NewRequest("GET", "https://myapp.test/", nil)
	req.Host = "myapp.test"
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req, upstream.URL[7:]) // strip "http://"

	if receivedHost != "myapp.test" {
		t.Errorf("expected upstream to receive Host 'myapp.test', got %q", receivedHost)
	}
}

func TestProxyStripsHopByHopHeaders(t *testing.T) {
	var receivedHeaders http.Header

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	p := New()

	req := httptest.NewRequest("GET", "https://myapp.test/", nil)
	req.Header.Set("Connection", "keep-alive, X-Custom-Hop")
	req.Header.Set("Keep-Alive", "timeout=5")
	req.Header.Set("Proxy-Authorization", "Basic dXNlcjpwYXNz")
	req.Header.Set("Transfer-Encoding", "chunked")
	req.Header.Set("X-Custom-Hop", "should-be-stripped")
	req.Header.Set("X-Legit-Header", "should-remain")

	w := httptest.NewRecorder()
	p.ServeHTTP(w, req, upstream.URL[7:])

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// These hop-by-hop headers should be stripped
	for _, h := range []string{"Connection", "Keep-Alive", "Proxy-Authorization", "Transfer-Encoding", "X-Custom-Hop"} {
		if receivedHeaders.Get(h) != "" {
			t.Errorf("hop-by-hop header %q should have been stripped, got %q", h, receivedHeaders.Get(h))
		}
	}

	// Regular headers should remain
	if receivedHeaders.Get("X-Legit-Header") != "should-remain" {
		t.Errorf("expected X-Legit-Header to be preserved, got %q", receivedHeaders.Get("X-Legit-Header"))
	}
}

func TestDialRejectsNonLoopback(t *testing.T) {
	p := New()

	// Create a request to a non-local upstream — DialContext should reject it
	req := httptest.NewRequest("GET", "https://myapp.test/", nil)
	w := httptest.NewRecorder()

	// Use an external address that should be rejected
	p.ServeHTTP(w, req, "192.168.1.1:8080")

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502 for non-loopback upstream, got %d", w.Code)
	}

	body, _ := io.ReadAll(w.Result().Body)
	if got := string(body); !strings.Contains(got, "refusing connection to non-local host") {
		t.Errorf("expected non-local host error, got: %s", got)
	}
}

func TestIsWebSocketRequiresConnectionUpgrade(t *testing.T) {
	tests := []struct {
		name       string
		upgrade    string
		connection string
		want       bool
	}{
		{
			name:       "valid websocket upgrade",
			upgrade:    "websocket",
			connection: "Upgrade",
			want:       true,
		},
		{
			name:       "upgrade header only",
			upgrade:    "websocket",
			connection: "",
			want:       false,
		},
		{
			name:       "connection header only",
			upgrade:    "",
			connection: "Upgrade",
			want:       false,
		},
		{
			name:       "neither header",
			upgrade:    "",
			connection: "",
			want:       false,
		},
		{
			name:       "case insensitive",
			upgrade:    "WebSocket",
			connection: "upgrade, keep-alive",
			want:       true,
		},
		{
			name:       "connection without upgrade token",
			upgrade:    "websocket",
			connection: "keep-alive",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "https://myapp.test/ws", nil)
			if tt.upgrade != "" {
				req.Header.Set("Upgrade", tt.upgrade)
			}
			if tt.connection != "" {
				req.Header.Set("Connection", tt.connection)
			}

			got := isWebSocket(req)
			if got != tt.want {
				t.Errorf("isWebSocket() = %v, want %v", got, tt.want)
			}
		})
	}
}

