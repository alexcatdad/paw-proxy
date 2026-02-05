// internal/ssl/ca_test.go
package ssl

import (
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
