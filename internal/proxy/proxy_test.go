// internal/proxy/proxy_test.go
package proxy

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProxy_PreservesHostHeader(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "myapp.test" {
			t.Errorf("expected Host header 'myapp.test', got %q", r.Host)
		}
		if r.Header.Get("X-Forwarded-Host") != "myapp.test" {
			t.Errorf("expected X-Forwarded-Host 'myapp.test', got %q", r.Header.Get("X-Forwarded-Host"))
		}
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	p := New()
	req := httptest.NewRequest("GET", "https://myapp.test/", nil)
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req, upstream.URL[7:])

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

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

func TestIdleTimeoutConn_ResetsDeadlineOnRead(t *testing.T) {
	// Create a pipe for testing
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// Wrap server side with idle timeout
	idleConn := &idleTimeoutConn{
		Conn:    server,
		timeout: 100 * time.Millisecond,
	}

	// Send data from client
	go func() {
		time.Sleep(50 * time.Millisecond)
		client.Write([]byte("hello"))
		time.Sleep(50 * time.Millisecond)
		client.Write([]byte("world"))
	}()

	// Read should succeed even though total time > timeout
	// because deadline is reset on each read
	buf := make([]byte, 5)
	n, err := idleConn.Read(buf)
	if err != nil {
		t.Fatalf("first read failed: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("expected 'hello', got %q", string(buf[:n]))
	}

	n, err = idleConn.Read(buf)
	if err != nil {
		t.Fatalf("second read failed: %v", err)
	}
	if string(buf[:n]) != "world" {
		t.Errorf("expected 'world', got %q", string(buf[:n]))
	}
}

func TestIdleTimeoutConn_ResetsDeadlineOnWrite(t *testing.T) {
	// Create a pipe for testing
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// Wrap server side with idle timeout
	idleConn := &idleTimeoutConn{
		Conn:    server,
		timeout: 100 * time.Millisecond,
	}

	// Read from client side
	go func() {
		buf := make([]byte, 10)
		io.ReadFull(client, buf)
	}()

	// Multiple writes should succeed even though total time > timeout
	time.Sleep(50 * time.Millisecond)
	if _, err := idleConn.Write([]byte("hello")); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if _, err := idleConn.Write([]byte("world")); err != nil {
		t.Fatalf("second write failed: %v", err)
	}
}

func TestIdleTimeoutConn_TimesOutOnIdle(t *testing.T) {
	// Create a pipe for testing
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// Wrap server side with short idle timeout
	idleConn := &idleTimeoutConn{
		Conn:    server,
		timeout: 50 * time.Millisecond,
	}

	// Don't send any data - connection should timeout
	buf := make([]byte, 5)
	start := time.Now()
	_, err := idleConn.Read(buf)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// Should timeout around 50ms (allow some variance)
	if elapsed < 40*time.Millisecond || elapsed > 150*time.Millisecond {
		t.Errorf("expected timeout around 50ms, got %v", elapsed)
	}
}
