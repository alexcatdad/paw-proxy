# paw-proxy Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a zero-config HTTPS proxy for local macOS development with two binaries: `paw-proxy` (daemon/setup) and `up` (command wrapper).

**Architecture:** Single daemon process handles DNS (port 9353), HTTP/HTTPS proxy (ports 80/443 via launchd), and control API (unix socket). `up` binary communicates with daemon via socket to register/deregister routes dynamically.

**Tech Stack:** Go 1.22, stdlib net/http, crypto/tls, no external dependencies except `github.com/miekg/dns` for DNS server.

---

## Phase 1: Core Infrastructure

### Task 1: DNS Server

**Files:**
- Create: `internal/dns/server.go`
- Create: `internal/dns/server_test.go`

**Step 1: Write the failing test**

```go
// internal/dns/server_test.go
package dns

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestDNSServer_ResolvesTestDomain(t *testing.T) {
	srv, err := NewServer("127.0.0.1:19353", "test")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.Stop()

	go srv.Start()

	// Query the server
	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion("myapp.test.", dns.TypeA)

	r, _, err := c.Exchange(m, "127.0.0.1:19353")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if len(r.Answer) == 0 {
		t.Fatal("expected answer, got none")
	}

	a, ok := r.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected A record, got %T", r.Answer[0])
	}

	if !a.A.Equal(net.ParseIP("127.0.0.1")) {
		t.Errorf("expected 127.0.0.1, got %v", a.A)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/alex/REPOS/alexcatdad/paw-proxy && go mod tidy && go test -v ./internal/dns/...`
Expected: FAIL with "no Go files in directory" or similar

**Step 3: Write minimal implementation**

```go
// internal/dns/server.go
package dns

import (
	"fmt"
	"net"
	"strings"

	"github.com/miekg/dns"
)

type Server struct {
	addr   string
	tld    string
	server *dns.Server
}

func NewServer(addr, tld string) (*Server, error) {
	s := &Server{
		addr: addr,
		tld:  tld,
	}

	s.server = &dns.Server{
		Addr: addr,
		Net:  "udp",
		Handler: dns.HandlerFunc(s.handleRequest),
	}

	return s, nil
}

func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

func (s *Server) Stop() error {
	return s.server.Shutdown()
}

func (s *Server) handleRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		if !strings.HasSuffix(strings.ToLower(q.Name), "."+s.tld+".") {
			continue
		}

		switch q.Qtype {
		case dns.TypeA:
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   q.Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				A: net.ParseIP("127.0.0.1"),
			}
			m.Answer = append(m.Answer, rr)

		case dns.TypeAAAA:
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   q.Name,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				AAAA: net.ParseIP("::1"),
			}
			m.Answer = append(m.Answer, rr)
		}
	}

	w.WriteMsg(m)
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/alex/REPOS/alexcatdad/paw-proxy && go mod tidy && go test -v ./internal/dns/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/dns/ go.mod go.sum
git commit -m "feat(dns): add DNS server for .test domain resolution"
```

---

### Task 2: SSL Certificate Generation

**Files:**
- Create: `internal/ssl/ca.go`
- Create: `internal/ssl/ca_test.go`
- Create: `internal/ssl/cert.go`
- Create: `internal/ssl/cert_test.go`

**Step 1: Write the failing test for CA generation**

```go
// internal/ssl/ca_test.go
package ssl

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateCA(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "ca.crt")
	keyPath := filepath.Join(tmpDir, "ca.key")

	err := GenerateCA(certPath, keyPath)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	// Verify files exist
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Error("CA cert file not created")
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("CA key file not created")
	}

	// Load and verify CA
	ca, err := LoadCA(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadCA failed: %v", err)
	}

	if !ca.Leaf.IsCA {
		t.Error("certificate is not a CA")
	}
	if ca.Leaf.Subject.CommonName != "paw-proxy CA" {
		t.Errorf("unexpected CN: %s", ca.Leaf.Subject.CommonName)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/ssl/...`
Expected: FAIL

**Step 3: Write CA implementation**

```go
// internal/ssl/ca.go
package ssl

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"
)

func GenerateCA(certPath, keyPath string) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generating RSA key: %w", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(10 * 365 * 24 * time.Hour) // 10 years

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generating serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"paw-proxy"},
			CommonName:   "paw-proxy CA",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, priv.Public(), priv)
	if err != nil {
		return fmt.Errorf("creating certificate: %w", err)
	}

	// Write cert
	certOut, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("creating cert file: %w", err)
	}
	defer certOut.Close()
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	// Write key
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("creating key file: %w", err)
	}
	defer keyOut.Close()
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return nil
}

func LoadCA(certPath, keyPath string) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("loading key pair: %w", err)
	}

	cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %w", err)
	}

	return &cert, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./internal/ssl/...`
Expected: PASS

**Step 5: Write test for per-domain cert generation**

```go
// internal/ssl/cert_test.go
package ssl

import (
	"crypto/tls"
	"path/filepath"
	"testing"
)

func TestCertCache_GeneratesCertForDomain(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "ca.crt")
	keyPath := filepath.Join(tmpDir, "ca.key")

	err := GenerateCA(certPath, keyPath)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	ca, err := LoadCA(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadCA failed: %v", err)
	}

	cache := NewCertCache(ca)

	// Simulate TLS handshake
	hello := &tls.ClientHelloInfo{
		ServerName: "myapp.test",
	}

	cert, err := cache.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}

	if cert.Leaf.Subject.CommonName != "myapp.test" {
		t.Errorf("unexpected CN: %s", cert.Leaf.Subject.CommonName)
	}

	// Second call should return cached cert
	cert2, err := cache.GetCertificate(hello)
	if err != nil {
		t.Fatalf("second GetCertificate failed: %v", err)
	}

	if cert != cert2 {
		t.Error("expected same cert from cache")
	}
}
```

**Step 6: Write cert cache implementation**

```go
// internal/ssl/cert.go
package ssl

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"sync"
	"time"
)

type CertCache struct {
	ca    *tls.Certificate
	cache map[string]*tls.Certificate
	mu    sync.RWMutex
}

func NewCertCache(ca *tls.Certificate) *CertCache {
	return &CertCache{
		ca:    ca,
		cache: make(map[string]*tls.Certificate),
	}
}

func (c *CertCache) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := hello.ServerName

	c.mu.RLock()
	if cert, ok := c.cache[name]; ok {
		c.mu.RUnlock()
		return cert, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if cert, ok := c.cache[name]; ok {
		return cert, nil
	}

	cert, err := c.generateCert(name)
	if err != nil {
		return nil, err
	}

	c.cache[name] = cert
	return cert, nil
}

func (c *CertCache) generateCert(name string) (*tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour) // 1 year

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generating serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"paw-proxy"},
			CommonName:   name,
		},
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{name},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, c.ca.Leaf, priv.Public(), c.ca.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("creating certificate: %w", err)
	}

	leaf, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %w", err)
	}

	return &tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
		Leaf:        leaf,
	}, nil
}
```

**Step 7: Run tests**

Run: `go test -v ./internal/ssl/...`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/ssl/
git commit -m "feat(ssl): add CA generation and per-domain cert cache"
```

---

### Task 3: Route Registry

**Files:**
- Create: `internal/api/routes.go`
- Create: `internal/api/routes_test.go`

**Step 1: Write the failing test**

```go
// internal/api/routes_test.go
package api

import (
	"testing"
	"time"
)

func TestRouteRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRouteRegistry(30 * time.Second)

	err := r.Register("myapp", "localhost:3000", "/path/to/project")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	route, ok := r.Lookup("myapp")
	if !ok {
		t.Fatal("Lookup failed")
	}

	if route.Name != "myapp" {
		t.Errorf("expected myapp, got %s", route.Name)
	}
	if route.Upstream != "localhost:3000" {
		t.Errorf("expected localhost:3000, got %s", route.Upstream)
	}
}

func TestRouteRegistry_ConflictFromSameDir(t *testing.T) {
	r := NewRouteRegistry(30 * time.Second)

	err := r.Register("myapp", "localhost:3000", "/path/to/project")
	if err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	// Same name, same dir = error
	err = r.Register("myapp", "localhost:4000", "/path/to/project")
	if err == nil {
		t.Fatal("expected error for conflict from same dir")
	}
}

func TestRouteRegistry_ConflictFromDifferentDir(t *testing.T) {
	r := NewRouteRegistry(30 * time.Second)

	err := r.Register("myapp", "localhost:3000", "/path/to/project1")
	if err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	// Same name, different dir = returns conflict info
	err = r.Register("myapp", "localhost:4000", "/path/to/project2")
	if err == nil {
		t.Fatal("expected error for conflict")
	}

	conflict, ok := err.(*ConflictError)
	if !ok {
		t.Fatalf("expected ConflictError, got %T", err)
	}
	if conflict.ExistingDir != "/path/to/project1" {
		t.Errorf("unexpected existing dir: %s", conflict.ExistingDir)
	}
}

func TestRouteRegistry_Heartbeat(t *testing.T) {
	r := NewRouteRegistry(100 * time.Millisecond)

	err := r.Register("myapp", "localhost:3000", "/path")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Heartbeat should succeed
	err = r.Heartbeat("myapp")
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)
	r.Cleanup()

	_, ok := r.Lookup("myapp")
	if ok {
		t.Error("expected route to be expired")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/api/...`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/api/routes.go
package api

import (
	"fmt"
	"sync"
	"time"
)

type Route struct {
	Name          string    `json:"name"`
	Upstream      string    `json:"upstream"`
	Dir           string    `json:"dir"`
	Registered    time.Time `json:"registered"`
	LastHeartbeat time.Time `json:"lastHeartbeat"`
}

type ConflictError struct {
	Name        string
	ExistingDir string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("route %q already registered from %s", e.Name, e.ExistingDir)
}

type RouteRegistry struct {
	routes  map[string]*Route
	timeout time.Duration
	mu      sync.RWMutex
}

func NewRouteRegistry(timeout time.Duration) *RouteRegistry {
	return &RouteRegistry{
		routes:  make(map[string]*Route),
		timeout: timeout,
	}
}

func (r *RouteRegistry) Register(name, upstream, dir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.routes[name]; ok {
		return &ConflictError{
			Name:        name,
			ExistingDir: existing.Dir,
		}
	}

	now := time.Now()
	r.routes[name] = &Route{
		Name:          name,
		Upstream:      upstream,
		Dir:           dir,
		Registered:    now,
		LastHeartbeat: now,
	}

	return nil
}

func (r *RouteRegistry) Deregister(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.routes[name]; ok {
		delete(r.routes, name)
		return true
	}
	return false
}

func (r *RouteRegistry) Lookup(name string) (*Route, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	route, ok := r.routes[name]
	return route, ok
}

func (r *RouteRegistry) LookupByHost(host string) (*Route, bool) {
	// host is like "myapp.test" or "myapp.test:443"
	// Extract just the name part
	name := host
	for i, c := range host {
		if c == '.' || c == ':' {
			name = host[:i]
			break
		}
	}

	return r.Lookup(name)
}

func (r *RouteRegistry) Heartbeat(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	route, ok := r.routes[name]
	if !ok {
		return fmt.Errorf("route %q not found", name)
	}

	route.LastHeartbeat = time.Now()
	return nil
}

func (r *RouteRegistry) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-r.timeout)
	for name, route := range r.routes {
		if route.LastHeartbeat.Before(cutoff) {
			delete(r.routes, name)
		}
	}
}

func (r *RouteRegistry) List() []*Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routes := make([]*Route, 0, len(r.routes))
	for _, route := range r.routes {
		routes = append(routes, route)
	}
	return routes
}
```

**Step 4: Run tests**

Run: `go test -v ./internal/api/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/
git commit -m "feat(api): add route registry with heartbeat expiry"
```

---

### Task 4: Reverse Proxy with WebSocket Support

**Files:**
- Create: `internal/proxy/proxy.go`
- Create: `internal/proxy/proxy_test.go`

**Step 1: Write the failing test**

```go
// internal/proxy/proxy_test.go
package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
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
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/proxy/...`
Expected: FAIL

**Step 3: Write implementation**

```go
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
			MaxIdleConns:        100,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  true,
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
	outReq.Host = upstream
	outReq.RequestURI = ""

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
```

**Step 4: Run tests**

Run: `go test -v ./internal/proxy/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/proxy/
git commit -m "feat(proxy): add reverse proxy with WebSocket and IPv4/IPv6 support"
```

---

### Task 5: Control API Server

**Files:**
- Create: `internal/api/server.go`
- Create: `internal/api/server_test.go`

**Step 1: Write the failing test**

```go
// internal/api/server_test.go
package api

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"os"
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
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/api/...`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/api/server.go
package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type Server struct {
	socketPath string
	registry   *RouteRegistry
	server     *http.Server
	listener   net.Listener
	startTime  time.Time
}

func NewServer(socketPath string, registry *RouteRegistry) *Server {
	s := &Server{
		socketPath: socketPath,
		registry:   registry,
		startTime:  time.Now(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /routes", s.handleRegister)
	mux.HandleFunc("DELETE /routes/{name}", s.handleDeregister)
	mux.HandleFunc("POST /routes/{name}/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("GET /routes", s.handleList)
	mux.HandleFunc("GET /health", s.handleHealth)

	s.server = &http.Server{Handler: mux}

	return s
}

func (s *Server) Start() error {
	// Remove existing socket
	os.Remove(s.socketPath)

	var err error
	s.listener, err = net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}

	// Make socket accessible
	os.Chmod(s.socketPath, 0666)

	return s.server.Serve(s.listener)
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

type RegisterRequest struct {
	Name     string `json:"name"`
	Upstream string `json:"upstream"`
	Dir      string `json:"dir"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := s.registry.Register(req.Name, req.Upstream, req.Dir)
	if err != nil {
		if conflict, ok := err.(*ConflictError); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{
				"error":       "conflict",
				"existingDir": conflict.ExistingDir,
			})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDeregister(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	if s.registry.Deregister(name) {
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	if err := s.registry.Heartbeat(name); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	routes := s.registry.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(routes)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startTime)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"version": "1.0.0",
		"uptime":  uptime.String(),
	})
}
```

**Step 4: Update test with context import and run**

Run: `go test -v ./internal/api/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/
git commit -m "feat(api): add control API server over unix socket"
```

---

## Phase 2: Main Binaries

### Task 6: paw-proxy daemon

**Files:**
- Create: `internal/daemon/daemon.go`
- Create: `cmd/paw-proxy/main.go`

**Step 1: Write daemon orchestrator**

```go
// internal/daemon/daemon.go
package daemon

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/alexcatdad/paw-proxy/internal/api"
	"github.com/alexcatdad/paw-proxy/internal/dns"
	"github.com/alexcatdad/paw-proxy/internal/proxy"
	"github.com/alexcatdad/paw-proxy/internal/ssl"
)

type Config struct {
	DNSPort      int
	HTTPPort     int
	HTTPSPort    int
	TLD          string
	SupportDir   string
	SocketPath   string
	LogPath      string
}

func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	supportDir := filepath.Join(homeDir, "Library", "Application Support", "paw-proxy")

	return &Config{
		DNSPort:    9353,
		HTTPPort:   80,
		HTTPSPort:  443,
		TLD:        "test",
		SupportDir: supportDir,
		SocketPath: filepath.Join(supportDir, "paw-proxy.sock"),
		LogPath:    filepath.Join(homeDir, "Library", "Logs", "paw-proxy.log"),
	}
}

type Daemon struct {
	config    *Config
	dnsServer *dns.Server
	registry  *api.RouteRegistry
	apiServer *api.Server
	certCache *ssl.CertCache
	proxy     *proxy.Proxy
}

func New(config *Config) (*Daemon, error) {
	// Ensure support directory exists
	if err := os.MkdirAll(config.SupportDir, 0700); err != nil {
		return nil, fmt.Errorf("creating support dir: %w", err)
	}

	// Load or create CA
	certPath := filepath.Join(config.SupportDir, "ca.crt")
	keyPath := filepath.Join(config.SupportDir, "ca.key")

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("CA not found - run 'paw-proxy setup' first")
	}

	ca, err := ssl.LoadCA(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("loading CA: %w", err)
	}

	// Create DNS server
	dnsAddr := fmt.Sprintf("127.0.0.1:%d", config.DNSPort)
	dnsServer, err := dns.NewServer(dnsAddr, config.TLD)
	if err != nil {
		return nil, fmt.Errorf("creating DNS server: %w", err)
	}

	// Create route registry with 30s heartbeat timeout
	registry := api.NewRouteRegistry(30 * time.Second)

	// Create API server
	apiServer := api.NewServer(config.SocketPath, registry)

	return &Daemon{
		config:    config,
		dnsServer: dnsServer,
		registry:  registry,
		apiServer: apiServer,
		certCache: ssl.NewCertCache(ca),
		proxy:     proxy.New(),
	}, nil
}

func (d *Daemon) Run() error {
	// Start DNS server
	go func() {
		log.Printf("DNS server listening on 127.0.0.1:%d", d.config.DNSPort)
		if err := d.dnsServer.Start(); err != nil {
			log.Printf("DNS server error: %v", err)
		}
	}()

	// Start API server
	go func() {
		log.Printf("API server listening on %s", d.config.SocketPath)
		if err := d.apiServer.Start(); err != nil {
			log.Printf("API server error: %v", err)
		}
	}()

	// Start cleanup routine
	go d.cleanupRoutine()

	// Start HTTP redirect server
	go d.serveHTTP()

	// Start HTTPS server
	return d.serveHTTPS()
}

func (d *Daemon) cleanupRoutine() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		d.registry.Cleanup()
	}
}

func (d *Daemon) serveHTTP() {
	addr := fmt.Sprintf(":%d", d.config.HTTPPort)
	log.Printf("HTTP redirect server listening on %s", addr)

	server := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := "https://" + r.Host + r.URL.Path
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		}),
	}
	server.ListenAndServe()
}

func (d *Daemon) serveHTTPS() error {
	addr := fmt.Sprintf(":%d", d.config.HTTPSPort)
	log.Printf("HTTPS server listening on %s", addr)

	tlsConfig := &tls.Config{
		GetCertificate: d.certCache.GetCertificate,
	}

	listener, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}

	server := &http.Server{
		Handler: http.HandlerFunc(d.handleRequest),
	}

	return server.Serve(listener)
}

func (d *Daemon) handleRequest(w http.ResponseWriter, r *http.Request) {
	route, ok := d.registry.LookupByHost(r.Host)
	if !ok {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, "No app registered for %s\n\nRun: up -n %s <your-dev-command>\n", r.Host, extractName(r.Host))
		return
	}

	d.proxy.ServeHTTP(w, r, route.Upstream)
}

func extractName(host string) string {
	for i, c := range host {
		if c == '.' || c == ':' {
			return host[:i]
		}
	}
	return host
}
```

**Step 2: Write main.go for paw-proxy**

```go
// cmd/paw-proxy/main.go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/alexcatdad/paw-proxy/internal/daemon"
)

func main() {
	// Subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "setup":
			cmdSetup()
			return
		case "uninstall":
			cmdUninstall()
			return
		case "status":
			cmdStatus()
			return
		case "run":
			cmdRun()
			return
		case "version":
			fmt.Println("paw-proxy version 1.0.0")
			return
		}
	}

	// Default: show usage
	fmt.Println("Usage: paw-proxy <command>")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  setup      Configure DNS, CA, and install daemon (requires sudo)")
	fmt.Println("  uninstall  Remove all paw-proxy components")
	fmt.Println("  status     Show daemon status and registered routes")
	fmt.Println("  run        Run daemon in foreground (for launchd)")
	fmt.Println("  version    Show version")
	os.Exit(1)
}

func cmdRun() {
	config := daemon.DefaultConfig()

	// Setup logging
	logFile, err := os.OpenFile(config.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	d, err := daemon.New(config)
	if err != nil {
		log.Fatalf("Failed to create daemon: %v", err)
	}

	log.Println("paw-proxy daemon starting...")
	if err := d.Run(); err != nil {
		log.Fatalf("Daemon error: %v", err)
	}
}

func cmdSetup() {
	// Will implement in Task 7
	fmt.Println("setup command - to be implemented")
}

func cmdUninstall() {
	// Will implement in Task 8
	fmt.Println("uninstall command - to be implemented")
}

func cmdStatus() {
	// Will implement in Task 9
	fmt.Println("status command - to be implemented")
}
```

**Step 3: Build and verify compilation**

Run: `go build -o paw-proxy ./cmd/paw-proxy`
Expected: Binary created successfully

**Step 4: Commit**

```bash
git add internal/daemon/ cmd/paw-proxy/
git commit -m "feat(daemon): add main daemon orchestrator and CLI entry point"
```

---

### Task 7: Setup Command

**Files:**
- Create: `internal/setup/setup_darwin.go`
- Modify: `cmd/paw-proxy/main.go`

**Step 1: Write setup implementation**

```go
// internal/setup/setup_darwin.go
package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

type Config struct {
	SupportDir string
	BinaryPath string
	DNSPort    int
	TLD        string
}

func Run(config *Config) error {
	fmt.Println("paw-proxy setup")
	fmt.Println("================")

	// 1. Create support directory
	fmt.Printf("\n[1/5] Creating support directory...\n")
	if err := os.MkdirAll(config.SupportDir, 0700); err != nil {
		return fmt.Errorf("creating support dir: %w", err)
	}
	fmt.Printf("  ✓ %s\n", config.SupportDir)

	// 2. Generate CA
	fmt.Printf("\n[2/5] Generating CA certificate...\n")
	certPath := filepath.Join(config.SupportDir, "ca.crt")
	keyPath := filepath.Join(config.SupportDir, "ca.key")

	if _, err := os.Stat(certPath); err == nil {
		fmt.Printf("  ✓ CA already exists\n")
	} else {
		// Import ssl package and generate
		if err := generateCA(certPath, keyPath); err != nil {
			return fmt.Errorf("generating CA: %w", err)
		}
		fmt.Printf("  ✓ Generated CA certificate\n")
	}

	// 3. Trust CA in keychain
	fmt.Printf("\n[3/5] Adding CA to keychain...\n")
	fmt.Printf("  Note: You may be prompted for your password\n")
	if err := trustCA(certPath); err != nil {
		return fmt.Errorf("trusting CA: %w", err)
	}
	fmt.Printf("  ✓ CA trusted in login keychain\n")

	// 4. Create resolver file
	fmt.Printf("\n[4/5] Configuring DNS resolver...\n")
	if err := configureResolver(config.TLD, config.DNSPort); err != nil {
		return fmt.Errorf("configuring resolver: %w", err)
	}
	fmt.Printf("  ✓ /etc/resolver/%s created\n", config.TLD)

	// 5. Install LaunchAgent
	fmt.Printf("\n[5/5] Installing daemon...\n")
	if err := installLaunchAgent(config); err != nil {
		return fmt.Errorf("installing LaunchAgent: %w", err)
	}
	fmt.Printf("  ✓ LaunchAgent installed and started\n")

	fmt.Println("\n================")
	fmt.Println("Setup complete!")
	fmt.Println("")
	fmt.Println("Note: macOS may show a 'Background Items Added' notification. This is normal.")
	fmt.Println("")
	fmt.Println("Firefox users: Install 'nss' for certificate trust:")
	fmt.Println("  brew install nss")
	fmt.Println("  paw-proxy setup  (re-run to update Firefox)")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  up bun dev           # Start dev server with HTTPS")
	fmt.Println("  up -n myapp npm start # Custom domain name")

	return nil
}

func generateCA(certPath, keyPath string) error {
	// This will call into ssl.GenerateCA
	// For now, placeholder
	return nil
}

func trustCA(certPath string) error {
	// Get login keychain
	out, err := exec.Command("security", "login-keychain").Output()
	if err != nil {
		return fmt.Errorf("finding login keychain: %w", err)
	}

	keychain := strings.TrimSpace(strings.Trim(string(out), `"`))

	// Add trusted cert
	cmd := exec.Command("security", "add-trusted-cert", "-k", keychain, certPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func configureResolver(tld string, port int) error {
	resolverDir := "/etc/resolver"
	if err := os.MkdirAll(resolverDir, 0755); err != nil {
		return err
	}

	content := fmt.Sprintf("# Generated by paw-proxy\nnameserver 127.0.0.1\nport %d\n", port)
	path := filepath.Join(resolverDir, tld)

	return os.WriteFile(path, []byte(content), 0644)
}

var launchAgentTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>dev.paw-proxy</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.BinaryPath}}</string>
        <string>run</string>
    </array>
    <key>KeepAlive</key>
    <true/>
    <key>RunAtLoad</key>
    <true/>
    <key>Sockets</key>
    <dict>
        <key>http</key>
        <dict>
            <key>SockNodeName</key>
            <string>0.0.0.0</string>
            <key>SockServiceName</key>
            <string>80</string>
        </dict>
        <key>https</key>
        <dict>
            <key>SockNodeName</key>
            <string>0.0.0.0</string>
            <key>SockServiceName</key>
            <string>443</string>
        </dict>
    </dict>
</dict>
</plist>
`

func installLaunchAgent(config *Config) error {
	homeDir, _ := os.UserHomeDir()
	plistDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	plistPath := filepath.Join(plistDir, "dev.paw-proxy.plist")

	if err := os.MkdirAll(plistDir, 0755); err != nil {
		return err
	}

	// Unload existing if present
	exec.Command("launchctl", "unload", plistPath).Run()

	// Write plist
	f, err := os.Create(plistPath)
	if err != nil {
		return err
	}
	defer f.Close()

	tmpl, err := template.New("plist").Parse(launchAgentTemplate)
	if err != nil {
		return err
	}

	if err := tmpl.Execute(f, config); err != nil {
		return err
	}

	// Load plist
	return exec.Command("launchctl", "load", plistPath).Run()
}
```

**Step 2: Wire up setup command in main.go**

Update `cmdSetup()` in `cmd/paw-proxy/main.go`:

```go
func cmdSetup() {
	// Check for root/sudo
	if os.Geteuid() != 0 {
		fmt.Println("Error: setup requires sudo")
		fmt.Println("Run: sudo paw-proxy setup")
		os.Exit(1)
	}

	exe, _ := os.Executable()
	config := &setup.Config{
		SupportDir: daemon.DefaultConfig().SupportDir,
		BinaryPath: exe,
		DNSPort:    9353,
		TLD:        "test",
	}

	if err := setup.Run(config); err != nil {
		fmt.Printf("Setup failed: %v\n", err)
		os.Exit(1)
	}
}
```

**Step 3: Build and verify**

Run: `go build -o paw-proxy ./cmd/paw-proxy`
Expected: Compiles successfully

**Step 4: Commit**

```bash
git add internal/setup/ cmd/paw-proxy/
git commit -m "feat(setup): add setup command for DNS, CA, and LaunchAgent"
```

---

### Task 8: up Binary

**Files:**
- Create: `cmd/up/main.go`

**Step 1: Write up binary**

```go
// cmd/up/main.go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var (
	nameFlag = flag.String("n", "", "Custom app name (default: from package.json or directory)")
)

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Println("Usage: up [-n name] <command> [args...]")
		fmt.Println("")
		fmt.Println("Options:")
		fmt.Println("  -n name    Custom domain name (default: package.json name or directory)")
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  up bun dev")
		fmt.Println("  up -n myapp npm run dev")
		os.Exit(1)
	}

	// Get socket path
	homeDir, _ := os.UserHomeDir()
	socketPath := filepath.Join(homeDir, "Library", "Application Support", "paw-proxy", "paw-proxy.sock")
	caPath := filepath.Join(homeDir, "Library", "Application Support", "paw-proxy", "ca.crt")

	// Check if daemon is running
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		fmt.Println("Error: paw-proxy daemon not running")
		fmt.Println("Run: sudo paw-proxy setup")
		os.Exit(1)
	}

	// Find free port
	port, err := findFreePort()
	if err != nil {
		fmt.Printf("Error finding free port: %v\n", err)
		os.Exit(1)
	}

	// Determine app name
	name := determineName(*nameFlag)
	dir, _ := os.Getwd()

	// Register route
	client := socketClient(socketPath)
	err = registerRoute(client, name, fmt.Sprintf("localhost:%d", port), dir)
	if err != nil {
		// Check for conflict
		if conflictDir := extractConflictDir(err); conflictDir != "" {
			// Try directory name fallback
			dirName := filepath.Base(dir)
			if dirName != name {
				fmt.Printf("⚠️  %s.test already in use from %s\n", name, conflictDir)
				fmt.Printf("   Using %s.test instead\n", dirName)
				name = dirName
				err = registerRoute(client, name, fmt.Sprintf("localhost:%d", port), dir)
			}
		}
		if err != nil {
			fmt.Printf("Error registering route: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("🔗 Mapping https://%s.test -> localhost:%d...\n", name, port)
	fmt.Printf("🚀 Project is live at: https://%s.test\n", name)
	fmt.Println("------------------------------------------------")

	// Setup cleanup
	cleanup := func() {
		fmt.Printf("\n🛑 Removing mapping for %s.test...\n", name)
		deregisterRoute(client, name)
	}

	// Start heartbeat
	ctx, cancel := context.WithCancel(context.Background())
	go heartbeat(ctx, client, name)

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Build command
	args := flag.Args()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", port),
		fmt.Sprintf("APP_DOMAIN=%s.test", name),
		fmt.Sprintf("APP_URL=https://%s.test", name),
		"HTTPS=true",
		fmt.Sprintf("NODE_EXTRA_CA_CERTS=%s", caPath),
	)

	// Start command
	if err := cmd.Start(); err != nil {
		cleanup()
		fmt.Printf("Error starting command: %v\n", err)
		os.Exit(1)
	}

	// Wait for signal or command exit
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
	}()

	var exitCode int
	select {
	case sig := <-sigCh:
		// Forward signal to child
		cmd.Process.Signal(sig)
		// Wait for child with timeout
		select {
		case <-doneCh:
		case <-time.After(5 * time.Second):
			cmd.Process.Kill()
		}
	case err := <-doneCh:
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	cancel()
	cleanup()
	os.Exit(exitCode)
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func determineName(explicit string) string {
	if explicit != "" {
		return explicit
	}

	// Try package.json
	if data, err := os.ReadFile("package.json"); err == nil {
		var pkg struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(data, &pkg) == nil && pkg.Name != "" {
			return sanitizeName(pkg.Name)
		}
	}

	// Fall back to directory name
	dir, _ := os.Getwd()
	return sanitizeName(filepath.Base(dir))
}

func sanitizeName(name string) string {
	// Replace non-alphanumeric with dashes
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			result = append(result, c)
		} else {
			result = append(result, '-')
		}
	}
	return string(result)
}

func socketClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}
}

func registerRoute(client *http.Client, name, upstream, dir string) error {
	body, _ := json.Marshal(map[string]string{
		"name":     name,
		"upstream": upstream,
		"dir":      dir,
	})

	resp, err := client.Post("http://unix/routes", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		return fmt.Errorf("%s: %s", resp.Status, errResp["error"])
	}

	return nil
}

func deregisterRoute(client *http.Client, name string) {
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("http://unix/routes/%s", name), nil)
	client.Do(req)
}

func heartbeat(ctx context.Context, client *http.Client, name string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			req, _ := http.NewRequest("POST", fmt.Sprintf("http://unix/routes/%s/heartbeat", name), nil)
			client.Do(req)
		}
	}
}

func extractConflictDir(err error) string {
	// Parse error message for conflict info
	// This is a simplification - real impl would parse JSON response
	return ""
}
```

**Step 2: Add bytes import and build**

Run: `go build -o up ./cmd/up`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add cmd/up/
git commit -m "feat(up): add up command for wrapping dev servers"
```

---

## Phase 3: Polish & Testing

### Task 9: Status Command

**Files:**
- Modify: `cmd/paw-proxy/main.go`

**Step 1: Implement status command**

Add to `cmd/paw-proxy/main.go`:

```go
func cmdStatus() {
	homeDir, _ := os.UserHomeDir()
	socketPath := filepath.Join(homeDir, "Library", "Application Support", "paw-proxy", "paw-proxy.sock")

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 2 * time.Second,
	}

	// Check health
	resp, err := client.Get("http://unix/health")
	if err != nil {
		fmt.Println("Status: ❌ Daemon not running")
		fmt.Println("")
		fmt.Println("Run: sudo paw-proxy setup")
		return
	}
	defer resp.Body.Close()

	var health struct {
		Status  string `json:"status"`
		Version string `json:"version"`
		Uptime  string `json:"uptime"`
	}
	json.NewDecoder(resp.Body).Decode(&health)

	fmt.Printf("Status: ✅ Running (v%s, up %s)\n", health.Version, health.Uptime)
	fmt.Println("")

	// Get routes
	resp, err = client.Get("http://unix/routes")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var routes []struct {
		Name          string    `json:"name"`
		Upstream      string    `json:"upstream"`
		Dir           string    `json:"dir"`
		Registered    time.Time `json:"registered"`
		LastHeartbeat time.Time `json:"lastHeartbeat"`
	}
	json.NewDecoder(resp.Body).Decode(&routes)

	if len(routes) == 0 {
		fmt.Println("Routes: (none)")
	} else {
		fmt.Println("Routes:")
		for _, r := range routes {
			age := time.Since(r.Registered).Round(time.Second)
			fmt.Printf("  • %s.test -> %s (%s)\n", r.Name, r.Upstream, age)
			fmt.Printf("    Dir: %s\n", r.Dir)
		}
	}

	// CA info
	certPath := filepath.Join(homeDir, "Library", "Application Support", "paw-proxy", "ca.crt")
	if certData, err := os.ReadFile(certPath); err == nil {
		block, _ := pem.Decode(certData)
		if block != nil {
			cert, _ := x509.ParseCertificate(block.Bytes)
			if cert != nil {
				fmt.Println("")
				fmt.Printf("CA Expires: %s\n", cert.NotAfter.Format("2006-01-02"))
			}
		}
	}
}
```

**Step 2: Build and test**

Run: `go build -o paw-proxy ./cmd/paw-proxy && ./paw-proxy status`
Expected: Shows status (daemon not running if not set up)

**Step 3: Commit**

```bash
git add cmd/paw-proxy/
git commit -m "feat(status): add status command with route and CA info"
```

---

### Task 10: Uninstall Command

**Files:**
- Create: `internal/setup/uninstall_darwin.go`
- Modify: `cmd/paw-proxy/main.go`

**Step 1: Implement uninstall**

```go
// internal/setup/uninstall_darwin.go
package setup

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Uninstall(supportDir, tld string, fromBrew bool) error {
	homeDir, _ := os.UserHomeDir()
	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", "dev.paw-proxy.plist")
	resolverPath := filepath.Join("/etc/resolver", tld)

	fmt.Println("paw-proxy uninstall")
	fmt.Println("===================")

	// 1. Stop and remove LaunchAgent
	fmt.Printf("\n[1/3] Removing daemon...\n")
	exec.Command("launchctl", "unload", plistPath).Run()
	os.Remove(plistPath)
	fmt.Printf("  ✓ LaunchAgent removed\n")

	// 2. Remove resolver
	fmt.Printf("\n[2/3] Removing DNS resolver...\n")
	os.Remove(resolverPath)
	fmt.Printf("  ✓ /etc/resolver/%s removed\n", tld)

	// 3. Remove CA (prompt unless --brew)
	removeCA := fromBrew
	if !fromBrew {
		fmt.Printf("\n[3/3] Remove CA certificate from keychain? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		removeCA = strings.ToLower(strings.TrimSpace(answer)) == "y"
	}

	if removeCA {
		// Remove from keychain
		cmd := exec.Command("sh", "-c", `
			for sha in $(security find-certificate -a -c "paw-proxy CA" -Z | awk '/SHA-1/ {print $3}'); do
				security delete-certificate -Z $sha 2>/dev/null || true
			done
		`)
		cmd.Run()
		fmt.Printf("  ✓ CA removed from keychain\n")

		// Remove support directory
		os.RemoveAll(supportDir)
		fmt.Printf("  ✓ Support directory removed\n")
	} else {
		fmt.Printf("  ⏭  CA kept in keychain\n")
	}

	fmt.Println("\n===================")
	fmt.Println("Uninstall complete!")

	return nil
}
```

**Step 2: Wire up in main.go**

```go
func cmdUninstall() {
	brewFlag := false
	for _, arg := range os.Args[2:] {
		if arg == "--brew" {
			brewFlag = true
		}
	}

	config := daemon.DefaultConfig()
	if err := setup.Uninstall(config.SupportDir, "test", brewFlag); err != nil {
		fmt.Printf("Uninstall failed: %v\n", err)
		os.Exit(1)
	}
}
```

**Step 3: Commit**

```bash
git add internal/setup/ cmd/paw-proxy/
git commit -m "feat(uninstall): add uninstall command with optional CA removal"
```

---

### Task 11: Integration Test Script

**Files:**
- Create: `integration-tests.sh`

**Step 1: Write integration test script**

```bash
#!/bin/bash
set -e

echo "=== paw-proxy Integration Tests ==="
echo ""

# Test 1: Daemon is running
echo "[Test 1] Daemon health check..."
curl -s --unix-socket ~/Library/Application\ Support/paw-proxy/paw-proxy.sock http://unix/health | grep -q "ok"
echo "  ✓ Daemon is healthy"

# Test 2: Register a route
echo "[Test 2] Route registration..."
curl -s --unix-socket ~/Library/Application\ Support/paw-proxy/paw-proxy.sock \
  -X POST http://unix/routes \
  -H "Content-Type: application/json" \
  -d '{"name":"integration-test","upstream":"localhost:9999","dir":"/tmp"}' | grep -q ""
echo "  ✓ Route registered"

# Test 3: Route appears in list
echo "[Test 3] Route listing..."
curl -s --unix-socket ~/Library/Application\ Support/paw-proxy/paw-proxy.sock http://unix/routes | grep -q "integration-test"
echo "  ✓ Route appears in list"

# Test 4: DNS resolution
echo "[Test 4] DNS resolution..."
dig +short integration-test.test @127.0.0.1 -p 9353 | grep -q "127.0.0.1"
echo "  ✓ DNS resolves to 127.0.0.1"

# Test 5: HTTPS certificate
echo "[Test 5] HTTPS certificate..."
echo | openssl s_client -connect integration-test.test:443 -servername integration-test.test 2>/dev/null | openssl x509 -noout -subject | grep -q "integration-test.test"
echo "  ✓ Certificate issued for domain"

# Test 6: Heartbeat
echo "[Test 6] Heartbeat..."
curl -s --unix-socket ~/Library/Application\ Support/paw-proxy/paw-proxy.sock \
  -X POST http://unix/routes/integration-test/heartbeat | grep -q ""
echo "  ✓ Heartbeat accepted"

# Test 7: Deregister
echo "[Test 7] Route deregistration..."
curl -s --unix-socket ~/Library/Application\ Support/paw-proxy/paw-proxy.sock \
  -X DELETE http://unix/routes/integration-test | grep -q ""
echo "  ✓ Route deregistered"

# Test 8: Route gone
echo "[Test 8] Route removal verification..."
! curl -s --unix-socket ~/Library/Application\ Support/paw-proxy/paw-proxy.sock http://unix/routes | grep -q "integration-test"
echo "  ✓ Route no longer in list"

echo ""
echo "=== All tests passed! ==="
```

**Step 2: Make executable**

Run: `chmod +x integration-tests.sh`

**Step 3: Commit**

```bash
git add integration-tests.sh
git commit -m "test: add integration test script"
```

---

### Task 12: GitHub Actions CI

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `.github/workflows/release.yml`

**Step 1: Create CI workflow**

```yaml
# .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - uses: golangci/golangci-lint-action@v4

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go test -v -race ./...

  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: |
          GOOS=darwin GOARCH=arm64 go build -o paw-proxy-arm64 ./cmd/paw-proxy
          GOOS=darwin GOARCH=amd64 go build -o paw-proxy-amd64 ./cmd/paw-proxy
          GOOS=darwin GOARCH=arm64 go build -o up-arm64 ./cmd/up
          GOOS=darwin GOARCH=amd64 go build -o up-amd64 ./cmd/up

  integration:
    runs-on: macos-14
    if: github.event_name == 'pull_request'
    needs: [lint, test, build]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Build
        run: |
          go build -o paw-proxy ./cmd/paw-proxy
          go build -o up ./cmd/up
      - name: Setup
        run: sudo ./paw-proxy setup
      - name: Integration Tests
        run: ./integration-tests.sh
```

**Step 2: Create release workflow**

```yaml
# .github/workflows/release.yml
name: Release

on:
  push:
    tags: ['v*']

jobs:
  release:
    runs-on: macos-14
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Run tests
        run: go test -v ./...

      - name: Build binaries
        run: |
          mkdir -p dist

          # paw-proxy
          GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/paw-proxy-darwin-arm64 ./cmd/paw-proxy
          GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/paw-proxy-darwin-amd64 ./cmd/paw-proxy
          lipo -create -output dist/paw-proxy-darwin-universal dist/paw-proxy-darwin-arm64 dist/paw-proxy-darwin-amd64

          # up
          GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/up-darwin-arm64 ./cmd/up
          GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/up-darwin-amd64 ./cmd/up
          lipo -create -output dist/up-darwin-universal dist/up-darwin-arm64 dist/up-darwin-amd64

      - name: Integration tests
        run: |
          cp dist/paw-proxy-darwin-universal paw-proxy
          cp dist/up-darwin-universal up
          chmod +x paw-proxy up
          sudo ./paw-proxy setup
          ./integration-tests.sh

      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: |
            dist/paw-proxy-darwin-arm64
            dist/paw-proxy-darwin-amd64
            dist/paw-proxy-darwin-universal
            dist/up-darwin-arm64
            dist/up-darwin-amd64
            dist/up-darwin-universal
          generate_release_notes: true
```

**Step 3: Commit**

```bash
git add .github/
git commit -m "ci: add GitHub Actions for CI and releases"
```

---

## Summary

**Total Tasks:** 12
**Estimated Time:** 4-6 hours

**Phase 1 (Core):** DNS, SSL, Routes, Proxy, API - Tasks 1-5
**Phase 2 (Binaries):** daemon, setup, up - Tasks 6-8
**Phase 3 (Polish):** status, uninstall, tests, CI - Tasks 9-12

After completing all tasks:
1. Create GitHub repo
2. Push code
3. Tag v1.0.0
4. Set up Homebrew tap
