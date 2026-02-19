package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexcatdad/paw-proxy/internal/api"
)

type mockRouteProvider struct {
	routes []api.Route
}

func (m *mockRouteProvider) List() []api.Route {
	return m.routes
}

func newTestDashboard(t *testing.T, metrics *Metrics, routes RouteProvider, version string, startTime time.Time) *Dashboard {
	t.Helper()
	d, err := New(metrics, routes, version, startTime)
	if err != nil {
		t.Fatalf("failed to create dashboard: %v", err)
	}
	return d
}

func TestDashboard_ServesHTML(t *testing.T) {
	d := newTestDashboard(t, NewMetrics(10), &mockRouteProvider{}, "1.0.0", time.Now())

	req := httptest.NewRequest("GET", "https://_paw.test/", nil)
	w := httptest.NewRecorder()
	d.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html content type, got %s", ct)
	}
	if !strings.Contains(w.Body.String(), "paw-proxy") {
		t.Error("expected 'paw-proxy' in HTML body")
	}
}

func TestDashboard_ServesCSS(t *testing.T) {
	d := newTestDashboard(t, NewMetrics(10), &mockRouteProvider{}, "1.0.0", time.Now())

	req := httptest.NewRequest("GET", "https://_paw.test/style.css", nil)
	w := httptest.NewRecorder()
	d.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "css") {
		t.Errorf("expected CSS content type, got %s", ct)
	}
}

func TestDashboard_ServesJS(t *testing.T) {
	d := newTestDashboard(t, NewMetrics(10), &mockRouteProvider{}, "1.0.0", time.Now())

	req := httptest.NewRequest("GET", "https://_paw.test/app.js", nil)
	w := httptest.NewRecorder()
	d.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "EventSource") {
		t.Error("expected JavaScript SSE code in body")
	}
}

func TestDashboard_APIRoutes(t *testing.T) {
	now := time.Now()
	routes := &mockRouteProvider{
		routes: []api.Route{
			{Name: "myapp", Upstream: "localhost:3000", Dir: "/home/user/myapp", Registered: now},
		},
	}
	m := NewMetrics(10)
	m.Record(RequestEntry{Timestamp: now, Route: "myapp", StatusCode: 200, LatencyMs: 25})
	m.Record(RequestEntry{Timestamp: now, Route: "myapp", StatusCode: 500, LatencyMs: 100})

	d := newTestDashboard(t, m, routes, "1.0.0", now)

	req := httptest.NewRequest("GET", "https://_paw.test/api/routes", nil)
	w := httptest.NewRecorder()
	d.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 route, got %d", len(result))
	}
	if result[0]["name"] != "myapp" {
		t.Errorf("expected name 'myapp', got %v", result[0]["name"])
	}
	if result[0]["requests"].(float64) != 2 {
		t.Errorf("expected 2 requests, got %v", result[0]["requests"])
	}
	// (25 + 100) / 2 = 62 (integer division)
	if result[0]["avgMs"].(float64) != 62 {
		t.Errorf("expected avgMs 62, got %v", result[0]["avgMs"])
	}
	if result[0]["errors"].(float64) != 1 {
		t.Errorf("expected 1 error, got %v", result[0]["errors"])
	}
}

func TestDashboard_APIStats(t *testing.T) {
	startTime := time.Now().Add(-5 * time.Minute)
	d := newTestDashboard(t, NewMetrics(10), &mockRouteProvider{}, "1.2.3", startTime)

	req := httptest.NewRequest("GET", "https://_paw.test/api/stats", nil)
	w := httptest.NewRecorder()
	d.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if result["version"] != "1.2.3" {
		t.Errorf("expected version '1.2.3', got %v", result["version"])
	}
	if _, ok := result["uptime"]; !ok {
		t.Error("expected 'uptime' field in stats")
	}
}

func TestDashboard_SSEHeaders(t *testing.T) {
	d := newTestDashboard(t, NewMetrics(10), &mockRouteProvider{}, "1.0.0", time.Now())

	// Use a real httptest.Server to avoid data races with ResponseRecorder.
	srv := httptest.NewServer(d)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", srv.URL+"/events", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Context cancellation is expected; check headers if we got a response.
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}
	cc := resp.Header.Get("Cache-Control")
	if cc != "no-cache" {
		t.Errorf("expected no-cache, got %s", cc)
	}
}
