// internal/proxy/proxy.go
package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
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
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}

				// SECURITY: Defense-in-depth — reject non-loopback addresses even though
				// the API layer already validates upstream to localhost-only.
				ip := net.ParseIP(host)
				if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
					return nil, fmt.Errorf("proxy: refusing connection to non-local host %s", host)
				}

				// Try IPv4 first, then IPv6 — report both errors on failure
				conn, ipv4Err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", port), 2*time.Second)
				if ipv4Err == nil {
					return conn, nil
				}
				conn, ipv6Err := net.DialTimeout("tcp", net.JoinHostPort("::1", port), 2*time.Second)
				if ipv6Err == nil {
					return conn, nil
				}
				return nil, fmt.Errorf("upstream unreachable: IPv4: %v, IPv6: %v", ipv4Err, ipv6Err)
			},
			MaxIdleConns:       100,
			IdleConnTimeout:    90 * time.Second,
			DisableCompression: true,
		},
	}
}

// hopByHopHeaders are headers that apply to a single transport-level connection
// and must not be forwarded by proxies (RFC 2616 Section 13.5.1).
var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"TE",
	"Trailers",
	"Transfer-Encoding",
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
	outReq.RequestURI = ""
	// NOTE: We intentionally do NOT set outReq.Host = upstream.
	// The original Host header from the client is preserved so upstream
	// servers see the expected hostname (e.g. "myapp.test").

	// Strip hop-by-hop headers before forwarding
	toRemove := make([]string, len(hopByHopHeaders))
	copy(toRemove, hopByHopHeaders)
	if connHeader := outReq.Header.Get("Connection"); connHeader != "" {
		for _, h := range strings.Split(connHeader, ",") {
			toRemove = append(toRemove, strings.TrimSpace(h))
		}
	}
	for _, h := range toRemove {
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
		log.Printf("proxy: response copy: %v", err)
	}
}

func isWebSocket(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket" &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// WebSocket idle timeout (1 hour max for dev servers)
const wsIdleTimeout = 1 * time.Hour

func (p *Proxy) handleWebSocket(w http.ResponseWriter, r *http.Request, upstream string) {
	// SECURITY: Validate WebSocket upgrade request per RFC 6455 Section 4.1
	if r.Header.Get("Sec-WebSocket-Key") == "" || r.Header.Get("Sec-WebSocket-Version") != "13" {
		http.Error(w, "invalid WebSocket upgrade request", http.StatusBadRequest)
		return
	}

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
}
