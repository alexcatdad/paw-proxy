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
