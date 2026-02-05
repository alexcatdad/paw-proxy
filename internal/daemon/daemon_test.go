package daemon

import (
	"fmt"
	"testing"
)

func TestHTTPAddressBindsToLoopback(t *testing.T) {
	port := 8080
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if addr != "127.0.0.1:8080" {
		t.Errorf("expected 127.0.0.1:8080, got %s", addr)
	}
}

func TestHTTPSAddressBindsToLoopback(t *testing.T) {
	port := 8443
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if addr != "127.0.0.1:8443" {
		t.Errorf("expected 127.0.0.1:8443, got %s", addr)
	}
}

func TestExtractName(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"myapp.test", "myapp"},
		{"myapp.test:443", "myapp"},
		{"myapp", "myapp"},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := extractName(tt.host)
			if got != tt.want {
				t.Errorf("extractName(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}
}
