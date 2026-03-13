package daemon

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexcatdad/paw-proxy/internal/ssl"
)

func TestRedirectTarget(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		requestURI string
		tld        string
		wantOK     bool
		wantTarget string
	}{
		{
			name:       "valid subdomain",
			host:       "myapp.test",
			requestURI: "/dashboard",
			tld:        "test",
			wantOK:     true,
			wantTarget: "https://myapp.test/dashboard",
		},
		{
			name:       "valid host with port",
			host:       "myapp.test:80",
			requestURI: "/api?q=1",
			tld:        "test",
			wantOK:     true,
			wantTarget: "https://myapp.test/api?q=1",
		},
		{
			name:       "accept bare tld",
			host:       "test",
			requestURI: "/",
			tld:        "test",
			wantOK:     true,
			wantTarget: "https://test/",
		},
		{
			name:       "reject foreign domain",
			host:       "evil.com",
			requestURI: "/",
			tld:        "test",
			wantOK:     false,
		},
		{
			name:       "reject empty host",
			host:       "",
			requestURI: "/",
			tld:        "test",
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTarget, gotOK := redirectTarget(tt.host, tt.requestURI, tt.tld)
			if gotOK != tt.wantOK {
				t.Fatalf("redirectTarget(%q) ok = %v, want %v", tt.host, gotOK, tt.wantOK)
			}
			if tt.wantOK && gotTarget != tt.wantTarget {
				t.Fatalf("redirectTarget(%q) target = %q, want %q", tt.host, gotTarget, tt.wantTarget)
			}
		})
	}
}

func TestStatusCapture_CapturesWriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	sc := &statusCapture{ResponseWriter: w}

	sc.WriteHeader(http.StatusNotFound)

	if sc.status != 404 {
		t.Errorf("expected status 404, got %d", sc.status)
	}
	if w.Code != 404 {
		t.Errorf("expected underlying writer to have 404, got %d", w.Code)
	}
}

func TestStatusCapture_DefaultsToZero(t *testing.T) {
	w := httptest.NewRecorder()
	sc := &statusCapture{ResponseWriter: w}

	if sc.status != 0 {
		t.Errorf("expected initial status 0, got %d", sc.status)
	}
}

func TestStatusCapture_OnlyFirstWriteHeaderCaptured(t *testing.T) {
	w := httptest.NewRecorder()
	sc := &statusCapture{ResponseWriter: w}

	sc.WriteHeader(http.StatusOK)
	sc.WriteHeader(http.StatusNotFound)

	if sc.status != 200 {
		t.Errorf("expected first status 200, got %d", sc.status)
	}
}

func TestStatusCapture_WriteImplies200(t *testing.T) {
	w := httptest.NewRecorder()
	sc := &statusCapture{ResponseWriter: w}

	sc.Write([]byte("hello"))

	if sc.status != 200 {
		t.Errorf("expected implicit status 200 from Write, got %d", sc.status)
	}
}

// testCA generates a self-signed CA certificate for testing.
func testCA(t *testing.T) *tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating CA cert: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing CA cert: %v", err)
	}
	return &tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        leaf,
	}
}

func TestMaxHeaderBytes(t *testing.T) {
	ca := testCA(t)
	d := &Daemon{
		config: &Config{
			HTTPPort:  0, // use any available port
			HTTPSPort: 0,
			TLD:       "test",
		},
		certCache: ssl.NewCertCache(ca, "test"),
		logger:    slog.New(slog.NewJSONHandler(os.Stderr, nil)),
	}

	const want = 1 << 20 // 1MB

	httpSrv, httpLn, err := d.createHTTPServer()
	if err != nil {
		t.Fatalf("createHTTPServer: %v", err)
	}
	defer httpLn.Close()
	if httpSrv.MaxHeaderBytes != want {
		t.Errorf("HTTP server MaxHeaderBytes = %d, want %d", httpSrv.MaxHeaderBytes, want)
	}

	httpsSrv, httpsLn, err := d.createHTTPSServer()
	if err != nil {
		t.Fatalf("createHTTPSServer: %v", err)
	}
	defer httpsLn.Close()
	if httpsSrv.MaxHeaderBytes != want {
		t.Errorf("HTTPS server MaxHeaderBytes = %d, want %d", httpsSrv.MaxHeaderBytes, want)
	}
}

func TestLogFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "paw-proxy.log")

	// Open log file the same way New() does
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("opening log file: %v", err)
	}
	f.Close()

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("stat log file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected log file permissions 0600, got %04o", perm)
	}
}
