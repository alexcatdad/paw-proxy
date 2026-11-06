// internal/ssl/ca.go
package ssl

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"
)

func GenerateCA(certPath, keyPath string) error {
	// SECURITY: Use 4096-bit key for CA (stronger than 2048)
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("generating RSA key: %w", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(10 * 365 * 24 * time.Hour) // 10 years

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generating serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"paw-proxy"},
			CommonName:   "paw-proxy CA",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		// SECURITY: MaxPathLen=0 prevents this CA from signing intermediate CAs
		MaxPathLen:     0,
		MaxPathLenZero: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, priv.Public(), priv)
	if err != nil {
		return fmt.Errorf("creating certificate: %w", err)
	}

	// SECURITY: Write cert with explicit 0644 permissions (public cert, readable by all)
	certOut, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644) //nolint:gosec // G302/G304: the CA certificate is public and must be readable; the path comes from internal/paths
	if err != nil {
		return fmt.Errorf("creating cert file: %w", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		_ = certOut.Close()
		return fmt.Errorf("encoding certificate: %w", err)
	}
	if err := certOut.Close(); err != nil {
		return fmt.Errorf("writing cert file: %w", err)
	}

	// SECURITY: Write key with 0600 permissions (private key, owner-only)
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600) //nolint:gosec // G304: the path comes from internal/paths, not from user input
	if err != nil {
		return fmt.Errorf("creating key file: %w", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		_ = keyOut.Close()
		return fmt.Errorf("encoding private key: %w", err)
	}
	if err := keyOut.Close(); err != nil {
		return fmt.Errorf("writing key file: %w", err)
	}

	return nil
}

func LoadCA(certPath, keyPath string) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("loading key pair: %w", err)
	}

	cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %w", err)
	}

	return &cert, nil
}
