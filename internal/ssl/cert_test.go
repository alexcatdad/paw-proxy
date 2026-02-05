// internal/ssl/cert_test.go
package ssl

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"path/filepath"
	"testing"
	"time"
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

func TestCertCache_ExpiredCertEviction(t *testing.T) {
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

	// Manually create an expired certificate and inject it into the cache
	expiredCert := createExpiredCert(t, ca, "expired.test")
	cache.mu.Lock()
	cache.cache["expired.test"] = expiredCert
	cache.order = append(cache.order, "expired.test")
	cache.mu.Unlock()

	// Verify the cert is in the cache
	cache.mu.RLock()
	_, exists := cache.cache["expired.test"]
	cache.mu.RUnlock()
	if !exists {
		t.Fatal("expired cert not in cache")
	}

	// Request the expired cert - should be evicted and regenerated
	hello := &tls.ClientHelloInfo{
		ServerName: "expired.test",
	}

	newCert, err := cache.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}

	// Verify we got a new cert, not the expired one
	if newCert == expiredCert {
		t.Error("expected new cert, got expired cert")
	}

	// Verify the new cert is not expired
	if time.Now().After(newCert.Leaf.NotAfter) {
		t.Error("new cert is already expired")
	}

	// Verify the cert is still in cache (regenerated, not just deleted)
	cache.mu.RLock()
	cachedCert, exists := cache.cache["expired.test"]
	cache.mu.RUnlock()
	if !exists {
		t.Error("cert not in cache after regeneration")
	}
	if cachedCert != newCert {
		t.Error("cached cert does not match returned cert")
	}
}

// createExpiredCert creates a certificate that expired 1 day ago
func createExpiredCert(t *testing.T, ca *tls.Certificate, name string) *tls.Certificate {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	notBefore := time.Now().Add(-2 * 24 * time.Hour) // 2 days ago
	notAfter := time.Now().Add(-1 * 24 * time.Hour)  // 1 day ago (expired)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generating serial: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"paw-proxy-test"},
			CommonName:   name,
		},
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{name},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, ca.Leaf, priv.Public(), ca.PrivateKey)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}

	leaf, err := x509.ParseCertificate(derBytes)
	if err != nil {
		t.Fatalf("parsing certificate: %v", err)
	}

	return &tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
		Leaf:        leaf,
	}
}
