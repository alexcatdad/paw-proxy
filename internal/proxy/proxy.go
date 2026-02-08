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

	"github.com/alexcatdad/paw-proxy/internal/errorpage"
)

type Proxy struct {
	transport *http.Transport
}

func isLoopbackHost(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

func extractAndValidateUpstreamPort(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("proxy: split host/port: %w", err)
	}
	if !isLoopbackHost(host) {
		return "", fmt.Errorf("proxy: refusing connection to non-local host %s", host)
	}
	return port, nil
}

func dialLoopbackPort(port string, timeout time.Duration) (net.Conn, error) {
	conn, ipv4Err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", port), timeout)
	if ipv4Err == nil {
		return conn, nil
	}
	conn, ipv6Err := net.DialTimeout("tcp", net.JoinHostPort("::1", port), timeout)
	if ipv6Err == nil {
		return conn, nil
	}
	return nil, fmt.Errorf("upstream unreachable: IPv4: %v, IPv6: %v", ipv4Err, ipv6Err)
}

func New() *Proxy {
	return &Proxy{
		transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				port, err := extractAndValidateUpstreamPort(addr)
				if err != nil {
					return nil, err
				}
				return dialLoopbackPort(port, 2*time.Second)
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
	"Trailer",
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
		serveUpstreamError(w, r.Host, upstream, err)
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

func serveUpstreamError(w http.ResponseWriter, host string, upstream string, err error) {
	log.Printf("proxy: upstream error for %s -> %s: %v", host, upstream, err)
	errorpage.UpstreamDown(w, host, upstream)
}

func isWebSocket(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket" &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// SECURITY: wsIdleTimeout limits how long an idle WebSocket connection can
// remain open. This prevents zombie connections from accumulating when
// clients disconnect without a proper close handshake.
const wsIdleTimeout = 1 * time.Hour

// idleTimeoutConn wraps a net.Conn and resets the deadline on every Read
// or Write. This converts an absolute deadline into an idle timeout: the
// connection only expires if no data flows for the timeout duration.
type idleTimeoutConn struct {
	net.Conn
	timeout time.Duration
}

func (c *idleTimeoutConn) Read(b []byte) (int, error) {
	if err := c.Conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, fmt.Errorf("set read deadline: %w", err)
	}
	return c.Conn.Read(b)
}

func (c *idleTimeoutConn) Write(b []byte) (int, error) {
	if err := c.Conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, fmt.Errorf("set write deadline: %w", err)
	}
	return c.Conn.Write(b)
}

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
	port, err := extractAndValidateUpstreamPort(upstream)
	if err != nil {
		log.Printf("websocket: upstream validation failed: %v", err)
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	upstreamConn, err := dialLoopbackPort(port, 5*time.Second)
	if err != nil {
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer upstreamConn.Close()

	// Wrap connections with idle timeout instead of absolute deadline.
	// Each Read/Write resets the deadline, so the connection stays open
	// as long as data is flowing and only times out after inactivity.
	clientIdle := &idleTimeoutConn{Conn: clientConn, timeout: wsIdleTimeout}
	upstreamIdle := &idleTimeoutConn{Conn: upstreamConn, timeout: wsIdleTimeout}

	// Forward the original request
	r.Write(upstreamConn)

	// Bidirectional copy — wait for BOTH goroutines to finish to avoid
	// goroutine leaks. When one direction's io.Copy returns (client
	// disconnected or upstream closed), we close the write side of the
	// other connection to unblock the other io.Copy.
	done := make(chan struct{}, 2)

	go func() {
		if _, err := io.Copy(upstreamIdle, clientIdle); err != nil {
			log.Printf("websocket: client->upstream copy: %v", err)
		}
		if tc, ok := upstreamConn.(*net.TCPConn); ok {
			if err := tc.CloseWrite(); err != nil {
				log.Printf("websocket: upstream CloseWrite: %v", err)
			}
		}
		done <- struct{}{}
	}()

	go func() {
		if _, err := io.Copy(clientIdle, upstreamIdle); err != nil {
			log.Printf("websocket: upstream->client copy: %v", err)
		}
		if tc, ok := clientConn.(*net.TCPConn); ok {
			if err := tc.CloseWrite(); err != nil {
				log.Printf("websocket: client CloseWrite: %v", err)
			}
		}
		done <- struct{}{}
	}()

	<-done
	<-done
}
