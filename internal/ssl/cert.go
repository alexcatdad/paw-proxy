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
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"time"
)

// SECURITY: Limit cache size to prevent memory exhaustion
const maxCacheSize = 1000

type CertCache struct {
	ca     *tls.Certificate
	tld    string
	cache  map[string]*tls.Certificate
	order  []string // Track insertion order for LRU eviction
	mu     sync.RWMutex
	logger *slog.Logger
}

func NewCertCache(ca *tls.Certificate, tld string) *CertCache {
	return &CertCache{
		ca:    ca,
		tld:   tld,
		cache: make(map[string]*tls.Certificate),
		order: make([]string, 0, maxCacheSize),
	}
}

// SetLogger configures structured logging for TLS errors.
func (c *CertCache) SetLogger(logger *slog.Logger) {
	c.logger = logger
}

func (c *CertCache) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := hello.ServerName

	// SECURITY: Reject empty SNI to prevent serving a default cert for IP-based connections
	if name == "" {
		if c.logger != nil {
			c.logger.Warn("TLS: empty SNI rejected")
		}
		return nil, fmt.Errorf("SNI required: connect using hostname, not IP")
	}

	// Use wildcard cert for all domains under the configured TLD.
	// *.test covers myapp.test, api.test, etc.
	wildcardName := "*." + c.tld

	// Fast path: read lock for cache hit (non-expired)
	c.mu.RLock()
	if cert, ok := c.cache[wildcardName]; ok {
		if cert.Leaf == nil || time.Now().Before(cert.Leaf.NotAfter) {
			c.mu.RUnlock()
			return cert, nil
		}
	}
	c.mu.RUnlock()

	// Slow path: write lock for miss or expired
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock (avoids TOCTOU race)
	if cert, ok := c.cache[wildcardName]; ok {
		if cert.Leaf == nil || time.Now().Before(cert.Leaf.NotAfter) {
			return cert, nil
		}
		// Expired, remove and regenerate
		delete(c.cache, wildcardName)
		c.removeFromOrder(wildcardName)
	}

	cert, err := c.generateCert(wildcardName)
	if err != nil {
		if c.logger != nil {
			c.logger.Error("TLS: cert generation failed", "name", wildcardName, "error", err)
		}
		return nil, err
	}

	// SECURITY: Evict oldest entry if cache is full
	if len(c.cache) >= maxCacheSize {
		oldest := c.order[0]
		delete(c.cache, oldest)
		c.order = c.order[1:]
	}

	c.cache[wildcardName] = cert
	c.order = append(c.order, wildcardName)
	return cert, nil
}

func (c *CertCache) removeFromOrder(name string) {
	for i, n := range c.order {
		if n == name {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
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

	dnsNames := []string{name}
	// For wildcard certs, also include the bare TLD domain
	if strings.HasPrefix(name, "*.") {
		dnsNames = append(dnsNames, strings.TrimPrefix(name, "*."))
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
		DNSNames:    dnsNames,
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
