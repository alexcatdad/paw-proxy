// internal/ssl/cert_test.go
package ssl

import (
	"crypto/tls"
	"path/filepath"
	"sync"
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

func TestCertCacheRejectsEmptySNI(t *testing.T) {
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

	// SECURITY: Empty SNI should be rejected
	hello := &tls.ClientHelloInfo{
		ServerName: "",
	}

	_, err = cache.GetCertificate(hello)
	if err == nil {
		t.Fatal("expected error for empty SNI, got nil")
	}

	expected := "SNI required"
	if got := err.Error(); len(got) < len(expected) || got[:len(expected)] != expected {
		t.Errorf("expected error containing %q, got %q", expected, got)
	}
}

func TestCertCacheConcurrentAccess(t *testing.T) {
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

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			hello := &tls.ClientHelloInfo{
				ServerName: "concurrent.test",
			}
			cert, err := cache.GetCertificate(hello)
			if err != nil {
				errs <- err
				return
			}
			if cert.Leaf.Subject.CommonName != "concurrent.test" {
				errs <- err
				return
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent GetCertificate error: %v", err)
	}
}
