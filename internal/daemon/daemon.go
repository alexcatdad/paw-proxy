// internal/daemon/daemon.go
package daemon

import (
	"crypto/tls"
	"fmt"
	"log"
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
	DNSPort    int
	HTTPPort   int
	HTTPSPort  int
	TLD        string
	SupportDir string
	SocketPath string
	LogPath    string
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
	// SECURITY: Critical component failures must crash the daemon
	// Buffered channel allows all 4 goroutines to send errors without blocking
	errCh := make(chan error, 4)

	// Start DNS server
	go func() {
		log.Printf("DNS server listening on 127.0.0.1:%d", d.config.DNSPort)
		if err := d.dnsServer.Start(); err != nil {
			errCh <- fmt.Errorf("DNS server: %w", err)
		}
	}()

	// Start API server
	go func() {
		log.Printf("API server listening on %s", d.config.SocketPath)
		if err := d.apiServer.Start(); err != nil {
			errCh <- fmt.Errorf("API server: %w", err)
		}
	}()

	// Start cleanup routine
	go d.cleanupRoutine()

	// Start HTTP redirect server
	go func() {
		log.Printf("HTTP redirect server listening on 127.0.0.1:%d", d.config.HTTPPort)
		if err := d.serveHTTP(); err != nil {
			errCh <- fmt.Errorf("HTTP server: %w", err)
		}
	}()

	// Start HTTPS server in goroutine
	go func() {
		log.Printf("HTTPS server listening on 127.0.0.1:%d", d.config.HTTPSPort)
		if err := d.serveHTTPS(); err != nil {
			errCh <- fmt.Errorf("HTTPS server: %w", err)
		}
	}()

	// Block until first critical failure
	return <-errCh
}

func (d *Daemon) cleanupRoutine() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		d.registry.Cleanup()
	}
}

func (d *Daemon) serveHTTP() error {
	// SECURITY: Bind to loopback only, not all interfaces
	addr := fmt.Sprintf("127.0.0.1:%d", d.config.HTTPPort)

	server := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := "https://" + r.Host + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusPermanentRedirect)
		}),
	}

	if err := server.ListenAndServe(); err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}
	return nil
}

func (d *Daemon) serveHTTPS() error {
	// SECURITY: Bind to loopback only, not all interfaces
	addr := fmt.Sprintf("127.0.0.1:%d", d.config.HTTPSPort)

	// SECURITY: TLS hardening - minimum TLS 1.2, secure cipher suites
	tlsConfig := &tls.Config{
		GetCertificate: d.certCache.GetCertificate,
		MinVersion:     tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}

	listener, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}

	server := &http.Server{
		Handler:     http.HandlerFunc(d.handleRequest),
		ReadTimeout: 30 * time.Second,
		// WriteTimeout disabled (0) to support SSE connections (Vite HMR, Next.js Fast Refresh, etc.)
		// SSE keeps connections open indefinitely, and a fixed timeout breaks hot reload.
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
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
