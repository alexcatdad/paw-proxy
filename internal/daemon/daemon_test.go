package daemon

import "testing"

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
