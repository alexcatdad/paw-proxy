// internal/daemon/daemon.go
package daemon

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/alexcatdad/paw-proxy/internal/api"
	"github.com/alexcatdad/paw-proxy/internal/dns"
	"github.com/alexcatdad/paw-proxy/internal/errorpage"
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

func DefaultConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	supportDir := filepath.Join(homeDir, "Library", "Application Support", "paw-proxy")

	return &Config{
		DNSPort:    9353,
		HTTPPort:   80,
		HTTPSPort:  443,
		TLD:        "test",
		SupportDir: supportDir,
		SocketPath: filepath.Join(supportDir, "paw-proxy.sock"),
		LogPath:    filepath.Join(homeDir, "Library", "Logs", "paw-proxy.log"),
	}, nil
}

type Daemon struct {
	config    *Config
	dnsServer *dns.Server
	registry  *api.RouteRegistry
	apiServer *api.Server
	certCache *ssl.CertCache
	proxy     *proxy.Proxy
	logger    *slog.Logger
	logFile   *os.File
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

	// Set up structured JSON logger
	logFile, err := os.OpenFile(config.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	logger := slog.New(slog.NewJSONHandler(logFile, nil))

	// Warn if CA certificate is near expiry
	if ca.Leaf != nil {
		daysLeft := int(time.Until(ca.Leaf.NotAfter).Hours() / 24)
		if daysLeft < 30 {
			logger.Warn("CA certificate expiring soon", "days_left", daysLeft)
		}
	}

	// Create DNS server
	dnsAddr := fmt.Sprintf("127.0.0.1:%d", config.DNSPort)
	dnsServer, err := dns.NewServer(dnsAddr, config.TLD)
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("creating DNS server: %w", err)
	}

	// Create route registry with 30s heartbeat timeout
	registry := api.NewRouteRegistry(30 * time.Second)

	// Create API server
	apiServer := api.NewServer(config.SocketPath, registry)

	certCache := ssl.NewCertCache(ca, config.TLD)
	certCache.SetLogger(logger)

	return &Daemon{
		config:    config,
		dnsServer: dnsServer,
		registry:  registry,
		apiServer: apiServer,
		certCache: certCache,
		proxy:     proxy.New(),
		logger:    logger,
		logFile:   logFile,
	}, nil
}

func (d *Daemon) Run() error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	errCh := make(chan error, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// Start DNS server
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.logger.Info("server started", "component", "dns", "addr", fmt.Sprintf("127.0.0.1:%d", d.config.DNSPort))
		if err := d.dnsServer.Start(); err != nil {
			errCh <- fmt.Errorf("DNS server: %w", err)
		}
	}()

	// Start API server
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.logger.Info("server started", "component", "api", "addr", d.config.SocketPath)
		if err := d.apiServer.Start(); err != nil {
			// http.ErrServerClosed is expected during graceful shutdown
			if err != http.ErrServerClosed {
				errCh <- fmt.Errorf("API server: %w", err)
			}
		}
	}()

	// Start cleanup routine
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.cleanupRoutine(ctx)
	}()

	// Start HTTP redirect server
	httpServer, httpListener, err := d.createHTTPServer()
	if err != nil {
		cancel()
		d.dnsServer.Stop()
		d.apiServer.Stop()
		return fmt.Errorf("creating HTTP server: %w", err)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.logger.Info("server started", "component", "http", "addr", httpListener.Addr().String())
		if err := httpServer.Serve(httpListener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("HTTP server: %w", err)
		}
	}()

	// Start HTTPS server
	httpsServer, httpsListener, err := d.createHTTPSServer()
	if err != nil {
		cancel()
		httpListener.Close()
		d.dnsServer.Stop()
		d.apiServer.Stop()
		return fmt.Errorf("creating HTTPS server: %w", err)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.logger.Info("server started", "component", "https", "addr", httpsListener.Addr().String())
		if err := httpsServer.Serve(httpsListener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("HTTPS server: %w", err)
		}
	}()

	// Wait for signal or component failure
	select {
	case sig := <-sigCh:
		d.logger.Info("shutdown signal received", "signal", sig.String())
	case err := <-errCh:
		d.logger.Error("component failure", "error", err)
	}

	// Begin graceful shutdown
	cancel() // stop cleanup routine

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	// Shut down all servers concurrently
	var shutdownWg sync.WaitGroup

	shutdownWg.Add(1)
	go func() {
		defer shutdownWg.Done()
		if err := httpsServer.Shutdown(shutdownCtx); err != nil {
			d.logger.Error("shutdown error", "component", "https", "error", err)
		}
	}()

	shutdownWg.Add(1)
	go func() {
		defer shutdownWg.Done()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			d.logger.Error("shutdown error", "component", "http", "error", err)
		}
	}()

	shutdownWg.Add(1)
	go func() {
		defer shutdownWg.Done()
		if err := d.apiServer.Stop(); err != nil {
			d.logger.Error("shutdown error", "component", "api", "error", err)
		}
	}()

	shutdownWg.Add(1)
	go func() {
		defer shutdownWg.Done()
		if err := d.dnsServer.Stop(); err != nil {
			d.logger.Error("shutdown error", "component", "dns", "error", err)
		}
	}()

	shutdownWg.Wait()

	// Clean up socket file
	if err := os.Remove(d.config.SocketPath); err != nil && !os.IsNotExist(err) {
		d.logger.Warn("socket cleanup failed", "error", err)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	d.logger.Info("shutdown complete")

	// Close log file after all logging is done
	if d.logFile != nil {
		d.logFile.Close()
	}

	return nil
}

func (d *Daemon) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.registry.Cleanup()
		}
	}
}

// createHTTPServer creates the HTTP redirect server and its listener.
// The caller owns the lifecycle of the returned server.
func (d *Daemon) createHTTPServer() (*http.Server, net.Listener, error) {
	// SECURITY: Bind to loopback only to prevent external access
	addr := fmt.Sprintf("127.0.0.1:%d", d.config.HTTPPort)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("listening on %s: %w", addr, err)
	}

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := "https://" + r.Host + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusPermanentRedirect)
		}),
	}

	return server, listener, nil
}

// createHTTPSServer creates the HTTPS server and its TLS listener.
// The caller owns the lifecycle of the returned server.
func (d *Daemon) createHTTPSServer() (*http.Server, net.Listener, error) {
	// SECURITY: Bind to loopback only to prevent external access
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
		return nil, nil, fmt.Errorf("listening on %s: %w", addr, err)
	}

	server := &http.Server{
		Handler:     http.HandlerFunc(d.handleRequest),
		IdleTimeout: 120 * time.Second,
	}

	return server, listener, nil
}

func (d *Daemon) handleRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	route, ok := d.registry.LookupByHost(r.Host)
	if !ok {
		d.serveNotFound(w, r)
		d.logger.Info("request",
			"host", r.Host,
			"method", r.Method,
			"path", r.URL.Path,
			"status", 404,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return
	}

	d.proxy.ServeHTTP(w, r, route.Upstream)
	d.logger.Info("request",
		"host", r.Host,
		"method", r.Method,
		"path", r.URL.Path,
		"route", route.Name,
		"upstream", route.Upstream,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

func (d *Daemon) serveNotFound(w http.ResponseWriter, r *http.Request) {
	appName := api.ExtractName(r.Host)
	routes := d.registry.List()
	var names []string
	for _, route := range routes {
		names = append(names, route.Name)
	}
	errorpage.NotFound(w, r.Host, appName, names)
}
