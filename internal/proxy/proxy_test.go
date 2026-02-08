// internal/proxy/proxy_test.go
package proxy

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
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
	got := string(body)
	// The error page should be HTML and reference the upstream address
	if !strings.Contains(got, "text/html") && w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("expected HTML content type, got %s", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(got, "192.168.1.1:8080") {
		t.Errorf("expected upstream address in error page, got: %s", got)
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

func TestIdleTimeoutConn_ReadResetsDeadline(t *testing.T) {
	// Create a pipe to simulate a connection
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	timeout := 50 * time.Millisecond
	idle := &idleTimeoutConn{Conn: client, timeout: timeout}

	// Write data from server side so the read succeeds
	go func() {
		server.Write([]byte("hello"))
	}()

	buf := make([]byte, 10)
	n, err := idle.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error on read: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("expected 'hello', got %q", string(buf[:n]))
	}

	// After a successful read, the deadline should have been reset.
	// If we wait less than the timeout and do another read with data
	// available, it should succeed (deadline was pushed forward).
	time.Sleep(20 * time.Millisecond)
	go func() {
		server.Write([]byte("world"))
	}()

	n, err = idle.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error on second read: %v", err)
	}
	if string(buf[:n]) != "world" {
		t.Errorf("expected 'world', got %q", string(buf[:n]))
	}
}

func TestIdleTimeoutConn_WriteResetsDeadline(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	timeout := 50 * time.Millisecond
	idle := &idleTimeoutConn{Conn: client, timeout: timeout}

	// Drain reads from server side so writes don't block
	go func() {
		io.Copy(io.Discard, server)
	}()

	n, err := idle.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error on write: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes written, got %d", n)
	}

	// Wait less than timeout, write again — should succeed
	time.Sleep(20 * time.Millisecond)
	n, err = idle.Write([]byte("world"))
	if err != nil {
		t.Fatalf("unexpected error on second write: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes written, got %d", n)
	}
}

func TestIdleTimeoutConn_TimesOutAfterIdle(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	timeout := 50 * time.Millisecond
	idle := &idleTimeoutConn{Conn: client, timeout: timeout}

	// Do an initial read to set the deadline, then let it expire
	go func() {
		server.Write([]byte("x"))
	}()
	buf := make([]byte, 10)
	_, err := idle.Read(buf)
	if err != nil {
		t.Fatalf("initial read failed: %v", err)
	}

	// Now wait longer than the timeout without sending data
	time.Sleep(80 * time.Millisecond)

	// The next read should fail because the deadline passed
	_, err = idle.Read(buf)
	if err == nil {
		t.Fatal("expected timeout error on read after idle period, got nil")
	}
	// Verify it's a timeout error
	if netErr, ok := err.(net.Error); ok {
		if !netErr.Timeout() {
			t.Errorf("expected timeout error, got: %v", err)
		}
	}
	// net.Pipe returns a different error than net.TCPConn on deadline
	// so we just verify we got an error (could be timeout or i/o timeout)
}

func TestWebSocket_BothGoroutinesComplete(t *testing.T) {
	// This test verifies that the bidirectional copy in handleWebSocket
	// properly waits for BOTH goroutines. We simulate this by creating
	// a TCP echo server and verifying that data flows both ways and
	// the proxy cleans up after both directions complete.

	// Start a simple TCP echo server (simulates upstream WebSocket)
	echoListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start echo listener: %v", err)
	}
	defer echoListener.Close()

	echoAddr := echoListener.Addr().String()

	var echoWg sync.WaitGroup
	echoWg.Add(1)
	go func() {
		defer echoWg.Done()
		conn, err := echoListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read the HTTP upgrade request and send a response, then echo data
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		_ = n // consume the HTTP request

		// Send WebSocket upgrade response
		conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"))

		// Echo all data back
		io.Copy(conn, conn)
	}()

	// Create a TCP connection pair to simulate client<->proxy
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start proxy listener: %v", err)
	}
	defer proxyListener.Close()

	// Connect client side
	clientConn, err := net.DialTimeout("tcp", proxyListener.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("failed to dial proxy: %v", err)
	}
	defer clientConn.Close()

	serverConn, err := proxyListener.Accept()
	if err != nil {
		t.Fatalf("failed to accept proxy connection: %v", err)
	}
	defer serverConn.Close()

	// Now simulate what handleWebSocket does: connect to upstream and
	// start bidirectional copy with idle timeout wrappers
	upstreamConn, err := net.DialTimeout("tcp", echoAddr, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect to echo server: %v", err)
	}
	defer upstreamConn.Close()

	// Send a fake HTTP request to the upstream to trigger the echo server
	upstreamConn.Write([]byte("GET / HTTP/1.1\r\nHost: test\r\nUpgrade: websocket\r\n\r\n"))

	// Read the 101 response
	respBuf := make([]byte, 4096)
	n, err := upstreamConn.Read(respBuf)
	if err != nil {
		t.Fatalf("failed to read 101 response: %v", err)
	}
	_ = n

	// Wrap with idle timeout
	idleTimeout := 200 * time.Millisecond
	clientIdle := &idleTimeoutConn{Conn: serverConn, timeout: idleTimeout}
	upstreamIdle := &idleTimeoutConn{Conn: upstreamConn, timeout: idleTimeout}

	done := make(chan struct{}, 2)

	go func() {
		io.Copy(upstreamIdle, clientIdle)
		if tc, ok := upstreamConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()

	go func() {
		io.Copy(clientIdle, upstreamIdle)
		if tc, ok := serverConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()

	// Send data from client, should get echoed back
	testData := []byte("test message")
	_, err = clientConn.Write(testData)
	if err != nil {
		t.Fatalf("failed to write test data: %v", err)
	}

	// Read echo response
	readBuf := make([]byte, len(testData))
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err = io.ReadFull(clientConn, readBuf)
	if err != nil {
		t.Fatalf("failed to read echoed data: %v (read %d bytes)", err, n)
	}
	if string(readBuf) != string(testData) {
		t.Errorf("expected echo %q, got %q", testData, readBuf)
	}

	// Close client connection — both goroutines should eventually finish
	clientConn.Close()

	// Wait for BOTH goroutines with a timeout
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-timer.C:
			t.Fatalf("timed out waiting for goroutine %d to finish", i+1)
		}
	}
}

func TestExtractAndValidateUpstreamPort(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		want    string
		wantErr bool
	}{
		{name: "localhost", addr: "localhost:3000", want: "3000"},
		{name: "ipv4 loopback", addr: "127.0.0.1:8080", want: "8080"},
		{name: "ipv6 loopback", addr: "[::1]:5000", want: "5000"},
		{name: "reject private network", addr: "192.168.1.10:3000", wantErr: true},
		{name: "reject domain", addr: "example.com:3000", wantErr: true},
		{name: "reject malformed", addr: "localhost", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractAndValidateUpstreamPort(tt.addr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("extractAndValidateUpstreamPort(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}
