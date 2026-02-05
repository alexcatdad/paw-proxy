// internal/proxy/proxy.go
package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type Proxy struct {
	transport *http.Transport
}

func New() *Proxy {
	return &Proxy{
		transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Try IPv4 first, then IPv6 (Happy Eyeballs)
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}

				// If host is "localhost", try both
				if host == "localhost" {
					// Try IPv4
					conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", port), 2*time.Second)
					if err == nil {
						return conn, nil
					}
					// Try IPv6
					return net.DialTimeout("tcp", net.JoinHostPort("::1", port), 2*time.Second)
				}

				return net.DialTimeout("tcp", addr, 5*time.Second)
			},
			MaxIdleConns:       100,
			IdleConnTimeout:    90 * time.Second,
			DisableCompression: true,
		},
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request, upstream string) {
	// Check for WebSocket upgrade
	if isWebSocket(r) {
		p.handleWebSocket(w, r, upstream)
		return
	}

	// Create outbound request
	outReq := r.Clone(r.Context())
	outReq.URL.Scheme = "http"
	outReq.URL.Host = upstream
	// Preserve original Host header for upstream virtual hosting (URL.Host is used for dialing)
	outReq.RequestURI = ""

	// SECURITY: Strip hop-by-hop headers (RFC 7230 §6.1)
	// These are connection-specific and must not be forwarded to upstream
	hopByHopHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"TE",
		"Trailers",
		"Transfer-Encoding",
	}
	for _, h := range hopByHopHeaders {
		outReq.Header.Del(h)
	}

	// Set forwarding headers
	if clientIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		outReq.Header.Set("X-Forwarded-For", clientIP)
	}
	outReq.Header.Set("X-Forwarded-Proto", "https")
	outReq.Header.Set("X-Forwarded-Host", r.Host)

	// Send request
	resp, err := p.transport.RoundTrip(outReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		// Note: Headers already sent, cannot return error to client
		fmt.Fprintf(os.Stderr, "proxy: response copy error: %v\n", err)
	}
}

func isWebSocket(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket"
}

// WebSocket idle timeout (1 hour max for dev servers)
const wsIdleTimeout = 1 * time.Hour

// idleTimeoutConn wraps a net.Conn to reset the deadline on each Read/Write operation.
// This enables idle-based timeouts instead of absolute deadlines, keeping active connections
// alive indefinitely while closing idle ones.
type idleTimeoutConn struct {
	net.Conn
	timeout time.Duration
}

func (c *idleTimeoutConn) Read(b []byte) (int, error) {
	// SECURITY: Reset deadline on each read to prevent zombie connections while allowing active ones
	if err := c.Conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, fmt.Errorf("failed to set read deadline: %w", err)
	}
	return c.Conn.Read(b)
}

func (c *idleTimeoutConn) Write(b []byte) (int, error) {
	// SECURITY: Reset deadline on each write to prevent zombie connections while allowing active ones
	if err := c.Conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, fmt.Errorf("failed to set write deadline: %w", err)
	}
	return c.Conn.Write(b)
}

func (p *Proxy) handleWebSocket(w http.ResponseWriter, r *http.Request, upstream string) {
	// Hijack the connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Connect to upstream
	upstreamConn, err := net.DialTimeout("tcp", upstream, 5*time.Second)
	if err != nil {
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer upstreamConn.Close()

	// Wrap connections with idle timeout that resets on each Read/Write
	clientIdle := &idleTimeoutConn{Conn: clientConn, timeout: wsIdleTimeout}
	upstreamIdle := &idleTimeoutConn{Conn: upstreamConn, timeout: wsIdleTimeout}

	// Forward the original request
	r.Write(upstreamIdle)

	// Bidirectional copy
	done := make(chan struct{}, 2)

	go func() {
		if _, err := io.Copy(upstreamIdle, clientIdle); err != nil {
			fmt.Fprintf(os.Stderr, "proxy: websocket client->upstream error: %v\n", err)
		}
		done <- struct{}{}
	}()

	go func() {
		if _, err := io.Copy(clientIdle, upstreamIdle); err != nil {
			fmt.Fprintf(os.Stderr, "proxy: websocket upstream->client error: %v\n", err)
		}
		done <- struct{}{}
	}()

	<-done
	<-done // Wait for both goroutines to complete
}
