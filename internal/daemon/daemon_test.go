package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPRedirectUses308(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + r.Host + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})

	req := httptest.NewRequest("POST", "http://myapp.test/api/data", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusPermanentRedirect {
		t.Errorf("expected status %d, got %d", http.StatusPermanentRedirect, rr.Code)
	}

	location := rr.Header().Get("Location")
	expected := "https://myapp.test/api/data"
	if location != expected {
		t.Errorf("expected Location %q, got %q", expected, location)
	}
}

func TestHTTPRedirectPreservesFullURL(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + r.Host + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})

	tests := []struct {
		name     string
		url      string
		host     string
		expected string
	}{
		{
			name:     "path only",
			url:      "http://myapp.test/dashboard",
			host:     "myapp.test",
			expected: "https://myapp.test/dashboard",
		},
		{
			name:     "path with query string",
			url:      "http://myapp.test/search?q=hello&page=2",
			host:     "myapp.test",
			expected: "https://myapp.test/search?q=hello&page=2",
		},
		{
			name:     "root path",
			url:      "http://myapp.test/",
			host:     "myapp.test",
			expected: "https://myapp.test/",
		},
		{
			name:     "path with fragment is stripped by HTTP",
			url:      "http://myapp.test/page",
			host:     "myapp.test",
			expected: "https://myapp.test/page",
		},
		{
			name:     "encoded path segments",
			url:      "http://myapp.test/path%20with%20spaces",
			host:     "myapp.test",
			expected: "https://myapp.test/path%20with%20spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusPermanentRedirect {
				t.Errorf("expected status %d, got %d", http.StatusPermanentRedirect, rr.Code)
			}

			location := rr.Header().Get("Location")
			if location != tt.expected {
				t.Errorf("expected Location %q, got %q", tt.expected, location)
			}
		})
	}
}

func TestExtractName(t *testing.T) {
	tests := []struct {
		host     string
		expected string
	}{
		{"myapp.test", "myapp"},
		{"myapp.test:443", "myapp"},
		{"dashboard.test", "dashboard"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := extractName(tt.host)
			if got != tt.expected {
				t.Errorf("extractName(%q) = %q, want %q", tt.host, got, tt.expected)
			}
		})
	}
}
