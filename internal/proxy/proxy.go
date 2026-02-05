// internal/proxy/proxy.go
package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
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
	io.Copy(w, resp.Body)
}

func isWebSocket(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket"
}

// WebSocket idle timeout (1 hour max for dev servers)
const wsIdleTimeout = 1 * time.Hour

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

	// SECURITY: Set deadline to prevent zombie connections
	deadline := time.Now().Add(wsIdleTimeout)
	clientConn.SetDeadline(deadline)
	upstreamConn.SetDeadline(deadline)

	// Forward the original request
	r.Write(upstreamConn)

	// Bidirectional copy
	done := make(chan struct{}, 2)

	go func() {
		io.Copy(upstreamConn, clientConn)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(clientConn, upstreamConn)
		done <- struct{}{}
	}()

	<-done
	<-done // Wait for both goroutines to complete
}
