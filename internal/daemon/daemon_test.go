package daemon

import (
	"fmt"
	"testing"
)

func TestLoopbackAddresses(t *testing.T) {
	port := 8080

	tests := []struct {
		name   string
		format string
		want   string
	}{
		{"IPv4 HTTP", "127.0.0.1:%d", "127.0.0.1:8080"},
		{"IPv6 HTTP", "[::1]:%d", "[::1]:8080"},
		{"IPv4 HTTPS", "127.0.0.1:%d", "127.0.0.1:8080"},
		{"IPv6 HTTPS", "[::1]:%d", "[::1]:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := fmt.Sprintf(tt.format, port)
			if addr != tt.want {
				t.Errorf("got %s, want %s", addr, tt.want)
			}
		})
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
