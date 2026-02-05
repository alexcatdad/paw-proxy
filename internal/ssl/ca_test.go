// internal/ssl/ca_test.go
package ssl

import (
	"crypto/x509"
	"encoding/pem"
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

func TestGenerateCA_MaxPathLen(t *testing.T) {
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

	// SECURITY: CA should only sign leaf certs, not intermediate CAs
	if ca.Leaf.MaxPathLen != 0 {
		t.Errorf("expected MaxPathLen=0, got %d", ca.Leaf.MaxPathLen)
	}
	if !ca.Leaf.MaxPathLenZero {
		t.Error("expected MaxPathLenZero=true")
	}
}

func TestGenerateCA_NoExtKeyUsage(t *testing.T) {
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

	// SECURITY: CA should not have ExtKeyUsage (only leaf certs need ServerAuth)
	if len(ca.Leaf.ExtKeyUsage) != 0 {
		t.Errorf("expected no ExtKeyUsage on CA, got %v", ca.Leaf.ExtKeyUsage)
	}
}

func TestGenerateCA_CertFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "ca.crt")
	keyPath := filepath.Join(tmpDir, "ca.key")

	err := GenerateCA(certPath, keyPath)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	// Cert file should be 0644 (readable by all)
	certInfo, err := os.Stat(certPath)
	if err != nil {
		t.Fatalf("stat cert file: %v", err)
	}
	if perm := certInfo.Mode().Perm(); perm != 0644 {
		t.Errorf("expected cert file permissions 0644, got %04o", perm)
	}

	// Key file should be 0600 (owner-only)
	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if perm := keyInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("expected key file permissions 0600, got %04o", perm)
	}
}

func TestGenerateCA_PemEncodeErrorOnReadOnlyPath(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "ca.crt")
	keyPath := filepath.Join(tmpDir, "ca.key")

	// Create a read-only directory to force pem.Encode errors
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0555); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	badCertPath := filepath.Join(readOnlyDir, "ca.crt")
	badKeyPath := filepath.Join(readOnlyDir, "ca.key")

	// Should fail to create cert file in read-only directory
	err := GenerateCA(badCertPath, badKeyPath)
	if err == nil {
		t.Error("expected error when writing to read-only directory")
	}

	// Also verify a valid CA generates parseable PEM
	err = GenerateCA(certPath, keyPath)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	certData, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("reading cert: %v", err)
	}
	block, _ := pem.Decode(certData)
	if block == nil {
		t.Fatal("failed to decode PEM block from cert")
	}
	if block.Type != "CERTIFICATE" {
		t.Errorf("expected PEM type CERTIFICATE, got %s", block.Type)
	}
	_, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parsing certificate DER: %v", err)
	}
}
