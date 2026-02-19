package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
