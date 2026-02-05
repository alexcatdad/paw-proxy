// internal/ssl/cert_test.go
package ssl

import (
	"crypto/tls"
	"fmt"
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

// TestCertCacheLRUEviction fills the cache to maxCacheSize by directly inserting
// entries, then adds one more via GetCertificate and verifies the oldest entry
// was evicted. This avoids generating 1001 real certs.
func TestCertCacheLRUEviction(t *testing.T) {
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

	// Generate one real cert to use as a placeholder for filling the cache
	hello := &tls.ClientHelloInfo{
		ServerName: "placeholder.test",
	}
	placeholderCert, err := cache.GetCertificate(hello)
	if err != nil {
		t.Fatalf("failed to generate placeholder cert: %v", err)
	}

	// Directly fill the cache to maxCacheSize by inserting fake entries.
	// We hold the lock to manipulate cache internals directly.
	cache.mu.Lock()
	// Clear the placeholder entry first
	delete(cache.cache, "placeholder.test")
	cache.order = cache.order[:0]

	for i := 0; i < maxCacheSize; i++ {
		name := fmt.Sprintf("domain-%04d.test", i)
		cache.cache[name] = placeholderCert
		cache.order = append(cache.order, name)
	}
	cache.mu.Unlock()

	// Verify cache is full
	cache.mu.RLock()
	if len(cache.cache) != maxCacheSize {
		t.Fatalf("expected cache size %d, got %d", maxCacheSize, len(cache.cache))
	}
	cache.mu.RUnlock()

	// The oldest entry is "domain-0000.test"
	// Now add one more entry via GetCertificate, which should evict it
	newHello := &tls.ClientHelloInfo{
		ServerName: "newdomain.test",
	}
	_, err = cache.GetCertificate(newHello)
	if err != nil {
		t.Fatalf("GetCertificate for new domain failed: %v", err)
	}

	// Verify cache is still at maxCacheSize (not maxCacheSize+1)
	cache.mu.RLock()
	defer cache.mu.RUnlock()

	if len(cache.cache) != maxCacheSize {
		t.Errorf("expected cache size %d after eviction, got %d", maxCacheSize, len(cache.cache))
	}

	// Verify the oldest entry was evicted
	if _, ok := cache.cache["domain-0000.test"]; ok {
		t.Error("expected oldest entry 'domain-0000.test' to be evicted, but it still exists")
	}

	// Verify the new entry exists
	if _, ok := cache.cache["newdomain.test"]; !ok {
		t.Error("expected new entry 'newdomain.test' to exist in cache")
	}

	// Verify a non-evicted entry still exists
	if _, ok := cache.cache["domain-0001.test"]; !ok {
		t.Error("expected 'domain-0001.test' to still exist in cache")
	}
}
