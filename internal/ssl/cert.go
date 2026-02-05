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

// SECURITY: Limit cache size to prevent memory exhaustion
const maxCacheSize = 1000

type CertCache struct {
	ca    *tls.Certificate
	cache map[string]*tls.Certificate
	order []string // Track insertion order for LRU eviction
	mu    sync.RWMutex
}

func NewCertCache(ca *tls.Certificate) *CertCache {
	return &CertCache{
		ca:    ca,
		cache: make(map[string]*tls.Certificate),
		order: make([]string, 0, maxCacheSize),
	}
}

func (c *CertCache) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := hello.ServerName
	if name == "" {
		return nil, fmt.Errorf("SNI required: connect using hostname, not IP")
	}

	c.mu.RLock()
	if cert, ok := c.cache[name]; ok {
		// SECURITY: Check certificate expiry
		if cert.Leaf != nil && time.Now().After(cert.Leaf.NotAfter) {
			c.mu.RUnlock()
			// Certificate expired, regenerate
			c.mu.Lock()
			// SECURITY: Re-check expiry after acquiring write lock to prevent TOCTOU race
			// Another goroutine may have already deleted or regenerated this cert
			if cert, ok := c.cache[name]; ok && cert.Leaf != nil && time.Now().After(cert.Leaf.NotAfter) {
				delete(c.cache, name)
				c.removeFromOrder(name)
			}
			c.mu.Unlock()
		} else {
			c.mu.RUnlock()
			return cert, nil
		}
	} else {
		c.mu.RUnlock()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if cert, ok := c.cache[name]; ok {
		if cert.Leaf == nil || time.Now().Before(cert.Leaf.NotAfter) {
			return cert, nil
		}
		// Expired, remove and regenerate
		delete(c.cache, name)
		c.removeFromOrder(name)
	}

	cert, err := c.generateCert(name)
	if err != nil {
		return nil, err
	}

	// SECURITY: Evict oldest entry if cache is full
	if len(c.cache) >= maxCacheSize {
		oldest := c.order[0]
		delete(c.cache, oldest)
		c.order = c.order[1:]
	}

	c.cache[name] = cert
	c.order = append(c.order, name)
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
