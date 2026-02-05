// internal/proxy/proxy_test.go
package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxy_ForwardsRequest(t *testing.T) {
	// Create upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-Proto") != "https" {
			t.Error("missing X-Forwarded-Proto header")
		}
		w.Write([]byte("hello from upstream"))
	}))
	defer upstream.Close()

	// Create proxy
	p := New()

	// Create test request
	req := httptest.NewRequest("GET", "https://myapp.test/api", nil)
	w := httptest.NewRecorder()

	// Proxy the request
	p.ServeHTTP(w, req, upstream.URL[7:]) // strip "http://"

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != "hello from upstream" {
		t.Errorf("unexpected body: %s", string(body))
	}
}
